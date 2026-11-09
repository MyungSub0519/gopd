package parser

import (
	_ "embed"
	"strconv"
	"strings"
	"sync"
	"unicode/utf16"
)

// glyphlistData is the Adobe Glyph List, which maps glyph names to Unicode.
// It is embedded so that /Differences names resolve without file access.
//
//go:embed glyphlist.txt
var glyphlistData string

// glyphlist parses the embedded list on first use. Each non-comment line is
// "name;code ..." with one hexadecimal UTF-16 code unit per value.
var glyphlist = sync.OnceValue(func() map[string]string {
	names := make(map[string]string, 4400)
	for line := range strings.SplitSeq(glyphlistData, "\n") {
		name, codes, found := strings.Cut(line, ";")
		if !found || strings.HasPrefix(name, "#") {
			continue
		}
		var units []uint16
		for code := range strings.FieldsSeq(codes) {
			n, err := strconv.ParseUint(code, 16, 16)
			if err != nil {
				units = nil
				break
			}
			units = append(units, uint16(n))
		}
		if len(units) > 0 {
			names[name] = string(utf16.Decode(units))
		}
	}
	return names
})
