package gopd

import (
	"bytes"
	"strings"
	"testing"
)

func TestReviewContentWinAnsiGlyphAliases(t *testing.T) {
	// PDF WinAnsi assigns space/hyphen and bullet glyphs to these byte slots;
	// their Unicode meaning differs from the Windows-1252 character mapping.
	p := semanticRead(t,
		`<< /Type /Catalog /Pages 2 0 R >>`,
		`<< /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox [0 0 100 100] >>`,
		`<< /Type /Page /Parent 2 0 R /Contents 4 0 R /Resources << /Font << /F 5 0 R >> >> >>`,
		semanticStream("", `BT /F 10 Tf <A0AD7F818D8F909D> Tj ET`),
		`<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>`)
	if got := p.Texts[0]; got.Unicode != " -••••••" || !got.DecodeComplete {
		t.Fatalf("WinAnsi decode = %q complete=%v, want space/hyphen/six bullets", got.Unicode, got.DecodeComplete)
	}
}

func TestReviewContentRejectsNonFiniteDerivedGeometry(t *testing.T) {
	huge := "1" + strings.Repeat("0", 200)
	for _, test := range []struct {
		name    string
		content string
		form    string
	}{
		{"path", huge + " 0 0 1 0 0 cm " + huge + " 0 m 1 1 l S", "0 0 1 1 re f"},
		{"text", "BT /F " + huge + " Tf " + huge + " 0 0 1 0 0 Tm (A) Tj ET", "0 0 1 1 re f"},
		{"form", huge + " 0 0 1 0 0 cm /Fm Do", "0 0 1 1 re f"},
	} {
		t.Run(test.name, func(t *testing.T) {
			data := semanticFixture(
				`<< /Type /Catalog /Pages 2 0 R >>`,
				`<< /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox [0 0 100 100] >>`,
				`<< /Type /Page /Parent 2 0 R /Contents 4 0 R /Resources << /Font << /F 5 0 R >> /XObject << /Fm 6 0 R >> >> >>`,
				semanticStream("", test.content),
				`<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding /FirstChar 65 /Widths [600] >>`,
				semanticStream(`/Type /XObject /Subtype /Form /BBox [0 0 1 1] /Matrix [`+huge+` 0 0 1 0 0]`, test.form))
			if _, err := Read(bytes.NewReader(data), int64(len(data))); err == nil {
				t.Fatal("accepted derived coordinates containing infinity or NaN")
			}
		})
	}
}

func TestReviewContentBoundsCumulativeClipSnapshots(t *testing.T) {
	// There are only 45 operators, but copying the growing clip history requires
	// 105 previous clip references. The configured budget must bound that work.
	data := semanticFixture(
		`<< /Type /Catalog /Pages 2 0 R >>`,
		`<< /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox [0 0 100 100] >>`,
		`<< /Type /Page /Parent 2 0 R /Contents 4 0 R >>`,
		semanticStream("", strings.Repeat(`0 0 1 1 re W n `, 15)))
	doc, err := Parse(bytes.NewReader(data), int64(len(data)), ReadOptions{Limits: Limits{MaxObjects: 100}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := BuildPDF(doc); err == nil || !strings.Contains(err.Error(), "limit") {
		t.Fatalf("cumulative clip snapshot budget not enforced: %v", err)
	}
}

func TestReviewContentBoundsRepeatedCIDWidthExpansion(t *testing.T) {
	data := semanticFixture(
		`<< /Type /Catalog /Pages 2 0 R >>`,
		`<< /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox [0 0 100 100] >>`,
		`<< /Type /Page /Parent 2 0 R /Contents 4 0 R /Resources << /Font << /F 5 0 R >> >> >>`,
		semanticStream("", `BT /F 10 Tf <0001> Tj ET`),
		`<< /Type /Font /Subtype /Type0 /BaseFont /Test /Encoding /Identity-H /DescendantFonts [6 0 R] >>`,
		`<< /Type /Font /Subtype /CIDFontType2 /W [0 99 500 0 99 500] >>`)
	doc, err := Parse(bytes.NewReader(data), int64(len(data)), ReadOptions{Limits: Limits{MaxObjects: 100}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := BuildPDF(doc); err == nil {
		t.Fatal("repeated CID-width ranges bypassed the expansion budget")
	}
}
