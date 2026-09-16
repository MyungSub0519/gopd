package model

// TokenKind classifies one lexical unit of PDF syntax.
//
// Whitespace and comments are kinds of their own rather than being discarded,
// because a token stream is also used to map byte ranges back to the original
// file. Numbers are split into TokenInteger and TokenReal at scan time, since
// the two are not interchangeable: an object number, a /Length or an array
// index must be an integer, and only the scanner still knows how the number
// was written.
type TokenKind uint8

const (
	// TokenInvalid is the zero value and never appears in a scan result; a
	// scanner reports malformed input as an error instead.
	TokenInvalid TokenKind = iota

	// TokenEOF terminates every successful scan. Its span is empty and sits
	// at the end of the scanned range.
	TokenEOF

	// Trivia carries no value but is preserved so that spans stay exact.
	TokenWhitespace
	TokenComment

	// Numbers keep their written form; see TokenKind on why they are split.
	TokenInteger
	TokenReal

	// TokenName is a /Name, including the leading solidus.
	TokenName

	// Strings keep their delimiters; the two forms decode differently.
	TokenLiteralString // (...)
	TokenHexString     // <...>

	// Container delimiters are individual tokens; the parser pairs them.
	TokenArrayOpen  // [
	TokenArrayClose // ]
	TokenDictOpen   // <<
	TokenDictClose  // >>

	// TokenKeyword is a bare word: obj, endobj, stream, R, true, false,
	// null, or a content-stream operator such as Tj or re. The scanner does
	// not judge whether the word is meaningful in context.
	TokenKeyword
)

// Token is one lexical unit together with its exact location. Token carries no
// decoded value: callers read Span from the source to obtain the raw bytes, so
// that a token never duplicates or reinterprets the input.
type Token struct {
	Kind TokenKind
	Span Span
}
