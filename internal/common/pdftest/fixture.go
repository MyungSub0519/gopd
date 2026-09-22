// Package pdftest builds synthetic PDF fixtures independently of the parser.
// It is intended for tests only.
package pdftest

import (
	"bytes"
	"fmt"
)

// File writes a PDF 1.7 file with sequential generation-zero objects, a text
// xref table and object 1 as the catalog. trailerSuffix is appended verbatim
// after /Root 1 0 R; include leading whitespace for additional trailer entries.
// Keeping it verbatim also lets tests construct malformed trailers.
func File(objects []string, trailerSuffix string) []byte {
	var b bytes.Buffer
	b.WriteString("%PDF-1.7\n")
	offsets := make([]int, len(objects)+1)
	for i, object := range objects {
		offsets[i+1] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, object)
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(offsets))
	for _, offset := range offsets[1:] {
		fmt.Fprintf(&b, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R%s >>\nstartxref\n%d\n%%%%EOF\n", len(offsets), trailerSuffix, xref)
	return b.Bytes()
}

// Stream writes a direct-length stream. dict contains additional dictionary
// entries without << >>; content may contain binary bytes.
func Stream(dict, content string) string {
	return fmt.Sprintf("<< /Length %d %s >>\nstream\n%s\nendstream", len(content), dict, content)
}
