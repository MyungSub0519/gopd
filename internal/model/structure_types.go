package model

// Version is a PDF version number, such as 1.7 or 2.0.
type Version struct {
	Major uint8
	Minor uint8
}

// Header is the "%PDF-n.m" marker at the start of the file.
type Header struct {
	Span Span

	// Version is the version claimed by the header. The catalog may carry a
	// later /Version that overrides it, so this is what the file says here,
	// not necessarily the version that governs the document.
	Version Version
}

// FileTail is one "startxref / offset / %%EOF" trailer at the end of a file
// revision. A file updated incrementally has several, one per revision.
type FileTail struct {
	StartXRef Span
	Offset    int64
	OffsetRaw Span // the offset digits as written, for provenance
	EOFMarker Span
}

// RegionKind classifies a stretch of the file. Regions let a caller see how
// much of the input has been accounted for, and which bytes belong to nothing
// the reader recognised.
type RegionKind uint8

const (
	RegionUnknown RegionKind = iota
	RegionHeader
	RegionTrivia
	RegionIndirectObject
	RegionXRef
	RegionTrailer
	RegionFileTail
)

// FileRegion is one classified stretch of the file.
type FileRegion struct {
	Kind RegionKind
	Span Span
}

// Severity ranks a Diagnostic.
type Severity uint8

const (
	SeverityInfo Severity = iota + 1
	SeverityWarning
	SeverityError
)

// Diagnostic is one finding about the input.
//
// A diagnostic is how this library reports something it understood but could
// not fully honour, as opposed to an error, which ends the operation. Content
// that cannot be interpreted is generally diagnosed rather than dropped, so
// that a partial result stays usable and the gap stays visible.
type Diagnostic struct {
	Severity Severity

	// Code is a stable machine-readable identifier such as
	// "unsupported-content-effect". Message is for humans and may change.
	Code    string
	Message string

	Span Span

	// Related holds additional locations that help explain the finding, for
	// example the resource a failing operator referred to.
	Related []Span
}

// Limits bounds the work a single read may perform.
//
// PDF is a format where a small file can legitimately ask for an unbounded
// amount of work: object references can form cycles, containers can nest
// arbitrarily, and a compressed stream can expand without limit. These caps
// are what keep a malformed or hostile input from exhausting memory or time.
// A zero field means "use the default"; see structure.ReadOptions.
type Limits struct {
	MaxDepth        int   // container nesting and reference-chain depth
	MaxTokenBytes   int64 // largest single token
	MaxObjects      int   // parsed objects, page-tree visits and content operations
	MaxXRefSections int   // cross-reference sections followed through /Prev
	MaxDecodedBytes int64 // total decoded stream output for the whole session
}

// Structure is what the reader has learned about the file's physical layout,
// as opposed to the document's meaning.
//
// It is filled in progressively: a Document resolves objects lazily, so
// Objects and Regions grow as more of the file is touched, and Diagnostics
// accumulates alongside them.
type Structure struct {
	File        SourceID
	Header      *Header
	Tails       []FileTail
	XRefs       []XRefSection
	Objects     []IndirectObject // occurrences parsed so far, not a complete index
	Regions     []FileRegion
	Diagnostics []Diagnostic
}
