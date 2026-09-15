package gopd

// Lex scans one complete byte range, preserving whitespace and comments as
// tokens and appending an EOF token. Spans use offset as the range's absolute
// position in source. Tokens are limited to 16 MiB each and the result to
// 1,048,576 non-EOF tokens. On error the valid token prefix is returned.
// Stream payloads must be excluded by the caller; they are not PDF syntax.
func Lex(data []byte, source SourceID, offset int64) ([]Token, error) {
	s, err := newSyntaxScanner(data, source, offset, defaultSyntaxTokenBytes)
	if err != nil {
		return nil, err
	}
	var tokens []Token
	for {
		token, err := s.next()
		if err != nil {
			return tokens, err
		}
		if len(tokens) >= maxSyntaxTokens && token.Kind != TokenEOF {
			return tokens, s.errorAt(int(token.Span.Start-offset), "token count limit exceeded")
		}
		tokens = append(tokens, token)
		if token.Kind == TokenEOF {
			return tokens, nil
		}
	}
}

// ParseObject parses one direct PDF object or indirect reference. consumed is
// relative to data and includes leading whitespace and comments, but excludes
// trailing trivia. Spans are absolute within source and exclude leading trivia.
// Bare keywords other than true, false and null are not object values.
// Default limits are 256 container levels, 16 MiB per token and 1,048,576 values.
func ParseObject(data []byte, source SourceID, offset int64) (object Object, consumed int, err error) {
	return parseObjectWithLimits(data, source, offset, Limits{})
}
