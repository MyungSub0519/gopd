package document

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/MyungSub0519/gopd/internal/pdfmodel"
)

// bufferedInput retains one aligned block. Independent cursors share the block,
// but never retain slices into it; nested object loading can safely replace it.
// Like Document's other lazy state, this reader is not concurrent-safe.
type bufferedInput struct {
	reader     io.ReaderAt
	size       int64
	bufferSize int
	block      []byte
	start      int64
	err        error
	closed     bool
}

func newBufferedInput(r io.ReaderAt, size int64, bufferSize int) *bufferedInput {
	return &bufferedInput{reader: r, size: size, bufferSize: bufferSize, start: -1}
}

func (r *bufferedInput) ReadAt(p []byte, off int64) (int, error) {
	if r.closed {
		return 0, os.ErrClosed
	}
	if off < 0 {
		return 0, fmt.Errorf("negative input offset %d", off)
	}
	if len(p) == 0 {
		return 0, nil
	}
	if r.bufferSize <= 0 {
		return 0, errors.New("input buffer size must be positive")
	}
	n := 0
	for n < len(p) {
		if off >= r.size {
			return n, io.EOF
		}
		start := off - off%int64(r.bufferSize)
		if r.start != start {
			length := int(min(int64(r.bufferSize), r.size-start))
			if cap(r.block) < length {
				r.block = make([]byte, length)
			} else {
				r.block = r.block[:length]
			}
			got, err := r.reader.ReadAt(r.block, start)
			if got < 0 || got > length {
				got, err = 0, errors.New("invalid ReaderAt byte count")
			}
			if got < length && err == nil {
				err = io.ErrUnexpectedEOF
			}
			if got < length && errors.Is(err, io.EOF) {
				// Distinguish a prematurely exhausted backing source from the
				// cursor's ordinary EOF while preserving the original cause.
				err = fmt.Errorf("short PDF input read: %w", err)
			}
			if got == length && errors.Is(err, io.EOF) {
				err = nil
			}
			r.block, r.start, r.err = r.block[:got], start, err
		}
		index := off - r.start
		if index >= int64(len(r.block)) {
			if r.err != nil {
				return n, r.err
			}
			return n, io.ErrUnexpectedEOF
		}
		copied := copy(p[n:], r.block[int(index):])
		n += copied
		off += int64(copied)
		// A backing read error remains observable even when its prefix contains
		// all requested bytes. A later cache hit must not hide the same error.
		if r.err != nil {
			return n, r.err
		}
	}
	return n, nil
}

type inputCursor struct {
	d   *Document
	pos int64
}

func (d *Document) cursor(pos int64) *inputCursor { return &inputCursor{d: d, pos: pos} }

func (c *inputCursor) peek() (byte, error) {
	var p [1]byte
	_, err := c.d.input.ReadAt(p[:], c.pos)
	return p[0], err
}

func (c *inputCursor) take() (byte, error) {
	b, err := c.peek()
	if err == nil {
		c.pos++
	}
	return b, err
}

