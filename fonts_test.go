package gopd

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

func TestFontWinAnsiGlyphAliases(t *testing.T) {
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

func TestFontBoundsRepeatedCIDWidthExpansion(t *testing.T) {
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

func TestToUnicodeKoreanAndRanges(t *testing.T) {
	cmap, err := parseToUnicode([]byte(`begincmap
1 begincodespacerange <00> <ff> endcodespacerange
1 beginbfchar <01> <AC00> endbfchar
2 beginbfrange <02> <03> <AC01> <10> <11> [<D55C> <AE00>] endbfrange
endcmap`))
	if err != nil {
		t.Fatal(err)
	}
	got, codes, complete := cmap.decode([]byte{1, 2, 3, 16, 17, 255})
	if got != "가각갂한글\uFFFD" || complete || len(codes) != 6 {
		t.Fatalf("decode = %q, %v, %v", got, codes, complete)
	}
}

func TestToUnicodeVariableCodesAndSurrogate(t *testing.T) {
	cmap, err := parseToUnicode([]byte(`2 begincodespacerange <00> <7F> <8000> <FFFF> endcodespacerange
2 beginbfchar <41> <0041> <8001> <D83DDE00> endbfchar`))
	if err != nil {
		t.Fatal(err)
	}
	got, _, complete := cmap.decode([]byte{0x41, 0x80, 0x01})
	if got != "A😀" || !complete {
		t.Fatalf("decode = %q, %v", got, complete)
	}
}

func TestToUnicodeRejectsMalformedAndOversizedRanges(t *testing.T) {
	for _, data := range []string{`1 beginbfchar <01> <D800> endbfchar`, `1 beginbfrange <00000000> <FFFFFFFF> <0041> endbfrange`, `1 beginbfchar <01> endbfchar`} {
		if _, err := parseToUnicode([]byte(data)); err == nil {
			t.Errorf("accepted %q", data)
		}
	}
}

func TestToUnicodeRejectsDestinationOverflowAndCodesOutsideSpace(t *testing.T) {
	for _, data := range []string{`1 beginbfrange <01> <02> <FFFF> endbfrange`, `1 begincodespacerange <00> <7F> endcodespacerange 1 beginbfchar <FF> <0041> endbfchar`} {
		if _, err := parseToUnicode([]byte(data)); err == nil {
			t.Errorf("accepted invalid mapping %q", data)
		}
	}
}

func TestToUnicodeCodeSpaceLimit(t *testing.T) {
	var data strings.Builder
	data.WriteString("300 begincodespacerange ")
	for i := 0; i < 300; i++ {
		fmt.Fprintf(&data, "<%04X> <%04X> ", i, i)
	}
	data.WriteString("endcodespacerange 1 beginbfchar <0000> <0041> endbfchar")
	if _, err := parseToUnicode([]byte(data.String())); err == nil || !strings.Contains(err.Error(), "limit") {
		t.Fatalf("codespace resource limit = %v", err)
	}
}
