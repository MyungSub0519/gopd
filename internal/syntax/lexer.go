package syntax

import (
	"fmt"
	"math"

	"github.com/MyungSub0519/gopd/internal/model"
)

const (
	defaultSyntaxDepth      = 256
	defaultSyntaxTokenBytes = 16 << 20
	maxSyntaxTokens         = 1 << 20
	maxSyntaxObjects        = 1 << 20
)

type syntaxError struct {
	Position model.Position
	Message  string
}

func (e *syntaxError) Error() string {
	return fmt.Sprintf("gopd: source %d byte %d: %s", e.Position.Source, e.Position.Offset, e.Message)
}

type syntaxScanner struct {
	data          []byte
	source        model.SourceID
	offset        int64
	pos           int
	maxTokenBytes int64
}

func newSyntaxScanner(data []byte, source model.SourceID, offset, maxTokenBytes int64) (syntaxScanner, error) {
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
	return &syntaxError{Position: model.Position{Source: s.source, Offset: s.offset + int64(pos)}, Message: message}
}

func (s *syntaxScanner) token(kind model.TokenKind, start int) (model.Token, error) {
	if int64(s.pos-start) > s.maxTokenBytes {
		return model.Token{}, s.errorAt(start, "token byte limit exceeded")
	}
	return model.Token{Kind: kind, Span: model.Span{Source: s.source, Start: s.offset + int64(start), End: s.offset + int64(s.pos)}}, nil
}

func (s *syntaxScanner) checkSize(start int) error {
	if int64(s.pos-start) > s.maxTokenBytes {
		return s.errorAt(start, "token byte limit exceeded")
	}
	return nil
}

func (s *syntaxScanner) nextNonTrivia() (model.Token, error) {
	for {
		token, err := s.next()
		if err != nil || (token.Kind != model.TokenWhitespace && token.Kind != model.TokenComment) {
			return token, err
		}
	}
}

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
				s.pos++ // Escaped parentheses do not change the nesting level.
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
		for s.pos < len(s.data) && !isPDFWhitespace(s.data[s.pos]) && !isPDFDelimiter(s.data[s.pos]) {
			if s.data[s.pos] == '#' {
				if len(s.data)-s.pos < 3 || hexNibble(s.data[s.pos+1]) < 0 || hexNibble(s.data[s.pos+2]) < 0 {
					return model.Token{}, s.errorAt(s.pos, "invalid name escape; expected two hexadecimal digits")
				}
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
		return model.Token{}, s.errorAt(start, "unexpected delimiter")
	default:
		for s.pos < len(s.data) && !isPDFWhitespace(s.data[s.pos]) && !isPDFDelimiter(s.data[s.pos]) {
			s.pos++
			if err := s.checkSize(start); err != nil {
				return model.Token{}, err
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
	if digits == 0 || dots > 1 {
		return model.TokenKeyword
	}
	if dots == 1 {
		return model.TokenReal
	}
	return model.TokenInteger
}
