package syntax

import (
	"fmt"
	"math"

	"github.com/MyungSub0519/gopd/internal/model"
)

// Default bounds applied when a caller supplies no limits of its own. They are
// deliberately generous: they exist to stop a malformed file from consuming
// unbounded memory, not to reject unusual but legal input.
const (
	defaultSyntaxDepth      = 256
	defaultSyntaxTokenBytes = 16 << 20
	maxSyntaxTokens         = 1 << 20
	maxSyntaxObjects        = 1 << 20
)

// syntaxError is a scan or parse failure carrying the exact byte it occurred
// at, so that a caller can point at the input rather than describe it.
type syntaxError struct {
	Position model.Position
	Message  string
}

func (e *syntaxError) Error() string {
	return fmt.Sprintf("gopd: source %d byte %d: %s", e.Position.Source, e.Position.Offset, e.Message)
}

// syntaxScanner turns bytes into tokens.
//
// It scans a byte slice that the caller has already isolated, and reports
// positions in the coordinate space of source: pos is an index into data,
// while offset is where data begins within source. Keeping the two apart is
// what lets the same scanner run over the file and over a decoded stream
// payload and still produce spans that address the right source.
//
// The scanner recognises syntax only. It does not know about objects, streams
// or content operators, and in particular it must never be pointed at a stream
// payload, whose bytes are not PDF syntax.
type syntaxScanner struct {
	data          []byte
	source        model.SourceID
	offset        int64
	pos           int
	maxTokenBytes int64
}

// newSyntaxScanner prepares a scanner over data, validating the bounds that
// the rest of the scanner then assumes.
func newSyntaxScanner(data []byte, source model.SourceID, offset, maxTokenBytes int64) (syntaxScanner, error) {
	s := syntaxScanner{data: data, source: source, offset: offset, maxTokenBytes: maxTokenBytes}
	// Rejecting the overflow here means every later span arithmetic on
	// offset+pos is known to stay inside int64.
	if offset < 0 || int64(len(data)) > math.MaxInt64-offset {
		return s, s.errorAt(0, "invalid source offset or source range overflow")
	}
	if maxTokenBytes <= 0 {
		return s, s.errorAt(0, "token byte limit must be positive")
	}
	return s, nil
}

// errorAt reports a failure at pos, an index into data, translating it into a
// position within source.
func (s *syntaxScanner) errorAt(pos int, message string) error {
	return &syntaxError{Position: model.Position{Source: s.source, Offset: s.offset + int64(pos)}, Message: message}
}

// token closes off the token that began at start and runs to the current
// position.
func (s *syntaxScanner) token(kind model.TokenKind, start int) (model.Token, error) {
	if int64(s.pos-start) > s.maxTokenBytes {
		return model.Token{}, s.errorAt(start, "token byte limit exceeded")
	}
	return model.Token{Kind: kind, Span: model.Span{Source: s.source, Start: s.offset + int64(start), End: s.offset + int64(s.pos)}}, nil
}

// checkSize enforces the token size limit from inside a scanning loop.
//
// Every unbounded loop below calls it on each byte, so that an unterminated
// string or comment fails after maxTokenBytes rather than after consuming the
// rest of the input.
func (s *syntaxScanner) checkSize(start int) error {
	if int64(s.pos-start) > s.maxTokenBytes {
		return s.errorAt(start, "token byte limit exceeded")
	}
	return nil
}

// nextNonTrivia returns the next token that carries meaning, skipping
// whitespace and comments. It is what the parser uses; callers that need exact
// coverage of the input, such as Lex, call next directly.
func (s *syntaxScanner) nextNonTrivia() (model.Token, error) {
	for {
		token, err := s.next()
		if err != nil || (token.Kind != model.TokenWhitespace && token.Kind != model.TokenComment) {
			return token, err
		}
	}
}

