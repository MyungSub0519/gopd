package gopd

import (
	"bytes"
	"errors"
	"fmt"
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
