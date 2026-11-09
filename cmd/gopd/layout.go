package main

import (
	"fmt"
	"github.com/MyungSub0519/gopd"
	"io"
	"math"
	"sort"
	"strings"
)

// printLayout reconstructs visual lines from glyph origins: one output line per
// (page, baseline) group, glyphs ordered by x, a space inserted only where the
// advance-based gap exceeds a fraction of the font size.
func printLayout(path string, out, stderr io.Writer) (status int) {
	result, err := gopd.ParseFile(path, gopd.ParseOptions{Content: gopd.ContentText, Detail: gopd.DetailFull})
	if result != nil {
		defer func() {
			if err := result.Close(); err != nil {
				fmt.Fprintln(stderr, "gopd:", err)
				status = 1
			}
		}()
	}
	if err != nil {
		fmt.Fprintln(stderr, "gopd:", err)
		return 1
	}
	detail := result.Details()
	type lineKey struct {
		page int
		y    int
	}
	type glyph struct {
		x, end, size float64
		s            string
	}
	lines := map[lineKey][]glyph{}
	for _, t := range detail.Texts {
		for _, gl := range t.Glyphs {
			k := lineKey{t.Source.Page, int(math.Round(gl.Origin.Y))}
			lines[k] = append(lines[k], glyph{gl.Origin.X, gl.Origin.X + gl.Advance.X, t.FontSize, gl.Unicode})
		}
	}
	keys := make([]lineKey, 0, len(lines))
	for k := range lines {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].page != keys[j].page {
			return keys[i].page < keys[j].page
		}
		return keys[i].y > keys[j].y
	})
	lastPage := -1
	for _, k := range keys {
		row := lines[k]
		sort.Slice(row, func(i, j int) bool { return row[i].x < row[j].x })
		if lastPage >= 0 && k.page != lastPage {
			fmt.Fprintln(out, "\f")
		}
		lastPage = k.page
		var b strings.Builder
		prevEnd := math.Inf(1)
		prevSpace := true
		for _, g := range row {
			if gap := g.x - prevEnd; gap > g.size*0.5 && !prevSpace {
				b.WriteString(" ")
			}
			b.WriteString(g.s)
			prevSpace = strings.TrimSpace(g.s) == ""
			prevEnd = g.end
		}
		fmt.Fprintln(out, strings.TrimRight(b.String(), " "))
	}
	return 0
}
