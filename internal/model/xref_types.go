package model

// XRefEntry is one cross-reference entry: what the xref says about a single
// object number.
//
// The interface is sealed by an unexported method, so the four cases below are
// exhaustive. UnknownXRefEntry exists so that an unrecognised entry type can
// still be represented rather than aborting the read.
type XRefEntry interface {
	pdfXRefEntry()
}

// FreeEntry marks an object number as unused. Free entries form a linked list
// through NextFree, which a writer uses to recycle numbers.
type FreeEntry struct {
	NextFree   uint32
	Generation uint16
}

// InUseEntry locates an object written directly in the file.
//
// Generation must match the generation being requested. A mismatch means the
// xref and the file disagree, which is a sign of damage rather than something
// to resolve by preferring one over the other.
type InUseEntry struct {
	Offset     int64
	Generation uint16
}

// CompressedEntry locates an object stored inside an object stream. Such an
// object has no file offset of its own and its generation is always zero.
type CompressedEntry struct {
	StreamNumber uint32
	Index        uint32
}

// UnknownXRefEntry represents an entry type this reader does not recognise.
//
// The specification requires readers to ignore unknown types rather than fail,
// so the type number is kept and the original bytes remain reachable through
// XRefRecord.Span.
type UnknownXRefEntry struct {
	Type uint64
}

// XRefRecord is one entry together with the object number it describes and the
// bytes it was read from.
type XRefRecord struct {
	Number uint32

	// Span locates the entry. For a cross-reference stream this addresses
	// the stream's decoded source, not the file.
	Span  Span
	Entry XRefEntry
}

// XRefRange is a run of consecutive object numbers starting at First.
//
// The grouping is preserved rather than flattened because it is part of the
// file's structure: a cross-reference table is written as subsections, and an
// incremental update usually contains only the few ranges it touched.
type XRefRange struct {
	First uint32
	Count uint32

	// Header spans the "first count" subsection header of a textual table,
	// and is nil for a cross-reference stream, which has no such header.
	Header  *Span
	Records []XRefRecord
}

// XRefForm distinguishes the two ways cross-reference data is stored. Both may
// appear in one file: a hybrid-reference file carries a table for old readers
// and a stream for new ones.
type XRefForm uint8

const (
	XRefTable  XRefForm = iota + 1 // classic "xref" keyword followed by text entries
	XRefStream                     // /Type /XRef stream with binary entries
)

// SectionID numbers the cross-reference sections in the order they were read,
// which is newest first, since reading starts at startxref and walks /Prev
// backwards through the file's history.
type SectionID uint32

// XRefSection is one cross-reference section and its trailer.
type XRefSection struct {
	ID     SectionID
	Form   XRefForm
	Offset int64 // start of the xref keyword or the xref stream object, in the file
	Span   Span
	Ranges []XRefRange

	// Trailer is the table's trailer dictionary, or the cross-reference
	// stream's own dictionary, which serves the same purpose.
	Trailer Object

	// Prev is the validated /Prev offset, or nil when this is the oldest
	// section. The value as written stays in Trailer, so a caller can still
	// see what the file claimed if validation changed the interpretation.
	Prev *int64

	// XRefStm is the validated /XRefStm offset of a hybrid-reference file,
	// linking the textual table to the stream that supplements it.
	XRefStm *int64

	// StreamID identifies the object holding a cross-reference stream.
	StreamID *ObjectID
}

func (FreeEntry) pdfXRefEntry()        {}
func (InUseEntry) pdfXRefEntry()       {}
func (CompressedEntry) pdfXRefEntry()  {}
func (UnknownXRefEntry) pdfXRefEntry() {}
