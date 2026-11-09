// Package gopd parses PDF content and provides access to its underlying structure.
//
// Start with ParseFile or ParseReader. ParseOptions.Content selects text,
// graphics, images and annotations; Detail selects Basic, Layout or Full output.
// Both entry points return Result, with Result.Details available at DetailFull.
// Full describes retained information, not complete rendering or PDF conformance.
// Always check errors and diagnostics, including when a partial result is returned.
// Close Full results and legacy results when finished. Reader inputs must remain
// open and unchanged until their retained Document is closed.
// Low-level Document and syntax helpers remain available for object/byte access.
package gopd

import (
	"io"

	"github.com/MyungSub0519/gopd/internal/document"
	"github.com/MyungSub0519/gopd/internal/parser"
	"github.com/MyungSub0519/gopd/internal/pdfmodel"
	"github.com/MyungSub0519/gopd/internal/syntax"
)

// ParsePDF reads a file and returns basic text and graphics grouped by page.
// It retains eager detailed analysis for Details; it is not selective parsing.
// A semantic failure can return both a partial PDF and an error. Callers must
// check err and close any non-nil result, including partial results.
//
// Deprecated: Use ParseFile with Content and Detail. The Result layout differs
// from PDF; DetailFull retains the detailed view through Result.Details().
func ParsePDF(path string) (*PDF, error) {
	return parser.ParsePDF(path)
}

// Open returns detailed contents, resources, operations, and byte provenance.
// Close any non-nil result to release its input file, including partial results.
//
// Deprecated: Use ParseFile with ContentAll and DetailFull, then Result.Details().
func Open(path string) (*DetailedPDF, error) {
	return parser.Open(path)
}

// Read parses r through a bounded buffer and returns detailed page contents.
// Keep r open and unchanged until closing the result. Close any non-nil result,
// including partial results. Read never closes the caller-owned reader.
//
// Deprecated: Use ParseReader with ContentAll and DetailFull, then Result.Details().
func Read(r io.ReaderAt, size int64) (*DetailedPDF, error) {
	return parser.Read(r, size)
}

// BuildPDF interprets page contents. Invalid syntax and traversal limits return
// an error and any partial result. Unsupported effects are retained as operations
// and diagnostics. No pixel rendering, OCR, or reading-order reconstruction occurs.
// The result shares d; closing either closes their shared source access.
func BuildPDF(d *Document) (*DetailedPDF, error) {
	return parser.BuildPDF(d)
}

// LoadDocument opens a file and prepares buffered low-level object access.
// Close the returned Document when finished. Keep the file unchanged until then.
// At most one ReadOptions value may be supplied. A structural error may return
// a partial Document; close every non-nil Document, including on error.
func LoadDocument(path string, options ...ReadOptions) (*Document, error) {
	return document.Load(path, options...)
}

// ReadDocument borrows r through a bounded buffer and never closes it. Keep r
// open and unchanged until Document.Close, including for partial Documents
// returned with errors. At most one ReadOptions value may be supplied.
func ReadDocument(r io.ReaderAt, size int64, options ...ReadOptions) (*Document, error) {
	return document.Read(r, size, options...)
}

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

// Int converts a direct PDF Integer into int64, rejecting other types and
// out-of-range values. Resolve indirect references before calling Int.
func Int(object Object) (int64, error) {
	return pdfmodel.Int(object)
}

// Number converts a direct Integer or Real into a finite float64. It does not
// resolve indirect references or preserve exact decimal precision.
func Number(object Object) (float64, error) {
	return pdfmodel.Number(object)
}

// IsStream reports whether object directly contains a Stream. It does not
// follow references; use Document.ResolveObject first when necessary.
func IsStream(object Object) bool {
	return pdfmodel.IsStream(object)
}

// IdentityMatrix returns a transformation that leaves coordinates unchanged.
func IdentityMatrix() Matrix {
	return pdfmodel.IdentityMatrix()
}

// ParseFile reads a file through a bounded buffer and emits the requested content.
// Zero options return text at DetailBasic. Basic and Layout close the file before
// return; Full retains it until Result.Close. Keep the file unchanged until then.
// A semantic error can accompany a partial Result; check err and close any non-nil
// Full result, including when an error is returned.
func ParseFile(path string, options ParseOptions) (*Result, error) {
	return parser.ParseFile(path, options)
}

// ParseReader reads through a bounded buffer and never closes r. Keep r open
// and unchanged during the call and, at Full, until Result.Close. Close any
// non-nil Full result, including partial results. Content and Detail have the
// same meaning as in ParseFile. Decoded streams and results retain their existing
// memory budgets; this API does not promise bounded total process memory.
func ParseReader(r io.ReaderAt, size int64, options ParseOptions) (*Result, error) {
	return parser.ParseReader(r, size, options)
}
