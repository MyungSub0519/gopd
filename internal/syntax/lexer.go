package syntax

import (
	"fmt"
	"math"

	"github.com/MyungSub0519/gopd/internal/pdfmodel"
)

// Lex scans one complete byte range, preserving whitespace and comments as
// tokens and appending an EOF token. Spans use offset as the range's absolute
// position in source. Tokens are limited to 16 MiB each and the result to
// 1,048,576 non-EOF tokens. On error the valid token prefix is returned.
// Stream payloads must be excluded by the caller; they are not PDF syntax.
func Lex(data []byte, source pdfmodel.SourceID, offset int64) ([]pdfmodel.Token, error) {
	s, err := newSyntaxScanner(data, source, offset, defaultSyntaxTokenBytes)
	if err != nil {
		return nil, err
	}
	var tokens []pdfmodel.Token
	for {
		token, err := s.next()
		if err != nil {
			return tokens, err
		}
		if len(tokens) >= maxSyntaxTokens && token.Kind != pdfmodel.TokenEOF {
			return tokens, s.limitAt(int(token.Span.Start-offset), "token count limit exceeded")
		}
		tokens = append(tokens, token)
		if token.Kind == pdfmodel.TokenEOF {
			return tokens, nil
		}
	}
}

const (
	defaultSyntaxDepth      = 256
	defaultSyntaxTokenBytes = 16 << 20
	maxSyntaxTokens         = 1 << 20
	maxSyntaxObjects        = 1 << 20
)

type syntaxError struct {
	Position pdfmodel.Position
	Message  string
	cause    error
}

func (e *syntaxError) Unwrap() error { return e.cause }

func (e *syntaxError) Error() string {
	return fmt.Sprintf("gopd: source %d byte %d: %s", e.Position.Source, e.Position.Offset, e.Message)
}

type syntaxScanner struct {
	data          []byte
	source        pdfmodel.SourceID
	offset        int64
	pos           int
	maxTokenBytes int64
	// Optional sequential token budget. Value fields keep speculative reference
	// lookahead from charging tokens unless the lookahead is committed.
	maxTokens int
	tokens    int
}

func newSyntaxScanner(data []byte, source pdfmodel.SourceID, offset, maxTokenBytes int64) (syntaxScanner, error) {
	s := syntaxScanner{data: data, source: source, offset: offset, maxTokenBytes: maxTokenBytes}
	if offset < 0 || int64(len(data)) > math.MaxInt64-offset {
		return s, s.errorAt(0, "invalid source offset or source range overflow")
	}
	if maxTokenBytes <= 0 {
		return s, s.errorAt(0, "token byte limit must be positive")
	}
	return s, nil
}

func (s *syntaxScanner) errorAt(pos int, message string) error {
	return &syntaxError{Position: pdfmodel.Position{Source: s.source, Offset: s.offset + int64(pos)}, Message: message}
}

func (s *syntaxScanner) limitAt(pos int, message string) error {
	return &syntaxError{Position: pdfmodel.Position{Source: s.source, Offset: s.offset + int64(pos)}, Message: message, cause: pdfmodel.ErrLimit}
}

func (s *syntaxScanner) token(kind pdfmodel.TokenKind, start int) (pdfmodel.Token, error) {
	if int64(s.pos-start) > s.maxTokenBytes {
		return pdfmodel.Token{}, s.limitAt(start, "token byte limit exceeded")
	}
	if s.maxTokens > 0 && kind != pdfmodel.TokenEOF {
		if s.tokens >= s.maxTokens {
			return pdfmodel.Token{}, s.limitAt(start, "token count limit exceeded")
		}
		s.tokens++
	}
	return pdfmodel.Token{Kind: kind, Span: pdfmodel.Span{Source: s.source, Start: s.offset + int64(start), End: s.offset + int64(s.pos)}}, nil
}

func (s *syntaxScanner) checkSize(start int) error {
	if int64(s.pos-start) > s.maxTokenBytes {
		return s.limitAt(start, "token byte limit exceeded")
	}
	return nil
}

func (s *syntaxScanner) nextNonTrivia() (pdfmodel.Token, error) {
	for {
		token, err := s.next()
		if err != nil || (token.Kind != pdfmodel.TokenWhitespace && token.Kind != pdfmodel.TokenComment) {
			return token, err
		}
	}
}