func (c *inputCursor) prefix(want string) (bool, error) {
	look := *c
	for i := range len(want) {
		b, err := look.take()
		if err == io.EOF {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if b != want[i] {
			return false, nil
		}
	}
	return true, nil
}

func (c *inputCursor) skipSpace() error {
	f := objectFrame{cursor: c}
	return f.space(false)
}

func (c *inputCursor) word() (string, int64, error) {
	if err := c.skipSpace(); err != nil {
		return "", c.pos, err
	}
	start := c.pos
	var data []byte
	for {
		b, err := c.peek()
		if err == io.EOF || err == nil && docDelimiter(b) {
			break
		}
		if err != nil {
			return "", start, err
		}
		if int64(len(data)) >= c.d.Options.Limits.MaxTokenBytes {
			return "", start, frameLimit(start, "token byte limit exceeded")
		}
		data = append(data, b)
		c.pos++
	}
	if len(data) == 0 {
		return "", start, fmt.Errorf("expected token at byte %d", start)
	}
	return string(data), start, nil
}

func (c *inputCursor) uint(bits int) (uint64, int64, error) {
	word, start, err := c.word()
	if err != nil {
		return 0, start, err
	}
	n, err := strconv.ParseUint(word, 10, bits)
	if err != nil {
		return 0, start, fmt.Errorf("invalid unsigned integer at byte %d: %w", start, err)
	}
	return n, start, nil
}

// object frames ISO 32000-1:2008, 7.2-7.3 syntax before parsing it once. The
// frame stops at the direct object's end, so stream payloads are never gathered.
// Original trivia is retained to keep every nested object's source span exact.
func (c *inputCursor) object() (pdfmodel.Object, error) {
	start := c.pos
	f := objectFrame{cursor: c}
	frameErr := f.value(0)
	object, consumed, parseErr := c.d.parseObject(f.data, 1, start)
	if frameErr != nil {
		if frameErr == io.EOF && parseErr != nil {
			return object, parseErr
		}
		return object, frameErr
	}
	if parseErr != nil {
		return object, parseErr
	}
	c.pos = start + int64(consumed)
	return object, nil
}

type inputFrameError struct {
	pos   int64
	text  string
	cause error
}

func (e *inputFrameError) Error() string { return fmt.Sprintf("PDF input byte %d: %s", e.pos, e.text) }
func (e *inputFrameError) Unwrap() error { return e.cause }

func frameLimit(pos int64, text string) error {
	return &inputFrameError{pos: pos, text: text, cause: pdfmodel.ErrLimit}
}

// objectFrame bounds lexical tokens, nesting and values before allocating the
// full direct-object syntax. The authoritative parser owns value accounting;
// framing failures also pass their prefix through that parser exactly once.
type objectFrame struct {
	cursor *inputCursor
	data   []byte
	values int
}

func (f *objectFrame) malformed(text string) error {
	return &inputFrameError{pos: f.cursor.pos, text: text}
}

func (f *objectFrame) byte(start int64, retain bool) (byte, error) {
	b, err := f.cursor.peek()
	if err != nil {
		return 0, err
	}
	if f.cursor.pos-start >= f.cursor.d.Options.Limits.MaxTokenBytes {
		return 0, frameLimit(start, "token byte limit exceeded")
	}
	f.cursor.pos++
	if retain {
		f.data = append(f.data, b)
	}
	return b, nil
}

func (f *objectFrame) space(retain bool) error {
	for {
		b, err := f.cursor.peek()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if !docSpace(b) && b != '%' {
			return nil
		}
		comment := b == '%'
		start := f.cursor.pos
		for {
			if _, err := f.byte(start, retain); err != nil {
				return err
			}
			b, err = f.cursor.peek()
			if err == io.EOF {
				return nil
			}
			if err != nil {
				return err
			}
			if comment && (b == '\r' || b == '\n') || !comment && !docSpace(b) {
				break
			}
		}
	}
}

func (f *objectFrame) token() (pdfmodel.TokenKind, []byte, error) {
	if err := f.space(true); err != nil {
		return 0, nil, err
	}
	start, index := f.cursor.pos, len(f.data)
	b, err := f.byte(start, true)
	if err != nil {
		return 0, nil, err
	}
	var kind pdfmodel.TokenKind
	switch b {
	case '[':
		kind = pdfmodel.TokenArrayOpen
	case ']':
		kind = pdfmodel.TokenArrayClose
	case '<', '>':
		next, err := f.cursor.peek()
		if err != nil && err != io.EOF {
			return 0, nil, err
		}
		if err == nil && next == b {
			if _, err := f.byte(start, true); err != nil {
				return 0, nil, err
			}
			kind = pdfmodel.TokenDictOpen
			if b == '>' {
				kind = pdfmodel.TokenDictClose
			}
		} else if b == '>' {
			return 0, nil, f.malformed("unmatched hexadecimal string terminator")
		} else {
			kind = pdfmodel.TokenHexString
			for {
				next, err = f.byte(start, true)
				if err != nil {
					return 0, nil, err
				}
				if next == '>' {
					break
				}
				if !docSpace(next) && !inputHex(next) {
					return 0, nil, f.malformed("invalid hexadecimal string digit")
				}
			}
		}
	case '(':
		kind = pdfmodel.TokenLiteralString
		for depth := 1; depth > 0; {
			next, err := f.byte(start, true)
			if err != nil {
				return 0, nil, err
			}
			switch next {
			case '\\':
				if _, err := f.byte(start, true); err != nil {
					return 0, nil, err
				}
			case '(':
				depth++
			case ')':
				depth--
			}
		}
	case ')', '{', '}':
		return 0, nil, f.malformed("unexpected delimiter")
	default:
		for {
			next, err := f.cursor.peek()
			if err == io.EOF || err == nil && docDelimiter(next) {
				break
			}
			if err != nil {
				return 0, nil, err
			}
			if _, err := f.byte(start, true); err != nil {
				return 0, nil, err
			}
		}
		if b == '/' {
			kind = pdfmodel.TokenName
			raw := f.data[index:]
			for i := 1; i < len(raw); i++ {
				if raw[i] != '#' {
					continue
				}
				if len(raw)-i < 3 || !inputHex(raw[i+1]) || !inputHex(raw[i+2]) || raw[i+1] == '0' && raw[i+2] == '0' {
					return 0, nil, f.malformed("invalid name escape")
				}
				i += 2
			}
		} else {
			kind = inputNumberKind(f.data[index:])
		}
	}
	return kind, f.data[index:], nil
}

func (f *objectFrame) value(depth int) error {
	kind, raw, err := f.token()
	if err != nil {
		return err
	}
	if f.values >= f.cursor.d.Options.Limits.MaxValues-f.cursor.d.parsedValues {
		return frameLimit(f.cursor.pos-int64(len(raw)), "object value count limit exceeded")
	}
	f.values++
	switch kind {
	case pdfmodel.TokenInteger:
		return f.reference()
	case pdfmodel.TokenName, pdfmodel.TokenLiteralString, pdfmodel.TokenHexString, pdfmodel.TokenReal:
		return nil
	case pdfmodel.TokenKeyword:
		if string(raw) != "true" && string(raw) != "false" && string(raw) != "null" {
			return f.malformed("keyword is not a PDF object value")
		}
		return nil
	case pdfmodel.TokenArrayOpen, pdfmodel.TokenDictOpen:
		if depth >= min(f.cursor.d.Options.Limits.MaxDepth, 4096) {
			return frameLimit(f.cursor.pos-int64(len(raw)), "object nesting depth limit exceeded")
		}
		for {
			pos, length := f.cursor.pos, len(f.data)
			next, _, err := f.token()
			if err != nil {
				return err
			}
			if kind == pdfmodel.TokenArrayOpen && next == pdfmodel.TokenArrayClose || kind == pdfmodel.TokenDictOpen && next == pdfmodel.TokenDictClose {
				return nil
			}
			if kind == pdfmodel.TokenDictOpen {
				if next != pdfmodel.TokenName {
					return f.malformed("expected dictionary name key or >>")
				}
			} else {
				f.cursor.pos, f.data = pos, f.data[:length]
			}
			if err := f.value(depth + 1); err != nil {
				return err
			}
		}
	default:
		return f.malformed("unexpected delimiter; expected PDF object")
	}
}

func (f *objectFrame) reference() error {
	pos, length := f.cursor.pos, len(f.data)
	matched := false
	defer func() {
		if !matched {
			f.cursor.pos, f.data = pos, f.data[:length]
		}
	}()
	for component := 0; component < 2; component++ {
		if err := f.space(true); err != nil {
			return speculativeInputError(err)
		}
		b, err := f.cursor.peek()
		if err != nil {
			return speculativeInputError(err)
		}
		if component == 0 && b != '+' && b != '-' && (b < '0' || b > '9') || component == 1 && b != 'R' {
			return nil
		}
		kind, raw, err := f.token()
		if err != nil {
			return speculativeInputError(err)
		}
		if component == 0 && kind != pdfmodel.TokenInteger || component == 1 && (kind != pdfmodel.TokenKeyword || string(raw) != "R") {
			return nil
		}
	}
	matched = true
	return nil
}

func speculativeInputError(err error) error {
	var syntaxErr *inputFrameError
	if err == io.EOF || errors.As(err, &syntaxErr) {
		return nil
	}
	return err
}

func inputHex(b byte) bool {
	return b >= '0' && b <= '9' || b >= 'a' && b <= 'f' || b >= 'A' && b <= 'F'
}

func inputNumberKind(raw []byte) pdfmodel.TokenKind {
	if len(raw) > 0 && (raw[0] == '+' || raw[0] == '-') {
		raw = raw[1:]
	}
	digits, dots := 0, 0
	for _, b := range raw {
		switch {
		case b >= '0' && b <= '9':
			digits++
		case b == '.':
			dots++
		default:
			return pdfmodel.TokenKeyword
		}
	}
	if digits == 0 || dots > 1 {
		return pdfmodel.TokenKeyword
	}
	if dots == 1 {
		return pdfmodel.TokenReal
	}
	return pdfmodel.TokenInteger
}
