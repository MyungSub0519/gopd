package gopd

type TokenKind uint8

const (
	TokenInvalid TokenKind = iota
	TokenEOF
	TokenWhitespace
	TokenComment
	TokenInteger
	TokenReal
	TokenName
	TokenLiteralString
	TokenHexString
	TokenArrayOpen
	TokenArrayClose
	TokenDictOpen
	TokenDictClose
	TokenKeyword
)

type Token struct {
	Kind TokenKind
	Span Span
}
