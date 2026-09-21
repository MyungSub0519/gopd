package gopd

import (
	"io"

	"github.com/MyungSub0519/gopd/internal/common/document"
	"github.com/MyungSub0519/gopd/internal/common/pdfmodel"
	"github.com/MyungSub0519/gopd/internal/common/syntax"
	"github.com/MyungSub0519/gopd/internal/parser"
)

// ParsePDF snapshots a file and returns basic text and graphics grouped by page.
// It retains eager detailed analysis for Details; it is not selective parsing.
// A semantic failure can return both a partial PDF and an error. Callers must
// check err before treating the result as successful. No Close call is required.
func ParsePDF(path string) (*PDF, error) {
	return parser.ParsePDF(path)
}

// Open returns detailed contents, resources, operations, and byte provenance.
// Use ParsePDF for a basic result. Both functions close the input file.
func Open(path string) (*DetailedPDF, error) {
	return parser.Open(path)
}

// Read parses a bounded snapshot of r and returns detailed page contents.
// The caller retains ownership of r; no Close call is required on the result.
func Read(r io.ReaderAt, size int64) (*DetailedPDF, error) {
	return parser.Read(r, size)
}

// BuildPDF interprets page contents. Invalid syntax and traversal limits return
// an error and any partial result. Unsupported effects are retained as operations
// and diagnostics. No pixel rendering, OCR, or reading-order reconstruction occurs.
func BuildPDF(d *Document) (*DetailedPDF, error) {
	return parser.BuildPDF(d)
}

// ParseFile snapshots a file and prepares low-level object access. It closes
// the file before returning. At most one ReadOptions value may be supplied;
// a structural error can return a partial Document together with the error.
func ParseFile(path string, options ...ReadOptions) (*Document, error) {
	return document.ParseFile(path, options...)
}

// Parse snapshots r; it never closes a caller-owned ReaderAt.
func Parse(r io.ReaderAt, size int64, options ...ReadOptions) (*Document, error) {
	return document.Parse(r, size, options...)
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

// Extract snapshots a file and directly emits selected content. The file is
// closed before return. A semantic error can accompany a partial Extraction;
// callers must check err even when the result is non-nil.
func Extract(path string, options ExtractOptions) (*Extraction, error) {
	return parser.Extract(path, options)
}

// ExtractReader takes one bounded input snapshot and never closes r. Content
// kinds share one interpreter; reused Forms still execute under each caller's
// state. This API does not promise bounded total process memory or streaming I/O.
func ExtractReader(r io.ReaderAt, size int64, options ExtractOptions) (*Extraction, error) {
	return parser.ExtractReader(r, size, options)
}
