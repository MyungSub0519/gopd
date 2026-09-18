package gopd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/MyungSub0519/gopd/internal/pdftest"
)

func semanticFixture(objects ...string) []byte {
	return pdftest.File(objects, "")
}

func semanticStream(dict, content string) string {
	return pdftest.Stream(dict, content)
}

func semanticRead(t *testing.T, objects ...string) *DetailedPDF {
	t.Helper()
	data := semanticFixture(objects...)
	p, err := Read(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestContentMixedOrderUnicodeAndRepeatedImage(t *testing.T) {
	p := semanticRead(t,
		`<< /Type /Catalog /Pages 2 0 R >>`,
		`<< /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox [0 0 200 300] /Resources << /Font << /F1 5 0 R >> /XObject << /Im1 7 0 R >> >> >>`,
		`<< /Type /Page /Parent 2 0 R /Contents 4 0 R >>`,
		semanticStream("", `2 w 0 0 m 10 10 l S BT /F1 12 Tf 1 0 0 1 20 30 Tm <01> Tj ET q 2 0 0 3 10 20 cm /Im1 Do Q /Im1 Do`),
		`<< /Type /Font /Subtype /TrueType /BaseFont /Test /FirstChar 1 /Widths [500] /ToUnicode 6 0 R >>`,
		semanticStream("", `1 begincodespacerange <00> <FF> endcodespacerange 1 beginbfchar <01> <AC00> endbfchar`),
		semanticStream(`/Type /XObject /Subtype /Image /Width 1 /Height 1 /ColorSpace /DeviceGray /BitsPerComponent 8`, "A"))
	if len(p.Pages) != 1 || len(p.Texts) != 1 || len(p.Graphics) != 1 || len(p.Images) != 2 || len(p.ImageResources) != 1 {
		t.Fatalf("counts: pages %d texts %d graphics %d images %d resources %d", len(p.Pages), len(p.Texts), len(p.Graphics), len(p.Images), len(p.ImageResources))
	}
	if got := p.Texts[0]; got.Unicode != "가" || !got.DecodeComplete || !got.PositionComplete || got.Matrix[4] != 20 || got.Matrix[5] != 30 {
		t.Fatalf("text = %+v", got)
	}
	want := []ElementKind{ElementGraphic, ElementText, ElementImage, ElementImage}
	for i, kind := range want {
		if p.Pages[0].Items[i].Kind != kind {
			t.Fatalf("order = %+v", p.Pages[0].Items)
		}
	}
	if p.Images[0].Matrix != (Matrix{2, 0, 0, 3, 10, 20}) || p.Images[1].Matrix != IdentityMatrix() {
		t.Fatalf("image matrices: %+v", p.Images)
	}
	if p.Graphics[0].State.LineWidth != 2 || p.Pages[0].MediaBox.Max.Y != 300 {
		t.Fatal("graphics state or inherited MediaBox lost")
	}
	if p.Texts[0].Source.Spans[0].Source == p.Structure.File {
		t.Fatal("text provenance must point into decoded content")
	}
}

func TestContentStateAcrossStreamsAndTextAdvance(t *testing.T) {
	p := semanticRead(t, `<< /Type /Catalog /Pages 2 0 R >>`, `<< /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox [0 0 100 100] >>`, `<< /Type /Page /Parent 2 0 R /Resources << /Font << /F 6 0 R >> >> /Contents [4 0 R 5 0 R] >>`, semanticStream("", `q 3 w 0 0 m 1 1 l S Q BT /F 10 Tf 1 0 0 1 10 20 Tm`), semanticStream("", `[(A) 100 (A)] TJ (A) Tj ET 0 0 m 1 1 l S`), `<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding /FirstChar 65 /Widths [600] >>`)
	if len(p.Texts) != 2 || p.Texts[1].Matrix[4] != 21 {
		t.Fatalf("text advancement: %+v", p.Texts)
	}
	if p.Graphics[0].State.LineWidth != 3 || p.Graphics[1].State.LineWidth != 1 {
		t.Fatal("q/Q did not restore state")
	}
}

func TestContentFormsReuseAndCycle(t *testing.T) {
	objects := []string{`<< /Type /Catalog /Pages 2 0 R >>`, `<< /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox [0 0 100 100] >>`, `<< /Type /Page /Parent 2 0 R /Contents 4 0 R /Resources << /XObject << /Fm 5 0 R >> >> >>`, semanticStream("", `/Fm Do /Fm Do`), semanticStream(`/Type /XObject /Subtype /Form /BBox [0 0 1 1] /Matrix [2 0 0 2 0 0]`, `0 0 1 1 re f`)}
	p := semanticRead(t, objects...)
	if len(p.Graphics) != 2 || len(p.Graphics[0].Source.FormPath) != 1 || p.Graphics[0].State.CTM[0] != 2 {
		t.Fatalf("form placements = %+v", p.Graphics)
	}
	objects[4] = semanticStream(`/Type /XObject /Subtype /Form /BBox [0 0 1 1] /Resources << /XObject << /Fm 5 0 R >> >>`, `/Fm Do`)
	data := semanticFixture(objects...)
	_, err := Read(bytes.NewReader(data), int64(len(data)))
	if err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("cycle error = %v", err)
	}
}

func TestContentUnsupportedIsDiagnosedAndUnmappedCodesRetained(t *testing.T) {
	p := semanticRead(t, `<< /Type /Catalog /Pages 2 0 R >>`, `<< /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox [0 0 100 100] >>`, `<< /Type /Page /Parent 2 0 R /Contents 4 0 R /Resources << /Font << /F 5 0 R >> >> >>`, semanticStream("", `BT /F 10 Tf <FF> Tj ET /Unknown sh`), `<< /Type /Font /Subtype /TrueType /BaseFont /Unknown >>`)
	if p.Texts[0].DecodeComplete || !bytes.Equal(p.Texts[0].RawCodes, []byte{255}) || len(p.Diagnostics) == 0 || p.Pages[0].Complete {
		t.Fatalf("unsupported claims: %+v", p)
	}
}

func TestContentStandardEncodingQuotes(t *testing.T) {
	p := semanticRead(t, `<< /Type /Catalog /Pages 2 0 R >>`, `<< /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox [0 0 100 100] >>`, `<< /Type /Page /Parent 2 0 R /Contents 4 0 R /Resources << /Font << /F 5 0 R >> >> >>`, semanticStream("", `BT /F 10 Tf <2760> Tj ET`), `<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /StandardEncoding >>`)
	if p.Texts[0].Unicode != "’‘" || !p.Texts[0].DecodeComplete {
		t.Fatalf("StandardEncoding quotes = %q", p.Texts[0].Unicode)
	}
}

func TestContentHonorsGlyphAndOperandDepthLimits(t *testing.T) {
	for _, content := range []string{`BT /F 10 Tf (` + strings.Repeat("A", 101) + `) Tj ET`, strings.Repeat("[", 20) + strings.Repeat("]", 20) + ` DP`} {
		data := semanticFixture(`<< /Type /Catalog /Pages 2 0 R >>`, `<< /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox [0 0 100 100] >>`, `<< /Type /Page /Parent 2 0 R /Contents 4 0 R /Resources << /Font << /F 5 0 R >> >> >>`, semanticStream("", content), `<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>`)
		d, err := Parse(bytes.NewReader(data), int64(len(data)), ReadOptions{Limits: Limits{MaxObjects: 100, MaxDepth: 8}})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = BuildPDF(d); err == nil || !strings.Contains(err.Error(), "limit") {
			t.Fatalf("resource limit not enforced for %q: %v", content, err)
		}
	}
}

func TestContentBoundsRepeatedEmptyPageTreeNodes(t *testing.T) {
	data := semanticFixture(`<< /Type /Catalog /Pages 2 0 R >>`, `<< /Type /Pages /Kids [3 0 R 3 0 R] /Count 0 >>`, `<< /Type /Pages /Kids [4 0 R 4 0 R] /Count 0 >>`, `<< /Type /Pages /Kids [5 0 R 5 0 R] /Count 0 >>`, `<< /Type /Pages /Kids [6 0 R 6 0 R] /Count 0 >>`, `<< /Type /Pages /Kids [] /Count 0 >>`)
	d, err := Parse(bytes.NewReader(data), int64(len(data)), ReadOptions{Limits: Limits{MaxObjects: 20}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = BuildPDF(d); err == nil || !strings.Contains(err.Error(), "limit") {
		t.Fatalf("tree visit limit = %v", err)
	}
}

func TestContentRejectsDuplicateGraphicsStateEntries(t *testing.T) {
	data := semanticFixture(`<< /Type /Catalog /Pages 2 0 R >>`, `<< /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox [0 0 100 100] >>`, `<< /Type /Page /Parent 2 0 R /Contents 4 0 R /Resources << /ExtGState << /GS << /LW 1 /LW 2 >> >> >> >>`, semanticStream("", `/GS gs`))
	if _, err := Read(bytes.NewReader(data), int64(len(data))); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("duplicate graphics state = %v", err)
	}
}

func TestContentDoesNotClaimCustomCIDPositioning(t *testing.T) {
	for _, font := range []string{`<< /Type /Font /Subtype /Type0 /BaseFont /Test /Encoding /Custom-H /DescendantFonts [6 0 R] /ToUnicode 7 0 R >>`} {
		p := semanticRead(t, `<< /Type /Catalog /Pages 2 0 R >>`, `<< /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox [0 0 100 100] >>`, `<< /Type /Page /Parent 2 0 R /Contents 4 0 R /Resources << /Font << /F 5 0 R >> >> >>`, semanticStream("", `BT /F 10 Tf <01> Tj ET`), font, `<< /Type /Font /Subtype /CIDFontType2 /W [1 [500]] >>`, semanticStream("", `1 beginbfchar <01> <AC00> endbfchar`))
		if p.Texts[0].PositionComplete {
			t.Fatalf("claimed unsupported positioning for %s", font)
		}
	}
}

func TestContentBoundsExpandedFontMaps(t *testing.T) {
	data := semanticFixture(`<< /Type /Catalog /Pages 2 0 R >>`, `<< /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox [0 0 100 100] >>`, `<< /Type /Page /Parent 2 0 R /Contents 4 0 R /Resources << /Font << /F 5 0 R >> >> >>`, semanticStream("", `BT /F 10 Tf (A) Tj ET`), `<< /Type /Font /Subtype /TrueType /ToUnicode 6 0 R >>`, semanticStream("", `1 beginbfrange <00> <64> <0041> endbfrange`))
	d, err := Parse(bytes.NewReader(data), int64(len(data)), ReadOptions{Limits: Limits{MaxObjects: 100}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = BuildPDF(d); err == nil || !strings.Contains(err.Error(), "limit") {
		t.Fatalf("CMap entry budget = %v", err)
	}
}

func TestContentBoundsExpandedUnicodeText(t *testing.T) {
	data := semanticFixture(`<< /Type /Catalog /Pages 2 0 R >>`, `<< /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox [0 0 100 100] >>`, `<< /Type /Page /Parent 2 0 R /Contents 4 0 R /Resources << /Font << /F 5 0 R >> >> >>`, semanticStream("", `BT /F 10 Tf (`+strings.Repeat("A", 300)+`) Tj ET`), `<< /Type /Font /Subtype /TrueType /ToUnicode 6 0 R >>`, semanticStream("", `1 beginbfchar <41> <`+strings.Repeat("0041", 10)+`> endbfchar`))
	d, err := Parse(bytes.NewReader(data), int64(len(data)), ReadOptions{Limits: Limits{MaxDecodedBytes: 1024}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = BuildPDF(d); err == nil || !strings.Contains(err.Error(), "limit") {
		t.Fatalf("expanded Unicode budget = %v", err)
	}
}

func TestContentRejectsNonFiniteDerivedGeometry(t *testing.T) {
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

func TestContentBoundsCumulativeClipSnapshots(t *testing.T) {
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
