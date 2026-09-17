package gopd

import (
	"bytes"
	"testing"
)

func FuzzExtractReader(f *testing.F) {
	f.Add([]byte(`BT /F 10 Tf [(A) 100 (A)] TJ ET`), uint16(17), byte(1), byte(0))
	f.Add([]byte(`q 2 0 0 2 0 0 cm /Fm Do Q /Im Do`), uint16(10), byte(7), byte(15))
	f.Add([]byte(`0 0 m 10 10 l S (unterminated`), uint16(9), byte(2), byte(8))
	f.Add([]byte(`[1 2 3] 0 d /Sh sh`), uint16(4), byte(1), byte(0))
	f.Fuzz(func(t *testing.T, content []byte, splitAt uint16, mask, flags byte) {
		if len(content) > 4096 {
			return
		}
		split := int(splitAt) % (len(content) + 1)
		data := semanticFixture(
			`<< /Type /Catalog /Pages 2 0 R >>`,
			`<< /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox [0 0 100 100] >>`,
			`<< /Type /Page /Parent 2 0 R /Contents [4 0 R 5 0 R] /Resources << /Font << /F 7 0 R >> /XObject << /Fm 6 0 R /Im 8 0 R >> >> >>`,
			semanticStream("", string(content[:split])), semanticStream("", string(content[split:])),
			semanticStream(`/Subtype /Form /BBox [0 0 10 10]`, `BT /F 10 Tf (A) Tj ET 0 0 1 1 re f`),
			`<< /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding /FirstChar 65 /Widths [600] >>`,
			semanticStream(`/Subtype /Image /Width 1 /Height 1 /ColorSpace /DeviceGray /BitsPerComponent 8`, "A"),
		)
		kind := ContentKind(mask) & ContentAll
		if kind == 0 {
			kind = ContentText
		}
		options := ExtractOptions{
			Content: kind, Positions: flags&1 != 0, Styles: flags&2 != 0,
			Glyphs: flags&4 != 0 && kind&ContentText != 0, Provenance: flags&8 != 0,
			ReadOptions: ReadOptions{Limits: Limits{
				MaxDepth: 12, MaxObjects: 128, MaxValues: 2048, MaxSemanticObjects: 256,
				MaxContentBytes: 32 << 10, MaxDecodedBytes: 32 << 10, MaxTokenBytes: 4096,
			}},
		}
		result, err := ExtractReader(bytes.NewReader(data), int64(len(data)), options)
		if result == nil {
			t.Fatalf("valid container lost partial result: %v", err)
		}
		if (result.Document != nil) != options.Provenance {
			t.Fatal("source retention disagrees with options")
		}
		for _, page := range result.Pages {
			if kind&ContentText == 0 && len(page.Texts) != 0 {
				t.Fatal("unselected text retained")
			}
			if kind&ContentGraphics == 0 && len(page.Graphics) != 0 {
				t.Fatal("unselected graphics retained")
			}
			if kind&ContentImages == 0 && len(page.Images) != 0 {
				t.Fatal("unselected images retained")
			}
			for _, text := range page.Texts {
				if text.Position != nil && !finiteMatrix(text.Position.Matrix) {
					t.Fatal("nonfinite text matrix")
				}
				for _, glyph := range text.Glyphs {
					if !finitePoint(glyph.Origin) || !finitePoint(glyph.Advance) {
						t.Fatal("nonfinite glyph")
					}
				}
			}
			for _, operation := range page.Operations {
				raw, e := result.Document.Bytes(operation.Span)
				if e != nil || string(raw) != operation.Operator {
					t.Fatal("invalid operation source")
				}
			}
		}
	})
}
