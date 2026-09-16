package gopd

import (
	"bytes"
	"testing"
)

// The PDF container is always valid so mutations exercise the content
// interpreter and source concatenation rather than stopping at the file header.
func FuzzBuildPDF(f *testing.F) {
	f.Add([]byte(`0 0 m 10 10 l S`), uint16(7))
	f.Add([]byte(`BT /F 10 Tf [(A) 100 (A)] TJ ET`), uint16(17))
	f.Add([]byte(`q /Fm Do Q /Im Do`), uint16(10))
	f.Add([]byte(`<< /Key [1 2 null] >> /Tag BDC EMC`), uint16(15))
	f.Add([]byte(``), uint16(0))
	f.Fuzz(func(t *testing.T, content []byte, splitAt uint16) {
		if len(content) > 4096 {
			return
		}
		split := int(splitAt) % (len(content) + 1)
		data := semanticFixture(
			`<< /Type /Catalog /Pages 2 0 R >>`,
			`<< /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox [0 0 100 100] >>`,
			`<< /Type /Page /Parent 2 0 R /Contents [4 0 R 5 0 R] /Resources << /Font << /F 7 0 R >> /XObject << /Fm 6 0 R /Im 8 0 R >> >> >>`,
			semanticStream("", string(content[:split])), semanticStream("", string(content[split:])),
			semanticStream(`/Subtype /Form /BBox [10 10 0 0] /Resources << /Font << /F 7 0 R >> >>`, `BT /F 10 Tf (A) Tj ET`),
			`<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding /FirstChar 65 /Widths [600] >>`,
			semanticStream(`/Subtype /Image /Width 1 /Height 1 /ColorSpace /DeviceGray /BitsPerComponent 8`, "A"))
		d, err := Parse(bytes.NewReader(data), int64(len(data)), ReadOptions{Limits: Limits{
			MaxDepth: 12, MaxObjects: 128, MaxValues: 2048, MaxSemanticObjects: 256,
			MaxContentBytes: 32 << 10, MaxDecodedBytes: 32 << 10, MaxTokenBytes: 4096,
		}})
		if err != nil {
			t.Fatalf("generated container failed to parse: %v", err)
		}
		p, _ := BuildPDF(d)
		if p == nil {
			t.Fatal("valid Document must retain a semantic snapshot, including on content errors")
		}
		for _, page := range p.Pages {
			for _, operation := range page.Operations {
				raw, err := d.Bytes(operation.Span)
				if err != nil || string(raw) != operation.Operator {
					t.Fatalf("operation provenance: %q != %q, %v", raw, operation.Operator, err)
				}
			}
		}
		for _, text := range p.Texts {
			if !finiteMatrix(text.Matrix) {
				t.Fatal("nonfinite retained text matrix")
			}
			for _, glyph := range text.Glyphs {
				if !finitePoint(glyph.Origin) || !finitePoint(glyph.Advance) {
					t.Fatal("nonfinite retained glyph")
				}
			}
		}
	})
}
