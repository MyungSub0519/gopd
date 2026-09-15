package pdf

import "io"

// Read parses a bounded snapshot of r and returns detailed page contents.
// The caller retains ownership of r; no Close call is required on the result.
func Read(r io.ReaderAt, size int64) (*DetailedPDF, error) {
	d, err := Parse(r, size)
	if err != nil {
		return nil, err
	}
	return BuildPDF(d)
}

// Open returns detailed contents, resources, operations, and byte provenance.
// Use ParsePDF for a basic result. Both functions close the input file.
func Open(path string) (*DetailedPDF, error) {
	d, err := ParseFile(path)
	if err != nil {
		return nil, err
	}
	return BuildPDF(d)
}
