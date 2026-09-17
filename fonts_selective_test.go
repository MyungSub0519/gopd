package gopd

import (
	"bytes"
	"errors"
	"reflect"
	"testing"
)

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
