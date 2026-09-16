package gopd

import (
	"github.com/MyungSub0519/gopd/internal/syntax"
)

// Lex scans one complete byte range, preserving whitespace and comments as
// tokens and appending an EOF token. Spans use offset as the range's absolute
// position in source. Tokens are limited to 16 MiB each and the result to
// 1,048,576 non-EOF tokens. On error the valid token prefix is returned.
// Stream payloads must be excluded by the caller; they are not PDF syntax.
func Lex(data []byte, source SourceID, offset int64) ([]Token, error) {
	return syntax.Lex(data, source, offset)
}

// ParseObject parses one direct PDF object or indirect reference. consumed is
// relative to data and includes leading whitespace and comments, but excludes
// trailing trivia. Spans are absolute within source and exclude leading trivia.
// Bare keywords other than true, false and null are not object values.
// Default limits are 256 container levels, 16 MiB per token and 1,048,576 values.
func ParseObject(data []byte, source SourceID, offset int64) (object Object, consumed int, err error) {
	return syntax.ParseObject(data, source, offset)
}
