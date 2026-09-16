package content

import (
	"io"

	"github.com/MyungSub0519/gopd/internal/structure"
)

// These mirror the public gopd entry points so that layer tests exercise the
// same composition the root package performs, including its partial-result
// behaviour on error.

type ReadOptions = structure.ReadOptions

func Parse(r io.ReaderAt, size int64, options ...ReadOptions) (*structure.Document, error) {
	return structure.Parse(r, size, options...)
}

func Read(r io.ReaderAt, size int64) (*DetailedPDF, error) {
	d, err := Parse(r, size)
	if err != nil {
		return nil, err
	}
	return BuildPDF(d)
}

func Open(path string) (*DetailedPDF, error) {
	d, err := structure.ParseFile(path)
	if err != nil {
		return nil, err
	}
	return BuildPDF(d)
}

func ParsePDF(path string) (*PDF, error) {
	detail, err := Open(path)
	return BasicPDF(detail), err
}
