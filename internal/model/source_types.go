package model

import "io"

// SourceID identifies one byte source within a Document.
//
// A PDF is not a single flat byte range. The file itself is one source, and
// every decoded stream payload becomes another, because an object can live
// inside a compressed object stream and its offsets are then relative to that
// decoded output rather than to the file. Tagging every offset with the source
// it belongs to is what keeps those two coordinate systems from being mixed up.
//
// Source 1 is always the original file. Zero means "no source" and is used by
// the zero values of Position and Span to mean "no location".
type SourceID uint32

// Position is a single byte location. Offset is always in bytes, never in
// characters or tokens.
type Position struct {
	Source SourceID
	Offset int64
}

// Span is a half-open byte range [Start, End): Start is included and End is
// excluded, so End-Start is the length and an empty range has Start == End.
type Span struct {
	Source SourceID
	Start  int64
	End    int64
}

// Source is one addressable byte range, either the input file or the decoded
// output of a stream.
type Source struct {
	ID     SourceID
	Reader io.ReaderAt
	Size   int64

	// Origin is nil for the original file, and otherwise records how this
	// source was produced, so that decoded bytes can always be traced back
	// to the encoded bytes they came from.
	Origin *Derivation
}

// Derivation records that a source was produced by transforming part of
// another source.
type Derivation struct {
	// Input is the range in the parent source that was transformed.
	Input Span

	// Steps are the transformations applied, in order.
	Steps []Transform
}

// TransformKind distinguishes the two ways stream bytes are rewritten on the
// way to their decoded form.
type TransformKind uint8

const (
	TransformFilter  TransformKind = iota + 1 // a /Filter entry, such as FlateDecode
	TransformDecrypt                          // decryption, which precedes filtering
)

// Transform is one step in a Derivation.
type Transform struct {
	Kind TransformKind
	Name Name

	// Params is the step's parameter dictionary, or nil when there is none.
	// A nil pointer means absent, which is distinct from a pointer to a PDF
	// null; producers write an explicit null often enough that collapsing
	// the two would lose information.
	Params *Object
}