// next scans one token, including whitespace and comments, and advances.
//
// At the end of the input it returns TokenEOF indefinitely rather than an
// error, so a caller can loop until EOF without a separate exhaustion check.
func (s *syntaxScanner) next() (model.Token, error) {
	start := s.pos
	if s.pos == len(s.data) {
		return s.token(model.TokenEOF, start)
	}
	c := s.data[s.pos]
	if isPDFWhitespace(c) {
		for s.pos < len(s.data) && isPDFWhitespace(s.data[s.pos]) {
			s.pos++
			if err := s.checkSize(start); err != nil {
				return model.Token{}, err
			}
		}
		return s.token(model.TokenWhitespace, start)
	}
	s.pos++
	switch c {
	case '%':
		// A comment runs to the end of the line. Both CR and LF end it, and
		// the terminator itself belongs to the following whitespace token.
		for s.pos < len(s.data) && s.data[s.pos] != '\r' && s.data[s.pos] != '\n' {
			s.pos++
			if err := s.checkSize(start); err != nil {
				return model.Token{}, err
			}
		}
		return s.token(model.TokenComment, start)
	case '[':
		return s.token(model.TokenArrayOpen, start)
	case ']':
		return s.token(model.TokenArrayClose, start)
	case '<':
		// '<' is ambiguous: doubled it opens a dictionary, alone it opens a
		// hex string. Only the next byte tells them apart.
		if s.pos < len(s.data) && s.data[s.pos] == '<' {
			s.pos++
			return s.token(model.TokenDictOpen, start)
		}
		for s.pos < len(s.data) {
			c = s.data[s.pos]
			s.pos++
			if err := s.checkSize(start); err != nil {
				return model.Token{}, err
			}
			if c == '>' {
				return s.token(model.TokenHexString, start)
			}
			// Whitespace may be sprinkled between hex digits; anything
			// else is not a digit and the string is malformed.
			if !isPDFWhitespace(c) && hexNibble(c) < 0 {
				return model.Token{}, s.errorAt(s.pos-1, "invalid hexadecimal string digit")
			}
		}
		return model.Token{}, s.errorAt(start, "unterminated hexadecimal string")
	case '>':
		if s.pos < len(s.data) && s.data[s.pos] == '>' {
			s.pos++
			return s.token(model.TokenDictClose, start)
		}
		return model.Token{}, s.errorAt(start, "unmatched hexadecimal string terminator")
	case '(':
		// Literal strings nest: an unescaped '(' inside a string opens an
		// inner level that its matching ')' closes, so the token ends only
		// at the parenthesis that brings the depth back to zero.
		depth := 1
		for s.pos < len(s.data) {
			c = s.data[s.pos]
			s.pos++
			if err := s.checkSize(start); err != nil {
				return model.Token{}, err
			}
			switch c {
			case '\\':
				if s.pos == len(s.data) {
					return model.Token{}, s.errorAt(start, "unterminated literal string escape")
				}
				// Skip the escaped byte without inspecting it. That is
				// what keeps an escaped parenthesis from changing depth;
				// the escape sequences themselves are decoded later, by
				// decodeLiteralString.
				s.pos++
			case '(':
				depth++
			case ')':
				depth--
				if depth == 0 {
					return s.token(model.TokenLiteralString, start)
				}
			}
		}
		return model.Token{}, s.errorAt(start, "unterminated literal string")
	case '/':
		// A name runs until whitespace or a delimiter. '#' introduces a
		// two-digit hex escape, which is how a name carries bytes that
		// would otherwise end it.
		for s.pos < len(s.data) && !isPDFWhitespace(s.data[s.pos]) && !isPDFDelimiter(s.data[s.pos]) {
			if s.data[s.pos] == '#' {
				if len(s.data)-s.pos < 3 || hexNibble(s.data[s.pos+1]) < 0 || hexNibble(s.data[s.pos+2]) < 0 {
					return model.Token{}, s.errorAt(s.pos, "invalid name escape; expected two hexadecimal digits")
				}
				// #00 is rejected outright: a NUL inside a name would make
				// the name unusable as a key and is forbidden.
				if s.data[s.pos+1] == '0' && s.data[s.pos+2] == '0' {
					return model.Token{}, s.errorAt(s.pos, "NUL byte is not allowed in a PDF name")
				}
				s.pos += 3
			} else {
				s.pos++
			}
			if err := s.checkSize(start); err != nil {
				return model.Token{}, err
			}
		}
		return s.token(model.TokenName, start)
	case ')', '{', '}':
		// These can only appear as part of a construct handled above, so
		// reaching one here means the input is malformed.
		return model.Token{}, s.errorAt(start, "unexpected delimiter")
	default:
		// Anything else is a run of regular characters: either a number or
		// a bare keyword. numberTokenKind decides which.
		for s.pos < len(s.data) && !isPDFWhitespace(s.data[s.pos]) && !isPDFDelimiter(s.data[s.pos]) {
			s.pos++
			if err := s.checkSize(start); err != nil {
				return model.Token{}, err
			}
		}
		return s.token(numberTokenKind(s.data[start:s.pos]), start)
	}
}

// isPDFWhitespace reports whether c is one of the six whitespace bytes.
//
// NUL counts as whitespace in PDF, which is unusual and easy to miss when
// porting logic from other formats.
func isPDFWhitespace(c byte) bool {
	return c == 0 || c == '\t' || c == '\n' || c == '\f' || c == '\r' || c == ' '
}

// isPDFDelimiter reports whether c ends an unquoted run of characters. These
// bytes are self-delimiting: they need no whitespace before or after them.
func isPDFDelimiter(c byte) bool {
	switch c {
	case '(', ')', '<', '>', '[', ']', '{', '}', '/', '%':
		return true
	}
	return false
}

// hexNibble returns the value of one hexadecimal digit, or -1 if c is not one.
func hexNibble(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	default:
		return -1
	}
}

// numberTokenKind classifies a run of regular characters as an integer, a real
// or a keyword.
//
// PDF numbers are plainer than they look: an optional sign, digits and at most
// one period, in any arrangement, so ".5", "4." and "-.002" are all valid.
// What is not valid is exponent notation, so "1e5" is a keyword here rather
// than a number, which is the intended outcome and not an oversight.
func numberTokenKind(raw []byte) model.TokenKind {
	i := 0
	if len(raw) > 0 && (raw[0] == '+' || raw[0] == '-') {
		i++
	}
	digits, dots := 0, 0
	for ; i < len(raw); i++ {
		switch {
		case raw[i] >= '0' && raw[i] <= '9':
			digits++
		case raw[i] == '.':
			dots++
		default:
			return model.TokenKeyword
		}
	}
	// A sign or a lone period with no digits is not a number, and neither is
	// something like "1.2.3".
	if digits == 0 || dots > 1 {
		return model.TokenKeyword
	}
	if dots == 1 {
		return model.TokenReal
	}
	return model.TokenInteger
}
