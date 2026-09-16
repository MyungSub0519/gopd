package syntax

import (
	"github.com/MyungSub0519/gopd/internal/model"
)

// Lex scans one complete byte range, preserving whitespace and comments as
// tokens and appending an EOF token. On error the valid token prefix is
// returned. Stream payloads must be excluded by the caller.
func Lex(data []byte, source model.SourceID, offset int64) ([]model.Token, error) {
	s, err := newSyntaxScanner(data, source, offset, defaultSyntaxTokenBytes)
	if err != nil {
		return nil, err
	}
	var tokens []model.Token
	for {
		token, err := s.next()
		if err != nil {
			return tokens, err
		}
		if len(tokens) >= maxSyntaxTokens && token.Kind != model.TokenEOF {
			return tokens, s.errorAt(int(token.Span.Start-offset), "token count limit exceeded")
		}
		tokens = append(tokens, token)
		if token.Kind == model.TokenEOF {
			return tokens, nil
		}
	}
}

// ParseObject parses one direct PDF object or indirect reference using the
// default limits.
func ParseObject(data []byte, source model.SourceID, offset int64) (model.Object, int, error) {
	return ParseObjectWithLimits(data, source, offset, model.Limits{})
}
