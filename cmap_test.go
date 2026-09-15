package pdf

import (
	"fmt"
	"strings"
	"testing"
)

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
