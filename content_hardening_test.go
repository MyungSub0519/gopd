package gopd

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestContentCompoundOperandAcrossStreams(t *testing.T) {
	p := semanticRead(t,
		`<< /Type /Catalog /Pages 2 0 R >>`,
		`<< /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox [0 0 100 100] >>`,
		`<< /Type /Page /Parent 2 0 R /Resources << /Font << /F 6 0 R >> >> /Contents [4 0 R 5 0 R] >>`,
		semanticStream("", `BT /F 10 Tf [(A) `),
		semanticStream("", `100 (A)] TJ ET`),
		`<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding /FirstChar 65 /Widths [600] >>`)
	if len(p.Texts) != 1 || p.Texts[0].Unicode != "AA" || p.Texts[0].Glyphs[1].Origin.X != 5 {
		t.Fatalf("split TJ = %+v", p.Texts)
	}
	operand := p.Pages[0].Operations[2].Operands[0]
	raw, err := p.Document.Bytes(operand.Span)
	if err != nil || string(raw) != "[(A) 100 (A)]" {
		t.Fatalf("split operand bytes = %q, err=%v", raw, err)
	}
	source := p.Document.Sources[operand.Span.Source]
	if source.Origin == nil || len(source.Origin.Inputs) != 2 || source.Origin.Input != (Span{}) {
		t.Fatalf("joined source provenance = %+v", source.Origin)
	}
	var reconstructed []byte
	for _, input := range source.Origin.Inputs {
		part, err := p.Document.Bytes(input)
		if err != nil {
			t.Fatal(err)
		}
		reconstructed = append(reconstructed, part...)
		if input.Source == source.ID || p.Document.Sources[input.Source].Origin == nil {
			t.Fatal("joined inputs must identify original decoded streams")
		}
	}
	joined, err := p.Document.Bytes(Span{Source: source.ID, End: source.Size})
	if err != nil || !bytes.Equal(joined, reconstructed) {
		t.Fatalf("joined bytes differ from exact input concatenation: %v", err)
	}
}

func TestContentSingleStreamRetainsDecodedSource(t *testing.T) {
	p := semanticRead(t,
		`<< /Type /Catalog /Pages 2 0 R >>`,
		`<< /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox [0 0 100 100] >>`,
		`<< /Type /Page /Parent 2 0 R /Contents [4 0 R] >>`, semanticStream("", `0 0 1 1 re f`))
	source := p.Document.Sources[p.Pages[0].Operations[0].Span.Source]
	if source.Origin == nil || len(source.Origin.Inputs) != 0 || source.Origin.Input.Source != p.Structure.File {
		t.Fatalf("single stream was unnecessarily joined: %+v", source.Origin)
	}
}

func TestContentCumulativeByteBudget(t *testing.T) {
	for _, repeats := range []int{2, 3} {
		t.Run(fmt.Sprint(repeats), func(t *testing.T) {
			data := semanticFixture(
				`<< /Type /Catalog /Pages 2 0 R >>`,
				`<< /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox [0 0 100 100] >>`,
				`<< /Type /Page /Parent 2 0 R /Contents [`+strings.Repeat("4 0 R ", repeats)+`] >>`,
				semanticStream("", strings.Repeat(" ", 64)))
			d, err := Parse(bytes.NewReader(data), int64(len(data)), ReadOptions{Limits: Limits{MaxDecodedBytes: 64, MaxContentBytes: 128}})
			if err != nil {
				t.Fatal(err)
			}
			_, err = BuildPDF(d)
			if repeats == 2 && err != nil {
				t.Fatalf("exact byte budget must pass: %v", err)
			}
			if repeats == 3 && (!errors.Is(err, ErrLimit) || !strings.Contains(err.Error(), "cumulative content byte limit")) {
				t.Fatalf("reused decoded source must count on each visit: %v", err)
			}
		})
	}
}

