package content

import (
	_ "embed"
	"strconv"
	"strings"
	"sync"
)

// glyphlistData is the Adobe Glyph List, which maps glyph names to Unicode
// scalar values. It ships with the library so that /Differences entries can be
// resolved without any file access at runtime.
//
//go:embed glyphlist.txt
var glyphlistData string

var (
	glyphlistOnce sync.Once
	glyphlist     map[string]string
)

// glyphlistMap parses the embedded Adobe Glyph List on first use.
//
// Each non-comment line is "name;code" where code is a sequence of 16-bit
// values, one per UTF-16 code unit of the Unicode string. Parsing is done once
// and shared by every lookup.
func glyphlistMap() map[string]string {
	glyphlistOnce.Do(func() {
		glyphlist = make(map[string]string, 4300)
		for _, line := range strings.Split(glyphlistData, "\n") {
			name, codes, found := strings.Cut(line, ";")
			if !found || strings.HasPrefix(name, "#") {
				continue
			}
			var text strings.Builder
			for len(codes) > 0 {
				var hex string
				// When no space remains Cut returns the whole rest as hex and
				// an empty after, which ends the loop.
				hex, codes, _ = strings.Cut(codes, " ")
				n, err := strconv.ParseUint(hex, 16, 16)
				if err != nil {
					text.Reset()
					break
				}
				text.WriteRune(rune(n))
			}
			if text.Len() > 0 {
				glyphlist[name] = text.String()
			}
		}
	})
	return glyphlist
}
