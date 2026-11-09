package parser

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/MyungSub0519/gopd/internal/pdftest"
)

// ISO 32000-1:2008, 9.6.5 Table 112: transform glyph-space widths once,
// using only the horizontal component even when FontMatrix rotates the glyph.
func TestType3FontMatrixAdvance(t *testing.T) {
	for _, tc := range []struct {
		name, matrix string
		want         float64
	}{
		{"standard", ".001 0 0 .001 0 0", 5},
		{"scaled", ".002 0 0 .002 0 0", 10},
		{"rotated", "0 .001 -.001 0 0 0", 0},
		{"reflected", "-.002 0 0 .002 0 0", -10},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := pdftest.File([]string{
				"<< /Type /Catalog /Pages 2 0 R >>",
				"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
				"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>",
				fmt.Sprintf("<< /Type /Font /Subtype /Type3 /FontBBox [0 0 500 500] /FontMatrix [%s] /CharProcs << /A 6 0 R >> /Encoding << /Differences [65 /A] >> /FirstChar 65 /LastChar 65 /Widths [500] >>", tc.matrix),
				pdftest.Stream("", "BT /F1 10 Tf (AA) Tj ET"),
				pdftest.Stream("", "500 0 d0 0 0 500 500 re f"),
			}, "")
			pdf, err := Read(bytes.NewReader(data), int64(len(data)))
			if err != nil {
				t.Fatal(err)
			}
			if len(pdf.Texts) != 1 || len(pdf.Texts[0].Glyphs) != 2 {
				t.Fatalf("unexpected text: %+v", pdf.Texts)
			}
			text := pdf.Texts[0]
			if !text.PositionComplete || text.Glyphs[0].Advance.X != tc.want || text.Glyphs[1].Origin.X != tc.want {
				t.Fatalf("want advance and next origin %g, got %+v", tc.want, text)
			}
		})
	}
}

// ISO 32000-1:2008, 7.3.9-7.3.10: an undefined optional entry is null/absent.
func TestUndefinedPageContents(t *testing.T) {
	data := pdftest.File([]string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Resources <<>> /Contents 99 0 R >>",
	}, "")
	pdf, err := Read(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	if len(pdf.Pages) != 1 || len(pdf.Pages[0].Contents) != 0 || !pdf.Pages[0].Complete {
		t.Fatalf("expected complete empty page: %+v", pdf.Pages)
	}
}