func TestContentByteBudgetIncludesForms(t *testing.T) {
	content := `/Fm Do /Fm Do`
	data := semanticFixture(
		`<< /Type /Catalog /Pages 2 0 R >>`,
		`<< /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox [0 0 100 100] >>`,
		`<< /Type /Page /Parent 2 0 R /Contents 4 0 R /Resources << /XObject << /Fm 5 0 R >> >> >>`,
		semanticStream("", content),
		semanticStream(`/Subtype /Form /BBox [0 0 1 1]`, strings.Repeat(" ", 64)))
	d, err := Parse(bytes.NewReader(data), int64(len(data)), ReadOptions{Limits: Limits{MaxContentBytes: int64(len(content) + 64)}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := BuildPDF(d); !errors.Is(err, ErrLimit) || !strings.Contains(err.Error(), "cumulative content byte limit") {
		t.Fatalf("Form bytes must be charged for every invocation: %v", err)
	}
}

func TestContentValueBudgetIncludesRepeatedOperands(t *testing.T) {
	// Each d has one array, two entries, and one phase: four values. Content
	// operands share a cumulative budget even though each individual array fits.
	data := semanticFixture(
		`<< /Type /Catalog /Pages 2 0 R >>`,
		`<< /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox [0 0 100 100] >>`,
		`<< /Type /Page /Parent 2 0 R /Contents 4 0 R >>`,
		semanticStream("", strings.Repeat("[1 2] 0 d ", 17)))
	d, err := Parse(bytes.NewReader(data), int64(len(data)), ReadOptions{Limits: Limits{MaxValues: 64}})
	if err != nil {
		t.Fatal(err)
	}
	p, err := BuildPDF(d)
	if err == nil || !strings.Contains(err.Error(), "value count limit") || len(p.Pages[0].Operations) != 16 {
		t.Fatalf("cumulative content values: operations=%d err=%v", len(p.Pages[0].Operations), err)
	}
}

func TestContentBoundsRepeatedWhitespaceStreams(t *testing.T) {
	data := semanticFixture(
		`<< /Type /Catalog /Pages 2 0 R >>`,
		`<< /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox [0 0 100 100] >>`,
		`<< /Type /Page /Parent 2 0 R /Contents [`+strings.Repeat("4 0 R ", 200)+`] >>`,
		semanticStream("", strings.Repeat(" ", 65536)))
	d, err := Parse(bytes.NewReader(data), int64(len(data)), ReadOptions{Limits: Limits{MaxObjects: 20}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := BuildPDF(d); !errors.Is(err, ErrLimit) || !strings.Contains(err.Error(), "semantic object limit") {
		t.Fatalf("repeated stream visit limit = %v", err)
	}
}

func TestContentBoundsAnnotationOccurrences(t *testing.T) {
	data := semanticFixture(
		`<< /Type /Catalog /Pages 2 0 R >>`,
		`<< /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox [0 0 100 100] >>`,
		`<< /Type /Page /Parent 2 0 R /Annots [`+strings.Repeat("4 0 R ", 200)+`] >>`,
		`<< /Type /Annot /Subtype /Text /Rect [0 0 10 10] >>`)
	d, err := Parse(bytes.NewReader(data), int64(len(data)), ReadOptions{Limits: Limits{MaxObjects: 20}})
	if err != nil {
		t.Fatal(err)
	}
	p, err := BuildPDF(d)
	if !errors.Is(err, ErrLimit) || !strings.Contains(err.Error(), "semantic object limit") || len(p.Annotations) > 20 {
		t.Fatalf("annotation limit: annotations=%d err=%v", len(p.Annotations), err)
	}
}

func TestContentSemanticBudgetIsCumulativeAcrossPages(t *testing.T) {
	// Three page-tree visits plus two annotations per page require seven units.
	// Reusing the same annotation array must not reset the cumulative count.
	data := semanticFixture(
		`<< /Type /Catalog /Pages 2 0 R >>`,
		`<< /Type /Pages /Kids [3 0 R 4 0 R] /Count 2 /MediaBox [0 0 100 100] >>`,
		`<< /Type /Page /Parent 2 0 R /Annots 5 0 R >>`,
		`<< /Type /Page /Parent 2 0 R /Annots 5 0 R >>`,
		`[6 0 R 6 0 R]`,
		`<< /Type /Annot /Subtype /Text /Rect [0 0 10 10] >>`)
	for _, budget := range []int{6, 7} {
		d, err := Parse(bytes.NewReader(data), int64(len(data)), ReadOptions{Limits: Limits{MaxObjects: 20, MaxSemanticObjects: budget}})
		if err != nil {
			t.Fatal(err)
		}
		p, err := BuildPDF(d)
		if budget == 6 && (!errors.Is(err, ErrLimit) || len(p.Annotations) != 3) {
			t.Fatalf("excess annotation: count=%d, err=%v", len(p.Annotations), err)
		}
		if budget == 7 && (err != nil || len(p.Annotations) != 4) {
			t.Fatalf("exact semantic budget: count=%d, err=%v", len(p.Annotations), err)
		}
	}
}

func TestContentOptionalNullUsesInheritedDefaults(t *testing.T) {
	for _, value := range []string{"null", "4 0 R"} {
		t.Run(value, func(t *testing.T) {
			p := semanticRead(t,
				`<< /Type /Catalog /Pages 2 0 R >>`,
				`<< /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox [0 0 100 100] /CropBox [1 2 90 80] /Rotate 90 /Resources << >> >>`,
				`<< /Type /Page /Parent 2 0 R /MediaBox `+value+` /CropBox `+value+` /Rotate `+value+` /Resources `+value+` /UserUnit `+value+` /Annots `+value+` >>`,
				`null`)
			page := p.Pages[0]
			if page.MediaBox.Max.X != 100 || page.CropBox.Min.X != 1 || page.Rotate != 90 || page.UserUnit != 1 {
				t.Fatalf("null should act as absent: %+v", page)
			}
		})
	}
}

func TestContentNormalizesRectangleCorners(t *testing.T) {
	for _, rectangle := range []string{"[100 100 0 0]", "[100 0 0 100]", "[0 100 100 0]"} {
		t.Run(rectangle, func(t *testing.T) {
			p := semanticRead(t,
				`<< /Type /Catalog /Pages 2 0 R >>`,
				`<< /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox `+rectangle+` >>`,
				`<< /Type /Page /Parent 2 0 R /CropBox `+rectangle+` /Annots [4 0 R] >>`,
				`<< /Type /Annot /Subtype /Text /Rect `+rectangle+` >>`)
			want := Rect{Max: Point{X: 100, Y: 100}}
			if p.Pages[0].MediaBox != want || p.Pages[0].CropBox != want || p.Annotations[0].Rect != want {
				t.Fatalf("rectangles not normalized: %+v, %+v", p.Pages[0], p.Annotations[0])
			}
		})
	}
}

func TestContentDiagnosesXObjectOptionalVisibility(t *testing.T) {
	for _, subtype := range []string{"Form", "Image"} {
		t.Run(subtype, func(t *testing.T) {
			xobject := semanticStream(`/Type /XObject /Subtype /Form /BBox [0 0 10 10] /OC 6 0 R`, `0 0 1 1 re f`)
			if subtype == "Image" {
				xobject = semanticStream(`/Type /XObject /Subtype /Image /Width 1 /Height 1 /ColorSpace /DeviceGray /BitsPerComponent 8 /OC 6 0 R`, "A")
			}
			p := semanticRead(t,
				`<< /Type /Catalog /Pages 2 0 R /OCProperties << /OCGs [6 0 R] /D << /BaseState /OFF /OFF [6 0 R] >> >> >>`,
				`<< /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox [0 0 100 100] >>`,
				`<< /Type /Page /Parent 2 0 R /Contents 4 0 R /Resources << /XObject << /X 5 0 R >> >> >>`,
				semanticStream("", `/X Do 0 0 1 1 re f`), xobject,
				`<< /Type /OCG /Name (Hidden layer) >>`)
			complete := false
			if subtype == "Image" {
				complete = p.Images[0].State.Complete
			} else {
				complete = p.Graphics[0].State.Complete
			}
			if p.Pages[0].Complete || complete || len(p.Diagnostics) != 1 || !strings.Contains(p.Diagnostics[0].Message, "visibility") {
				t.Fatalf("visibility must be diagnosed: page=%+v diagnostics=%+v", p.Pages[0], p.Diagnostics)
			}
			if !p.Graphics[len(p.Graphics)-1].State.Complete {
				t.Fatal("XObject visibility must not taint the caller's subsequent painting")
			}
		})
	}
}
