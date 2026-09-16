// Command gopd inspects PDF structure and classified page content.
//
// Usage:
//
//	gopd file.pdf          counts per document and per page
//	gopd -text file.pdf     extracted text, in content execution order
//	gopd -json file.pdf     the same counts as JSON
//
// The text output is in drawing order, not reading order: this tool does not
// reconstruct columns or paragraphs. Diagnostics are counted but not listed;
// the library's Details result holds them in full.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/MyungSub0519/gopd"
)

// pageSummary and summary are the JSON shape of the report. They exist so the
// -json output has a declared schema rather than being assembled ad hoc.
type pageSummary struct {
	Page     int  `json:"page"`
	Texts    int  `json:"texts"`
	Graphics int  `json:"graphics"`
	Images   int  `json:"images"`
	Complete bool `json:"interpretation_complete"`
}

// summary is the whole-document report; PageDetails breaks it down per page.
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

// pdfparse wraps the library entry point so that tests can exercise the
// failure path without going through run.
func pdfparse(path string) (*gopd.PDF, error) {
	return gopd.ParsePDF(path)
}

// run is main's body, with its streams and arguments injected so it can be
// tested. The return value is the process exit code: 0 on success, 1 for a
// failure to read the document, 2 for a usage error.
func run(args []string, out, stderr io.Writer) int {
	flags := flag.NewFlagSet("gopd", flag.ContinueOnError)
	flags.SetOutput(stderr)
	textOnly := flags.Bool("text", false, "print extracted text in content execution order")
	jsonOutput := flags.Bool("json", false, "print JSON counts without binary resources")
	flags.Usage = func() { fmt.Fprintln(stderr, "Usage: gopd [-text | -json] file.pdf"); flags.PrintDefaults() }
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 1 || (*textOnly && *jsonOutput) {
		flags.Usage()
		return 2
	}
	doc, err := pdfparse(flags.Arg(0))
	if err != nil {
		fmt.Fprintln(stderr, "gopd:", err)
		return 1
	}
	// Diagnostics come from two places: the content interpreter's findings,
	// and the structural ones the reader collected. Both matter to a caller
	// judging how much of the document was understood.
	detail := doc.Details()
	diagnostics := len(detail.Diagnostics)
	if detail.Structure != nil {
		diagnostics += len(detail.Structure.Diagnostics)
	}
	if *textOnly {
		for i, texts := range doc.Texts {
			// Pages are separated by a form feed, so downstream tools can
			// split the output back into pages.
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
	// Images are stored document-wide, so count them back per page.
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