func (s *syntaxScanner) next() (pdfmodel.Token, error) {
	start := s.pos
	if s.pos == len(s.data) {
		return s.token(pdfmodel.TokenEOF, start)
	}
	c := s.data[s.pos]
	if isPDFWhitespace(c) {
		for s.pos < len(s.data) && isPDFWhitespace(s.data[s.pos]) {
			s.pos++
			if err := s.checkSize(start); err != nil {
				return pdfmodel.Token{}, err
			}
		}
		return s.token(pdfmodel.TokenWhitespace, start)
	}
	s.pos++
	switch c {
	case '%':
		for s.pos < len(s.data) && s.data[s.pos] != '\r' && s.data[s.pos] != '\n' {
			s.pos++
			if err := s.checkSize(start); err != nil {
				return pdfmodel.Token{}, err
			}
		}
		return s.token(pdfmodel.TokenComment, start)
	case '[':
		return s.token(pdfmodel.TokenArrayOpen, start)
	case ']':
		return s.token(pdfmodel.TokenArrayClose, start)
	case '<':
		if s.pos < len(s.data) && s.data[s.pos] == '<' {
			s.pos++
			return s.token(pdfmodel.TokenDictOpen, start)
		}
		for s.pos < len(s.data) {
			c = s.data[s.pos]
			s.pos++
			if err := s.checkSize(start); err != nil {
				return pdfmodel.Token{}, err
			}
			if c == '>' {
				return s.token(pdfmodel.TokenHexString, start)
			}
			if !isPDFWhitespace(c) && hexNibble(c) < 0 {
				return pdfmodel.Token{}, s.errorAt(s.pos-1, "invalid hexadecimal string digit")
			}
		}
		return pdfmodel.Token{}, s.errorAt(start, "unterminated hexadecimal string")
	case '>':
		if s.pos < len(s.data) && s.data[s.pos] == '>' {
			s.pos++
			return s.token(pdfmodel.TokenDictClose, start)
		}
		return pdfmodel.Token{}, s.errorAt(start, "unmatched hexadecimal string terminator")
	case '(':
		depth := 1
		for s.pos < len(s.data) {
			c = s.data[s.pos]
			s.pos++
			if err := s.checkSize(start); err != nil {
				return pdfmodel.Token{}, err
			}
			switch c {
			case '\\':
				if s.pos == len(s.data) {
					return pdfmodel.Token{}, s.errorAt(start, "unterminated literal string escape")
				}
				s.pos++ // Escaped parentheses do not change the nesting level.
			case '(':
				depth++
			case ')':
				depth--
				if depth == 0 {
					return s.token(pdfmodel.TokenLiteralString, start)
				}
			}
		}
		return pdfmodel.Token{}, s.errorAt(start, "unterminated literal string")
	case '/':
		for s.pos < len(s.data) && !isPDFWhitespace(s.data[s.pos]) && !isPDFDelimiter(s.data[s.pos]) {
			if s.data[s.pos] == '#' {
				if len(s.data)-s.pos < 3 || hexNibble(s.data[s.pos+1]) < 0 || hexNibble(s.data[s.pos+2]) < 0 {
					return pdfmodel.Token{}, s.errorAt(s.pos, "invalid name escape; expected two hexadecimal digits")
				}
				if s.data[s.pos+1] == '0' && s.data[s.pos+2] == '0' {
					return pdfmodel.Token{}, s.errorAt(s.pos, "NUL byte is not allowed in a PDF name")
				}
				s.pos += 3
			} else {
				s.pos++
			}
			if err := s.checkSize(start); err != nil {
				return pdfmodel.Token{}, err
			}
		}
		return s.token(pdfmodel.TokenName, start)
	case ')', '{', '}':
		return pdfmodel.Token{}, s.errorAt(start, "unexpected delimiter")
	default:
		for s.pos < len(s.data) && !isPDFWhitespace(s.data[s.pos]) && !isPDFDelimiter(s.data[s.pos]) {
			s.pos++
			if err := s.checkSize(start); err != nil {
				return pdfmodel.Token{}, err
			}
		}
		return s.token(numberTokenKind(s.data[start:s.pos]), start)
	}
}

func isPDFWhitespace(c byte) bool {
	return c == 0 || c == '\t' || c == '\n' || c == '\f' || c == '\r' || c == ' '
}

func isPDFDelimiter(c byte) bool {
	switch c {
	case '(', ')', '<', '>', '[', ']', '{', '}', '/', '%':
		return true
	}
	return false
}

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

func numberTokenKind(raw []byte) pdfmodel.TokenKind {
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
