package gopd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// File entry points compare the user-visible APIs on the checked-in fixture.
func BenchmarkExtract(b *testing.B) {
	for _, mode := range []string{"legacy", "text", "all"} {
		b.Run(mode, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				var err error
				if mode == "legacy" {
					_, err = ParsePDF("testdata/synthetic.pdf")
				} else {
					options := ExtractOptions{}
					if mode == "all" {
						options = ExtractOptions{Content: ContentAll, Positions: true, Styles: true, Glyphs: true}
					}
					_, err = Extract("testdata/synthetic.pdf", options)
				}
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkExtractDense(b *testing.B) {
	data := semanticFixture(
		`<< /Type /Catalog /Pages 2 0 R >>`,
		`<< /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox [0 0 100 100] >>`,
		`<< /Type /Page /Parent 2 0 R /Contents 4 0 R /Resources << /Font << /F 5 0 R >> >> >>`,
		semanticStream("", strings.Repeat(`BT /F 12 Tf (AAAAAAAAAAAAAAAAAAAA) Tj ET 0 0 10 10 re f `, 2000)),
		`<< /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding /FirstChar 65 /Widths [600] >>`,
	)
	for _, mode := range []string{"legacy", "text", "all"} {
		b.Run(mode, func(b *testing.B) {
			parse := func() (any, error) {
				reader := bytes.NewReader(data)
				if mode == "legacy" {
					detail, err := Read(reader, int64(len(data)))
					return basicPDF(detail), err
				}
				options := ExtractOptions{}
				if mode == "all" {
					options = ExtractOptions{Content: ContentAll, Positions: true, Styles: true, Glyphs: true}
				}
				return ExtractReader(reader, int64(len(data)), options)
			}
			output, err := parse()
			if err != nil {
				b.Fatal(err)
			}
			encoded, err := json.Marshal(output)
			if err != nil {
				b.Fatal(err)
			}
			jsonBytes := len(encoded)
			b.ReportAllocs()
			for b.Loop() {
				if _, err := parse(); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportMetric(float64(jsonBytes), "json-bytes")
		})
	}
}
