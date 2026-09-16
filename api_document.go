package gopd

import (
	"io"

	"github.com/MyungSub0519/gopd/internal/structure"
)

// ParseFile snapshots a file and prepares low-level object access. It closes
// the file before returning. At most one ReadOptions value may be supplied;
// a structural error can return a partial Document together with the error.
func ParseFile(path string, options ...ReadOptions) (*Document, error) {
	return structure.ParseFile(path, options...)
}

// Parse snapshots r; it never closes a caller-owned ReaderAt.
func Parse(r io.ReaderAt, size int64, options ...ReadOptions) (*Document, error) {
	return structure.Parse(r, size, options...)
}
