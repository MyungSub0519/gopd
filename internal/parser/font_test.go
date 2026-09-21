package parser

import (
	"bytes"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func fontFixture(font, content string, extras ...string) []byte {
	objects := []string{
		`<< /Type /Catalog /Pages 2 0 R >>`,
		`<< /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox [0 0 100 100] >>`,
		`<< /Type /Page /Parent 2 0 R /Contents 4 0 R /Resources << /Font << /F 5 0 R >> >> >>`,
		semanticStream("", content), font,
	}
	return semanticFixture(append(objects, extras...)...)
}

func TestFontDifferencesImplicitBase(t *testing.T) {
	data := fontFixture(`<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding << /Differences [65 /B] >> >>`, "BT /F 10 Tf (AC'`) Tj ET")
	pdf, err := Read(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	if got := pdf.Texts[0]; got.Unicode != "BC’‘" || !got.DecodeComplete {
		t.Fatalf("implicit base = %q complete=%v", got.Unicode, got.DecodeComplete)
	}
}

func TestFontCMapExpansionBudget(t *testing.T) {
	cmap := `1 beginbfrange <00> <FF> <` + strings.Repeat("0061", 255) + `0000> endbfrange`
	data := fontFixture(`<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /ToUnicode 6 0 R >>`, `BT /F 10 Tf ET`, semanticStream("", cmap))
	doc, err := Parse(bytes.NewReader(data), int64(len(data)), ReadOptions{Limits: Limits{MaxDecodedBytes: 4096}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = BuildPDF(doc); !errors.Is(err, errCMapLimit) {
		t.Fatalf("expanded mapping byte limit = %v", err)
	}
}

func TestFontCMapEntryBudget(t *testing.T) {
	data := fontFixture(`<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /ToUnicode 6 0 R >>`, `BT /F 10 Tf ET`,
		semanticStream("", `1 beginbfrange <00> <64> <0000> endbfrange`))
	doc, err := Parse(bytes.NewReader(data), int64(len(data)), ReadOptions{Limits: Limits{MaxObjects: 64}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := BuildPDF(doc); !errors.Is(err, errCMapLimit) {
		t.Fatalf("expanded mapping entry limit = %v", err)
	}
}

func TestFontToUnicodeStreamLimit(t *testing.T) {
	data := fontFixture(`<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /ToUnicode 6 0 R >>`, `BT /F 10 Tf ET`,
		semanticStream("", strings.Repeat(" ", 512)+`1 beginbfchar <01> <0041> endbfchar`))
	doc, err := Parse(bytes.NewReader(data), int64(len(data)), ReadOptions{Limits: Limits{MaxDecodedBytes: 256}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := BuildPDF(doc); !errors.Is(err, ErrLimit) {
		t.Fatalf("ToUnicode stream limit = %v", err)
	}
}

func TestFontCIDIndirectWidths(t *testing.T) {
	data := fontFixture(`<< /Type /Font /Subtype /Type0 /BaseFont /Test /Encoding /Identity-H /DescendantFonts [6 0 R] >>`, `BT /F 10 Tf <000100020003> Tj ET`,
		`<< /Type /Font /Subtype /CIDFontType2 /W [7 0 R 8 0 R 10 0 R 11 0 R 9 0 R] >>`, `1`, `[9 0 R]`, `500`, `2`, `3`)
	pdf, err := Read(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	for _, glyph := range pdf.Texts[0].Glyphs {
		if !glyph.WidthKnown || glyph.Advance.X != 5 {
			t.Fatalf("glyph = %+v", glyph)
		}
	}
}

func TestToUnicodeCodespaceByteBounds(t *testing.T) {
	for _, data := range []string{
		`1 begincodespacerange <8140> <9FFE> endcodespacerange 1 beginbfchar <8201> <0041> endbfchar`,
		`1 begincodespacerange <81FF> <9F40> endcodespacerange 1 beginbfchar <8201> <0041> endbfchar`,
	} {
		if _, err := parseToUnicode([]byte(data)); err == nil {
			t.Fatalf("accepted invalid codespace/mapping: %s", data)
		}
	}
	c := CMap{CodeSpaces: []CodeSpace{{Low: []byte{0x81, 0x40}, High: []byte{0x9f, 0xfe}}}, Mappings: map[string]string{"\x82\x01": "A"}}
	if got, _, complete := c.decode([]byte{0x82, 0x01}); got == "A" || complete {
		t.Fatalf("decoded out-of-space code = %q complete=%v", got, complete)
	}
}

func TestToUnicodeBudgetBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name, data string
		entries    int
		bytes      int64
	}{
		{"ASCII", `1 beginbfchar <01> <0041> endbfchar`, 1, 2},
		{"Latin", `1 beginbfchar <01> <00E9> endbfchar`, 1, 3},
		{"Korean", `1 beginbfchar <01> <AC00> endbfchar`, 1, 4},
		{"surrogate", `1 beginbfchar <0001> <D83DDE00> endbfchar`, 1, 6},
		{"sequence", `1 beginbfchar <01> <004100E9AC00D83DDE00> endbfchar`, 1, 11},
		{"range", `1 beginbfrange <01> <02> <0041> endbfrange`, 2, 4},
		{"array", `1 beginbfrange <01> <02> [<0041> <AC00>] endbfrange`, 2, 6},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, err := parseToUnicodeBounded([]byte(tc.data), tc.entries, tc.bytes)
			if err != nil {
				t.Fatal(err)
			}
			if got := c.mappingBytes(); got != tc.bytes {
				t.Fatalf("mapping bytes = %d, want %d", got, tc.bytes)
			}
			for _, limits := range []struct {
				entries int
				bytes   int64
			}{{tc.entries - 1, tc.bytes}, {tc.entries, tc.bytes - 1}} {
				if _, err := parseToUnicodeBounded([]byte(tc.data), limits.entries, limits.bytes); !errors.Is(err, errCMapLimit) {
					t.Fatalf("limits %+v: got %v", limits, err)
				}
			}
		})
	}
	// Reject a range on its count before attempting to parse or expand a destination.
	if _, err := parseToUnicodeBounded([]byte(`1 beginbfrange <0000> <FFFF> invalid endbfrange`), 2, 4096); !errors.Is(err, errCMapLimit) {
		t.Fatalf("entry preflight = %v", err)
	}
}

func TestFontCMapSharedBudget(t *testing.T) {
	cmap := `1 beginbfrange <00> <07> <` + strings.Repeat("0061", 255) + `0000> endbfrange`
	for _, tc := range []struct {
		name      string
		separate  bool
		text      string
		wantError bool
	}{
		{"cached once", false, "", false},
		{"different maps", true, "", true},
		{"mapping plus text", false, `<000000000000000000> Tj`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			secondMap := 6
			if tc.separate {
				secondMap = 8
			}
			data := semanticFixture(
				`<< /Type /Catalog /Pages 2 0 R >>`,
				`<< /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox [0 0 100 100] >>`,
				`<< /Type /Page /Parent 2 0 R /Contents 4 0 R /Resources << /Font << /F 5 0 R /G 7 0 R >> >> >>`,
				semanticStream("", `BT /F 10 Tf /G 10 Tf `+tc.text+` ET`),
				`<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /ToUnicode 6 0 R >>`,
				semanticStream("", cmap),
				fmt.Sprintf(`<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /ToUnicode %d 0 R >>`, secondMap),
				semanticStream("", cmap))
			doc, err := Parse(bytes.NewReader(data), int64(len(data)), ReadOptions{Limits: Limits{MaxDecodedBytes: 4096}})
			if err != nil {
				t.Fatal(err)
			}
			pdf, err := BuildPDF(doc)
			if (err != nil) != tc.wantError {
				t.Fatalf("error = %v, want error %v", err, tc.wantError)
			}
			if !tc.wantError && pdf.Fonts[0].ToUnicode != pdf.Fonts[1].ToUnicode {
				t.Fatal("shared map was not cached")
			}
		})
	}
}

func FuzzToUnicodeBounded(f *testing.F) {
	f.Add([]byte(`1 beginbfchar <01> <0041> endbfchar`), []byte{1})
	f.Add([]byte(`1 beginbfrange <01> <02> [<0041> <D83DDE00>] endbfrange`), []byte{1, 2})
	f.Add([]byte(`1 beginbfrange <0000> <FFFF> <0041> endbfrange`), []byte{0, 0})
	f.Fuzz(func(t *testing.T, data, raw []byte) {
		if len(data) > 4096 || len(raw) > 256 {
			t.Skip()
		}
		c, err := parseToUnicodeBounded(data, 32, 4096)
		if err != nil {
			return
		}
		if len(c.Mappings) > 32 || c.mappingBytes() > 4096 {
			t.Fatal("accepted over-budget mapping")
		}
		text, _, _, err := c.decodeBounded(raw, 4096)
		if err == nil && len(text) > 4096 {
			t.Fatal("accepted over-budget text")
		}
	})
}

func TestFontWidthBudgetReturnsErrLimit(t *testing.T) {
	for _, test := range []struct {
		name   string
		font   string
		extras []string
	}{
		{"simple", `<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /FirstChar 0 /Widths [` + strings.Repeat("500 ", 21) + `] >>`, nil},
		{"CID array", `<< /Type /Font /Subtype /Type0 /Encoding /Identity-H /DescendantFonts [6 0 R] >>`, []string{`<< /Subtype /CIDFontType2 /W [0 [` + strings.Repeat("500 ", 21) + `]] >>`}},
		{"CID range", `<< /Type /Font /Subtype /Type0 /Encoding /Identity-H /DescendantFonts [6 0 R] >>`, []string{`<< /Subtype /CIDFontType2 /W [0 20 500] >>`}},
	} {
		t.Run(test.name, func(t *testing.T) {
			data := fontFixture(test.font, `BT /F 10 Tf ET`, test.extras...)
			d, err := Parse(bytes.NewReader(data), int64(len(data)), ReadOptions{Limits: Limits{MaxObjects: 20}})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := BuildPDF(d); !errors.Is(err, ErrLimit) {
				t.Fatalf("font width budget must return ErrLimit: %v", err)
			}
		})
	}
}

func TestFontDecodedTextBudgetReturnsErrLimit(t *testing.T) {
	for _, test := range []struct {
		name string
		font Font
	}{
		{"simple", Font{simpleEncoding: map[byte]string{'A': "가"}}},
		{"ToUnicode", Font{ToUnicode: &CMap{Mappings: map[string]string{"A": "가"}}}},
		{"unmapped", Font{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, _, _, err := test.font.decodeBounded([]byte("AA"), 5); !errors.Is(err, ErrLimit) {
				t.Fatalf("text expansion must return ErrLimit: %v", err)
			}
			text, _, _, err := test.font.decodeBounded([]byte("AA"), 6)
			if err != nil || len(text) != 6 {
				t.Fatalf("exact text byte budget must pass: %q, %v", text, err)
			}
		})
	}
}

func TestFontInvalidCIDRangeIsNotResourceLimit(t *testing.T) {
	for _, widths := range []string{`[65535 [500 500]]`, `[0 65536 500]`} {
		data := fontFixture(`<< /Type /Font /Subtype /Type0 /Encoding /Identity-H /DescendantFonts [6 0 R] >>`, `BT /F 10 Tf ET`, `<< /Subtype /CIDFontType2 /W `+widths+` >>`)
		if _, err := Read(bytes.NewReader(data), int64(len(data))); err == nil || errors.Is(err, ErrLimit) {
			t.Fatalf("invalid CID range is a format error: %v", err)
		}
	}
}

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

func TestExtractUnicodeSkipsFontMetrics(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name   string
		font   string
		extras []string
	}{
		{
			name: "simple widths",
			font: `<< /Subtype /Type1 /BaseFont /Helvetica /FirstChar 65 /Widths 99 0 R >>`,
		},
		{
			name: "simple descriptor",
			font: `<< /Subtype /Type1 /BaseFont /Helvetica /FontDescriptor 99 0 R >>`,
		},
		{
			name: "CID widths",
			font: `<< /Subtype /Type0 /Encoding /Identity-H /DescendantFonts [6 0 R] /ToUnicode 7 0 R >>`,
			extras: []string{
				`<< /Subtype /CIDFontType2 /W 99 0 R >>`,
				semanticStream("", `1 beginbfchar <41> <0041> endbfchar`),
			},
		},
		{
			name: "CID default width",
			font: `<< /Subtype /Type0 /Encoding /Identity-H /DescendantFonts [6 0 R] /ToUnicode 7 0 R >>`,
			extras: []string{
				`<< /Subtype /CIDFontType2 /DW 99 0 R >>`,
				semanticStream("", `1 beginbfchar <41> <0041> endbfchar`),
			},
		},
		{
			name: "CID descriptor",
			font: `<< /Subtype /Type0 /Encoding /Identity-H /DescendantFonts [6 0 R] /ToUnicode 7 0 R >>`,
			extras: []string{
				`<< /Subtype /CIDFontType2 /FontDescriptor 99 0 R >>`,
				semanticStream("", `1 beginbfchar <41> <0041> endbfchar`),
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			data := fontFixture(test.font, `BT /F 10 Tf (A) Tj ET`, test.extras...)
			got, err := ExtractReader(bytes.NewReader(data), int64(len(data)), ExtractOptions{Content: ContentText})
			if err != nil {
				t.Fatalf("Unicode extraction resolved unused metrics: %v", err)
			}
			if len(got.Pages[0].Texts) != 1 || got.Pages[0].Texts[0].Unicode != "A" || !got.Pages[0].Texts[0].DecodeComplete {
				t.Fatalf("Unicode text=%+v, want A", got.Pages[0].Texts)
			}
			_, err = ExtractReader(bytes.NewReader(data), int64(len(data)), ExtractOptions{Content: ContentText, Positions: true})
			if err == nil {
				t.Fatal("positioned text must resolve required metrics")
			}
		})
	}
}

func TestExtractPositionsSkipsEmbeddedFontStream(t *testing.T) {
	t.Parallel()
	data := fontFixture(`<< /Subtype /Type1 /BaseFont /Helvetica /FontDescriptor << /MissingWidth 500 /FontFile 99 0 R >> >>`, `BT /F 10 Tf (AA) Tj ET`)
	got, err := ExtractReader(bytes.NewReader(data), int64(len(data)), ExtractOptions{Content: ContentText, Glyphs: true})
	if err != nil {
		t.Fatalf("positioned extraction resolved unused font stream: %v", err)
	}
	text := got.Pages[0].Texts[0]
	if text.Unicode != "AA" || len(text.Glyphs) != 2 {
		t.Fatalf("positioned text=%+v", text)
	}
	if !text.Position.Complete || text.Glyphs[1].Origin != (Point{X: 5}) || text.Glyphs[1].Advance != (Point{X: 5}) {
		t.Fatalf("MissingWidth was not used for glyph positions: %+v", text.Glyphs)
	}
	if _, err := Read(bytes.NewReader(data), int64(len(data))); err == nil {
		t.Fatal("legacy detailed parsing must keep resolving the embedded font")
	}
}

func TestExtractUnicodeOmitsPositioningDiagnostics(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name string
		font string
	}{
		{"Type3", `<< /Subtype /Type3 /Encoding /StandardEncoding >>`},
		{"custom CID encoding", `<< /Subtype /Type0 /Encoding /Custom-H /DescendantFonts [7 0 R] /ToUnicode 6 0 R >>`},
	} {
		t.Run(test.name, func(t *testing.T) {
			data := fontFixture(test.font, `BT /F 10 Tf (A) Tj ET`,
				semanticStream("", `1 beginbfchar <41> <0041> endbfchar`), `<< /Subtype /CIDFontType2 >>`)
			got, err := ExtractReader(bytes.NewReader(data), int64(len(data)), ExtractOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if len(got.Diagnostics) != 0 || !got.Pages[0].Complete || got.Pages[0].Texts[0].Unicode != "A" {
				t.Fatalf("complete Unicode extraction reports positioning limitations: %+v", got)
			}
		})
	}
}

func TestFontDecodeSelected(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name     string
		font     Font
		raw      []byte
		want     string
		complete bool
		codes    []decodedCode
	}{
		{
			name: "simple",
			font: Font{simpleEncoding: map[byte]string{'A': "A", 0x80: "€"}},
			raw:  []byte{'A', 0x80}, want: "A€", complete: true,
			codes: []decodedCode{{[]byte{'A'}, "A", true}, {[]byte{0x80}, "€", true}},
		},
		{
			name: "composite fallback and truncated code",
			font: Font{composite: true}, raw: []byte{0x80, 1, 0x80}, want: "��",
			codes: []decodedCode{{[]byte{0x80, 1}, "�", false}, {[]byte{0x80}, "�", false}},
		},
		{
			name: "variable codes and unmapped code",
			font: Font{ToUnicode: &CMap{
				CodeSpaces: []CodeSpace{{[]byte{0}, []byte{0x7f}}, {[]byte{0x80, 0}, []byte{0xff, 0xff}}},
				Mappings:   map[string]string{"A": "A", "\x80\x01": "😀"},
			}},
			raw: []byte{'A', 0x80, 1, 0x80, 2}, want: "A😀�",
			codes: []decodedCode{{[]byte{'A'}, "A", true}, {[]byte{0x80, 1}, "😀", true}, {[]byte{0x80, 2}, "�", false}},
		},
		{
			name: "inferred codespace",
			font: Font{ToUnicode: &CMap{Mappings: map[string]string{"\x80\x01": "한글"}}},
			raw:  []byte{0x80, 1}, want: "한글", complete: true,
			codes: []decodedCode{{[]byte{0x80, 1}, "한글", true}},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, collect := range []bool{false, true} {
				text, codes, complete, err := test.font.decodeSelected(test.raw, int64(len(test.want)), collect)
				if err != nil || text != test.want || complete != test.complete {
					t.Fatalf("collect=%v: text=%q complete=%v err=%v", collect, text, complete, err)
				}
				if collect && !reflect.DeepEqual(codes, test.codes) {
					t.Fatalf("decoded codes=%+v, want %+v", codes, test.codes)
				}
				if !collect && codes != nil {
					t.Fatalf("Unicode-only decoding returned character details: %+v", codes)
				}
				if _, _, _, err := test.font.decodeSelected(test.raw, int64(len(test.want)-1), collect); !errors.Is(err, ErrLimit) {
					t.Fatalf("collect=%v: UTF-8 byte limit error=%v", collect, err)
				}
			}
		})
	}
}

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
