package syntax

import (
	"github.com/MyungSub0519/gopd/internal/model"
)

// Scanner walks a byte range one token at a time.
//
// Lex returns every token at once, which suits a caller that knows the whole
// range is PDF syntax. A content stream is not such a range: an inline image
// embeds raw bytes that must be skipped rather than tokenised, and where they
// end can only be worked out from the tokens before them. Scanning
// incrementally is what lets a caller stop, consume those bytes itself, and
// resume.
type Scanner struct {
	inner  syntaxScanner
	limits model.Limits
}

// NewScanner returns a Scanner over data, reporting positions relative to
// offset within source. Zero-valued limits take the syntax defaults.
func NewScanner(data []byte, source model.SourceID, offset int64, limits model.Limits) (*Scanner, error) {
	if limits.MaxTokenBytes == 0 {
		limits.MaxTokenBytes = defaultSyntaxTokenBytes
	}
	inner, err := newSyntaxScanner(data, source, offset, limits.MaxTokenBytes)
	if err != nil {
		return nil, err
	}
	return &Scanner{inner: inner, limits: limits}, nil
}

// Next returns the next token, including whitespace and comments. At the end of
// the input it returns TokenEOF indefinitely rather than an error.
func (s *Scanner) Next() (model.Token, error) { return s.inner.next() }

// NextNonTrivia returns the next token that is not whitespace or a comment.
func (s *Scanner) NextNonTrivia() (model.Token, error) { return s.inner.nextNonTrivia() }

// Pos is the scanner's position as an index into the data it was given, which
// is what Seek expects.
func (s *Scanner) Pos() int { return s.inner.pos }

// Offset is where the scanned data begins within its source, so that
// Offset+Pos is an absolute position.
func (s *Scanner) Offset() int64 { return s.inner.offset }

// Data returns the bytes being scanned. The result aliases the scanner's
// input and must not be modified.
func (s *Scanner) Data() []byte { return s.inner.data }

// Seek moves the scanner to an index into the data, so that a caller can skip
// over bytes that are not PDF syntax.
func (s *Scanner) Seek(pos int) error {
	if pos < 0 || pos > len(s.inner.data) {
		return s.inner.errorAt(0, "scanner position outside input")
	}
	s.inner.pos = pos
	return nil
}

// ParseObjectAt parses one object starting at an index into the data, and
// leaves the scanner positioned just after it.
//
// It exists because an operand may be a whole array or dictionary, which the
// object parser understands and the scanner alone does not.
func (s *Scanner) ParseObjectAt(pos int) (model.Object, error) {
	if pos < 0 || pos > len(s.inner.data) {
		return model.Object{}, s.inner.errorAt(0, "scanner position outside input")
	}
	object, consumed, err := ParseObjectWithLimits(s.inner.data[pos:], s.inner.source, s.inner.offset+int64(pos), s.limits)
	if err != nil {
		return object, err
	}
	if consumed <= 0 {
		return object, s.inner.errorAt(pos, "object parser made no progress")
	}
	s.inner.pos = pos + consumed
	return object, nil
}

// IsWhitespace reports whether c is one of the six bytes PDF treats as
// whitespace, NUL included.
func IsWhitespace(c byte) bool { return isPDFWhitespace(c) }

// IsDelimiter reports whether c is one of the self-delimiting characters that
// end a bare word.
func IsDelimiter(c byte) bool { return isPDFDelimiter(c) }
