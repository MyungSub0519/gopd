package gopd

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

// ISO 32000-1, 7.3.10 permits indirect objects; 9.6.6.1 defines the
// alternating character-code and glyph-name sequence in Differences.
func TestFontDifferencesIndirectItems(t *testing.T) {
	for _, tc := range []struct {
		name, differences string
		extras            []string
	}{
		{"code", `[6 0 R /B]`, []string{`65`}},
		{"glyph", `[65 6 0 R]`, []string{`/B`}},
		{"both", `[6 0 R 7 0 R]`, []string{`65`, `/B`}},
		{"reference chain", `[6 0 R 7 0 R]`, []string{`8 0 R`, `9 0 R`, `65`, `/B`}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := fontFixture(`<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding << /Differences `+tc.differences+` >> >>`, `BT /F 10 Tf (AC) Tj ET`, tc.extras...)
			pdf, err := Read(bytes.NewReader(data), int64(len(data)))
			if err != nil {
				t.Fatal(err)
			}
			if len(pdf.Texts) != 1 || pdf.Texts[0].Unicode != "BC" || !pdf.Texts[0].DecodeComplete {
				t.Fatalf("decoded text = %+v, want BC with complete decoding", pdf.Texts)
			}
			encoding := pdf.Fonts[0].Encoding.Value.(Dictionary)
			differences, err := encoding.Get("Differences")
			if err != nil {
				t.Fatal(err)
			}
			raw, err := pdf.Document.Bytes(differences.Span)
			if err != nil || string(raw) != tc.differences {
				t.Fatalf("preserved Differences = %q, %v; want %q", raw, err, tc.differences)
			}
			items := differences.Value.(Array).Items
			index := 0
			if tc.name == "glyph" {
				index = 1
			}
			if _, ok := items[index].Value.(Reference); !ok {
				t.Fatalf("original Differences item was rewritten: %+v", items[index])
			}
		})
	}
}

func TestFontDifferencesIndirectErrors(t *testing.T) {
	for _, tc := range []struct {
		name, differences, wantError string
		extras                       []string
	}{
		{"code cycle", `[6 0 R /B]`, "cyclic indirect reference", []string{`7 0 R`, `6 0 R`}},
		{"glyph cycle", `[65 6 0 R]`, "cyclic indirect reference", []string{`6 0 R`}},
		{"invalid code", `[6 0 R /B]`, "invalid encoding difference code", []string{`256`}},
		{"invalid glyph", `[65 6 0 R]`, "invalid Encoding Differences sequence", []string{`true`}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := fontFixture(`<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding << /Differences `+tc.differences+` >> >>`, `BT /F 10 Tf (A) Tj ET`, tc.extras...)
			_, err := Read(bytes.NewReader(data), int64(len(data)))
			if err == nil || !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("error = %v, want %q", err, tc.wantError)
			}
		})
	}
}

// ISO 32000-1, 9.6.2.2 names the standard 14 fonts; 9.6.6.1 describes
// StandardEncoding and a font's built-in encoding when Encoding is absent.
func TestFontImplicitStandardEncodingExactNames(t *testing.T) {
	for _, tc := range []struct {
		baseFont string
		want     string
		complete bool
	}{
		{"Helvetica", "A\u2019\u2018", true},
		{"Helvetica-Bold", "A\u2019\u2018", true},
		{"Helvetica-Oblique", "A\u2019\u2018", true},
		{"Helvetica-BoldOblique", "A\u2019\u2018", true},
		{"Times-Roman", "A\u2019\u2018", true},
		{"Times-Bold", "A\u2019\u2018", true},
		{"Times-Italic", "A\u2019\u2018", true},
		{"Times-BoldItalic", "A\u2019\u2018", true},
		{"Courier", "A\u2019\u2018", true},
		{"Courier-Bold", "A\u2019\u2018", true},
		{"Courier-Oblique", "A\u2019\u2018", true},
		{"Courier-BoldOblique", "A\u2019\u2018", true},
		{"HelveticaCustom", "\uFFFD\uFFFD\uFFFD", false},
		{"Times-Custom", "\uFFFD\uFFFD\uFFFD", false},
		{"CourierCustom", "\uFFFD\uFFFD\uFFFD", false},
		{"Symbol", "\uFFFD\uFFFD\uFFFD", false},
		{"ZapfDingbats", "\uFFFD\uFFFD\uFFFD", false},
	} {
		t.Run(tc.baseFont, func(t *testing.T) {
			data := fontFixture(fmt.Sprintf(`<< /Type /Font /Subtype /Type1 /BaseFont /%s >>`, tc.baseFont), `BT /F 10 Tf <412760> Tj ET`)
			pdf, err := Read(bytes.NewReader(data), int64(len(data)))
			if err != nil {
				t.Fatal(err)
			}
			if len(pdf.Texts) != 1 {
				t.Fatalf("text count = %d, want 1", len(pdf.Texts))
			}
			if got := pdf.Texts[0]; got.Unicode != tc.want || got.DecodeComplete != tc.complete {
				t.Fatalf("text = %q complete=%v, want %q complete=%v", got.Unicode, got.DecodeComplete, tc.want, tc.complete)
			}
			if got := pdf.Fonts[0].DecodeSupported; got != tc.complete {
				t.Fatalf("DecodeSupported = %v, want %v", got, tc.complete)
			}
		})
	}
}
