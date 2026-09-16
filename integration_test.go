package gopd

import (
	"bytes"
	"os"
	"slices"
	"testing"
)

func TestSyntheticPDFClassificationAndProvenance(t *testing.T) {
	const path = "testdata/synthetic.pdf"
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Pages) != 2 {
		t.Fatalf("pages=%d, want 2", len(doc.Pages))
	}
	if len(doc.Texts) != 4 || len(doc.Graphics) != 3 || len(doc.Images) != 2 || len(doc.ImageResources) != 1 {
		t.Fatalf("texts=%d graphics=%d images=%d resources=%d", len(doc.Texts), len(doc.Graphics), len(doc.Images), len(doc.ImageResources))
	}
	var texts []string
	for _, text := range doc.Texts {
		texts = append(texts, text.Unicode)
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
	want := []string{"GoPD synthetic fixture", "Alpha beta 123", "Page two: shared resources", "Reusable form"}
	if !slices.Equal(texts, want) {
		t.Fatalf("text order = %q, want %q", texts, want)
	}
	if len(doc.Fonts) != 1 || len(doc.Diagnostics) != 0 || len(doc.Structure.Diagnostics) != 0 {
		t.Fatal("synthetic fixture must use one shared font without diagnostics")
	}
	if doc.Images[0].Resource != doc.Images[1].Resource {
		t.Fatal("image resource was not reused across pages")
	}
	resource := doc.ImageResources[0]
	if resource.Width != 2 || resource.Height != 2 || resource.BitsPerComponent != 8 {
		t.Fatal("unexpected synthetic image dimensions")
	}
	source, err := doc.Document.DecodeStream(resource.Stream)
	if err != nil {
		t.Fatal(err)
	}
	pixels, err := doc.Document.Bytes(Span{Source: source.ID, End: source.Size})
	if err != nil || !bytes.Equal(pixels, []byte{255, 0, 0, 0, 255, 0, 0, 0, 255, 255, 255, 255}) {
		t.Fatalf("synthetic RGB pixels = %x, error = %v", pixels, err)
	}
	if len(doc.Texts[3].Source.FormPath) != 1 {
		t.Fatal("form text lost its call provenance")
	}
	counts := map[ElementKind]int{}
	for pageIndex, page := range doc.Pages {
		if !page.Complete {
			t.Fatalf("page %d interpretation is incomplete", pageIndex)
		}
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

func TestBasicSyntheticPDF(t *testing.T) {
	p, err := ParsePDF("testdata/synthetic.pdf")
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Texts) != 2 || len(p.Graphics) != 2 || len(p.Details().Images) != 2 || len(p.Details().Diagnostics) != 0 {
		t.Fatal("basic fixture classification changed")
	}
	wantTexts := []int{2, 2}
	wantGraphics := []int{2, 1}
	detailIndex := 0
	for page, texts := range p.Texts {
		if len(texts) != wantTexts[page] || len(p.Graphics[page]) != wantGraphics[page] {
			t.Fatalf("page %d content counts changed", page)
		}
		for i, text := range texts {
			if text.Unicode != p.Details().Texts[detailIndex].Unicode || text.Page != page || text.Font == nil || text.Font != p.Texts[0][0].Font {
				t.Fatalf("page %d text %d content or shared font changed", page, i)
			}
			detailIndex++
		}
	}
	assertBasicJSON(t, p)
}
