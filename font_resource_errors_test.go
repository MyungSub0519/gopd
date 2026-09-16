package gopd

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

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
