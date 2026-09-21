// Command gopd inspects PDF structure and classified page content.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"strings"

	"github.com/MyungSub0519/gopd"
)

type pageSummary struct {
	Page     int  `json:"page"`
	Texts    int  `json:"texts"`
	Graphics int  `json:"graphics"`
	Images   int  `json:"images"`
	Complete bool `json:"interpretation_complete"`
}
type summary struct {
	Pages          int           `json:"pages"`
	Texts          int           `json:"texts"`
	Graphics       int           `json:"graphics"`
	Images         int           `json:"images"`
	ImageResources int           `json:"image_resources"`
	Fonts          int           `json:"fonts"`
	Annotations    int           `json:"annotations"`
	Diagnostics    int           `json:"diagnostics"`
	PageDetails    []pageSummary `json:"page_details"`
}

// pdfparse returns the basic PDF object. Detailed data is available via Details().
func pdfparse(path string) (*gopd.PDF, error) {
	return gopd.ParsePDF(path)
}

func run(args []string, out, stderr io.Writer) int {
	flags := flag.NewFlagSet("gopd", flag.ContinueOnError)
	flags.SetOutput(stderr)
	textOnly := flags.Bool("text", false, "print extracted text in content execution order")
	jsonOutput := flags.Bool("json", false, "print JSON counts without binary resources")
	layoutOutput := flags.Bool("layout", false, "print text reconstructed into visual lines (implies -text)")
	flags.Usage = func() { fmt.Fprintln(stderr, "Usage: gopd [-text | -json | -layout] file.pdf"); flags.PrintDefaults() }
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *jsonOutput && (*textOnly || *layoutOutput) || flags.NArg() != 1 {
		flags.Usage()
		return 2
	}
	if *layoutOutput {
		return printLayout(flags.Arg(0), out, stderr)
	}
	doc, err := pdfparse(flags.Arg(0))
	if err != nil {
		fmt.Fprintln(stderr, "gopd:", err)
		return 1
	}
	detail := doc.Details()
	diagnostics := len(detail.Diagnostics)
	if detail.Structure != nil {
		diagnostics += len(detail.Structure.Diagnostics)
	}
	if *textOnly {
		for i, texts := range doc.Texts {
			if i > 0 {
				fmt.Fprintln(out, "\f")
			}
			for _, text := range texts {
				fmt.Fprintln(out, text.Unicode)
			}
		}
		if diagnostics > 0 {
			fmt.Fprintf(stderr, "gopd: %d diagnostic(s); inspect PDF.Details() for extraction limits\n", diagnostics)
		}
		return 0
	}
	s := summary{Pages: len(detail.Pages), Texts: len(detail.Texts), Graphics: len(detail.Graphics), Images: len(detail.Images), ImageResources: len(detail.ImageResources), Fonts: len(detail.Fonts), Annotations: len(detail.Annotations), Diagnostics: diagnostics}
	images := make([]int, len(detail.Pages))
	for _, image := range detail.Images {
		images[image.Source.Page]++
	}
	for i, page := range detail.Pages {
		s.PageDetails = append(s.PageDetails, pageSummary{
			Page:     i + 1,
			Complete: page.Complete,
			Texts:    len(doc.Texts[i]),
			Graphics: len(doc.Graphics[i]),
			Images:   images[i],
		})
	}
	if *jsonOutput {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		if err := enc.Encode(s); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	}
	fmt.Fprintf(out, "Pages: %d\nTexts: %d\nGraphics: %d\nImages: %d (%d shared resources)\nFonts: %d\nAnnotations: %d\nDiagnostics: %d\n", s.Pages, s.Texts, s.Graphics, s.Images, s.ImageResources, s.Fonts, s.Annotations, s.Diagnostics)
	for _, p := range s.PageDetails {
		fmt.Fprintf(out, "Page %d: texts=%d graphics=%d images=%d interpretation_complete=%t\n", p.Page, p.Texts, p.Graphics, p.Images, p.Complete)
	}
	return 0
}

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

// printLayout reconstructs visual lines from glyph origins: one output line per
// (page, baseline) group, glyphs ordered by x, a space inserted only where the
// advance-based gap exceeds a fraction of the font size.
func printLayout(path string, out, stderr io.Writer) int {
	detail, err := gopd.Open(path)
	if err != nil {
		fmt.Fprintln(stderr, "gopd:", err)
		return 1
	}
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
