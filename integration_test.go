package gopd

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestUserPDFClassificationAndProvenance(t *testing.T) {
	const path = "테스트PDF.pdf"
	before, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		t.Skip("user fixture is not present")
	}
	if err != nil {
		t.Fatal(err)
	}
	doc, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Pages) != 3 {
		t.Fatalf("pages=%d, want3", len(doc.Pages))
	}
	if len(doc.Texts) == 0 || len(doc.Graphics) == 0 || len(doc.Images) != 1 || len(doc.ImageResources) != 1 {
		t.Fatalf("texts=%d graphics=%d images=%d resources=%d", len(doc.Texts), len(doc.Graphics), len(doc.Images), len(doc.ImageResources))
	}
	var all strings.Builder
	for _, text := range doc.Texts {
		all.WriteString(text.Unicode)
		if !text.DecodeComplete || !text.PositionComplete {
			t.Fatalf("fixture text was not fully mapped: %q", text.Unicode)
		}
		if len(text.Source.Spans) == 0 {
			t.Fatal("text lost source provenance")
		}
		for _, span := range text.Source.Spans {
			raw, e := doc.Document.Bytes(span)
			if e != nil || len(raw) == 0 {
				t.Fatalf("invalid text source span: %+v %v", span, e)
			}
		}
	}
	// Literal title independently identified in the supplied fixture's content.
	for _, want := range []string{"열 람 용", "등기사항전부증명서(말소사항 포함)"} {
		if !strings.Contains(all.String(), want) {
			t.Fatalf("Korean title %q missing", want)
		}
	}
	counts := map[ElementKind]int{}
	for pageIndex, page := range doc.Pages {
		for _, item := range page.Items {
			var actualPage int
			switch item.Kind {
			case ElementText:
				if item.Index < 0 || item.Index >= len(doc.Texts) {
					t.Fatal("invalid text index")
				}
				actualPage = doc.Texts[item.Index].Source.Page
			case ElementGraphic:
				if item.Index < 0 || item.Index >= len(doc.Graphics) {
					t.Fatal("invalid graphic index")
				}
				actualPage = doc.Graphics[item.Index].Source.Page
			case ElementImage:
				if item.Index < 0 || item.Index >= len(doc.Images) {
					t.Fatal("invalid image index")
				}
				actualPage = doc.Images[item.Index].Source.Page
			default:
				t.Fatal("unknown element reference")
			}
			if actualPage != pageIndex {
				t.Fatal("element belongs to another page")
			}
			counts[item.Kind]++
		}
	}
	if counts[ElementText] != len(doc.Texts) || counts[ElementGraphic] != len(doc.Graphics) || counts[ElementImage] != len(doc.Images) {
		t.Fatal("page order list omitted classified elements")
	}
	var end int64
	for _, region := range doc.Structure.Regions {
		if region.Span.Source != 1 || region.Span.Start != end || region.Span.End <= end {
			t.Fatalf("overlapping/missing file region %+v", region)
		}
		end = region.Span.End
	}
	if end != int64(len(before)) {
		t.Fatal("regions do not cover original file")
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("inspection changed the PDF fixture")
	}
	t.Logf("pages=%d texts=%d graphics=%d images=%d fonts=%d diagnostics=%d", len(doc.Pages), len(doc.Texts), len(doc.Graphics), len(doc.Images), len(doc.Fonts), len(doc.Diagnostics))
}
