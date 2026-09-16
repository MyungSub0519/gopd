package model

// Value is any PDF object value.
//
// The interface is sealed by an unexported method: only this package can add a
// case, so a type switch over Value is exhaustive and callers cannot introduce
// a value the rest of the library would not recognise.
type Value interface {
	pdfValue()
}

// Object is a value together with the exact bytes it was read from.
//
// Every object carries its Span, which is what lets a caller go from a parsed
// value back to the original syntax, and lets diagnostics point at a byte
// offset rather than a description.
type Object struct {
	Span  Span
	Value Value
}

// Null is the PDF null object. It is distinct from a missing dictionary key
// and from InvalidValue; see InvalidValue on why the difference matters.
type Null struct{}

// Boolean is the PDF true or false object.
type Boolean bool

// Integer and Real hold a number in the decimal form it was written in, after
// its syntax has been validated.
//
// The text is kept rather than converted, for two reasons. Converting to
// float64 loses the distinction between an integer and a real written as an
// integer, which the specification relies on for object numbers, /Length and
// array indices. It also silently rounds values that a producer wrote exactly.
// Use Int or Number to convert at the point of use.
type Integer string

// Real is a PDF real number; see Integer for why the written form is kept.
type Real string

// Name is a PDF name with the leading solidus removed and #xx escapes decoded.
//
// The result is a byte string, not text. The specification does not require a
// name to be valid UTF-8, and names in damaged or unusual files often are not,
// so callers must not assume they can be printed as-is.
type Name string

// StringForm records which of the two string syntaxes was used. It is kept so
// that a caller rewriting or auditing a file can reproduce the original form.
type StringForm uint8

const (
	StringLiteral StringForm = iota + 1 // (...)
	StringHex                           // <...>
)

// PDFString is a PDF string object.
type PDFString struct {
	Form StringForm

	// Bytes has had syntax-level escapes and hex digits decoded, but is
	// otherwise raw: it has not been decrypted and no character encoding has
	// been applied. Interpreting these bytes as text requires knowing the
	// context the string appeared in.
	Bytes []byte
}

// Array is a PDF array. Items keep their original order and spans.
type Array struct {
	Items []Object
}

// DictionaryEntry is one key/value pair, with the key's own span retained so
// that a diagnostic can point at the key rather than the whole entry.
type DictionaryEntry struct {
	Key     Name
	KeySpan Span
	Value   Object
}

// Dictionary is a PDF dictionary.
//
// Entries is an ordered slice rather than a map, because a damaged file can
// contain the same key twice and a map would silently discard one of them.
// Dictionary.Get rejects such a duplicate instead of guessing; GetAll exposes
// every occurrence.
type Dictionary struct {
	Entries []DictionaryEntry
}

// ObjectID identifies an indirect object. The pair is the identity: the same
// number with a different generation is a different object.
type ObjectID struct {
	Number     uint32
	Generation uint16
}

// Reference is an indirect reference ("n g R"). Resolving it requires the
// Document that owns the file; see Document.Resolve.
type Reference struct {
	ID ObjectID
}

// InvalidValue marks syntax that could not be parsed.
//
// It is an analyser-only value with no counterpart in the specification, and
// it is deliberately distinct from Null: a file that says null and a file that
// is corrupt at that point are different findings, and collapsing them would
// make damage indistinguishable from intent.
type InvalidValue struct {
	Reason string
}

func (Null) pdfValue()         {}
func (Boolean) pdfValue()      {}
func (Integer) pdfValue()      {}
func (Real) pdfValue()         {}
func (Name) pdfValue()         {}
func (PDFString) pdfValue()    {}
func (Array) pdfValue()        {}
func (Dictionary) pdfValue()   {}
func (Reference) pdfValue()    {}
func (InvalidValue) pdfValue() {}
