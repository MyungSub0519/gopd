package content

import "testing"

func TestGlyphlistResolvesAdobeNames(t *testing.T) {
	cases := map[string]string{
		"A":           "A",
		"space":       " ",
		"ae":          "æ",
		"Euro":        "€",
		"fi":          "ﬁ",
		"Delta":       "∆",
		"u1D504":      "𝔄",
		"uni20AC":     "€",
		"notaglyph":   "",
		"colon":       ":",
		"hyphen":      "-",
		"eight":       "8",
	}
	for name, want := range cases {
		got, ok := glyphUnicode(name)
		if want == "" {
			if ok {
				t.Errorf("glyphUnicode(%q) = %q, true; want unmapped", name, got)
			}
			continue
		}
		if !ok || got != want {
			t.Errorf("glyphUnicode(%q) = %q, %v; want %q, true", name, got, ok, want)
		}
	}
}

func TestGlyphlistParsesEveryEntry(t *testing.T) {
	if len(glyphlistMap()) < 4000 {
		t.Errorf("glyphlist has %d entries; want the full Adobe list", len(glyphlistMap()))
	}
}
