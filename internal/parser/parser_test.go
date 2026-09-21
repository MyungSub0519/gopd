package parser

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/MyungSub0519/gopd/internal/common/pdftest"
)

// The PDF container is always valid so mutations exercise the content
// interpreter and source concatenation rather than stopping at the file header.
func FuzzBuildPDF(f *testing.F) {
	f.Add([]byte(`0 0 m 10 10 l S`), uint16(7))
	f.Add([]byte(`BT /F 10 Tf [(A) 100 (A)] TJ ET`), uint16(17))
	f.Add([]byte(`q /Fm Do Q /Im Do`), uint16(10))
	f.Add([]byte(`<< /Key [1 2 null] >> /Tag BDC EMC`), uint16(15))
	f.Add([]byte(``), uint16(0))
	f.Fuzz(func(t *testing.T, content []byte, splitAt uint16) {
		if len(content) > 4096 {
			return
		}
		split := int(splitAt) % (len(content) + 1)
		data := semanticFixture(
			`<< /Type /Catalog /Pages 2 0 R >>`,
			`<< /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox [0 0 100 100] >>`,
			`<< /Type /Page /Parent 2 0 R /Contents [4 0 R 5 0 R] /Resources << /Font << /F 7 0 R >> /XObject << /Fm 6 0 R /Im 8 0 R >> >> >>`,
			semanticStream("", string(content[:split])), semanticStream("", string(content[split:])),
			semanticStream(`/Subtype /Form /BBox [10 10 0 0] /Resources << /Font << /F 7 0 R >> >>`, `BT /F 10 Tf (A) Tj ET`),
			`<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding /FirstChar 65 /Widths [600] >>`,
			semanticStream(`/Subtype /Image /Width 1 /Height 1 /ColorSpace /DeviceGray /BitsPerComponent 8`, "A"))
		d, err := Parse(bytes.NewReader(data), int64(len(data)), ReadOptions{Limits: Limits{
			MaxDepth: 12, MaxObjects: 128, MaxValues: 2048, MaxSemanticObjects: 256,
			MaxContentBytes: 32 << 10, MaxDecodedBytes: 32 << 10, MaxTokenBytes: 4096,
		}})
		if err != nil {
			t.Fatalf("generated container failed to parse: %v", err)
		}
		p, _ := BuildPDF(d)
		if p == nil {
			t.Fatal("valid Document must retain a semantic snapshot, including on content errors")
		}
		for _, page := range p.Pages {
			for _, operation := range page.Operations {
				raw, err := d.Bytes(operation.Span)
				if err != nil || string(raw) != operation.Operator {
					t.Fatalf("operation provenance: %q != %q, %v", raw, operation.Operator, err)
				}
			}
		}
		for _, text := range p.Texts {
			if !finiteMatrix(text.Matrix) {
				t.Fatal("nonfinite retained text matrix")
			}
			for _, glyph := range text.Glyphs {
				if !finitePoint(glyph.Origin) || !finitePoint(glyph.Advance) {
					t.Fatal("nonfinite retained glyph")
				}
			}
		}
	})
}

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

func graphicsStateFixture(state, content string, extras ...string) []byte {
	objects := []string{
		`<< /Type /Catalog /Pages 2 0 R >>`,
		`<< /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox [0 0 100 100] >>`,
		`<< /Type /Page /Parent 2 0 R /Contents 4 0 R /Resources << /ExtGState << /G 5 0 R >> >> >>`,
		semanticStream("", content), state,
	}
	return semanticFixture(append(objects, extras...)...)
}

func TestExtGStateRepeatedEntriesConsumeSemanticBudget(t *testing.T) {
	var state strings.Builder
	state.WriteString("<<")
	for i := 0; i < 100; i++ {
		fmt.Fprintf(&state, " /Unused%d null", i)
	}
	state.WriteString(" >>")
	data := graphicsStateFixture(state.String(), strings.Repeat("/G gs ", 10))
	doc, err := Parse(bytes.NewReader(data), int64(len(data)), ReadOptions{Limits: Limits{MaxSemanticObjects: 300}})
	if err != nil {
		t.Fatal(err)
	}
	pdf, err := BuildPDF(doc)
	if !errors.Is(err, ErrLimit) {
		t.Fatalf("repeated resource entries must exhaust semantic work: %v", err)
	}
	if pdf == nil || len(pdf.Diagnostics) != 0 || len(pdf.Pages) != 1 || len(pdf.Pages[0].Operations) != 3 {
		t.Fatalf("expected dictionary work alone to exhaust budget on third operation: %+v", pdf)
	}
}

func TestXObjectRepeatedDictionaryWorkConsumesSemanticBudget(t *testing.T) {
	var unused strings.Builder
	for i := 0; i < 100; i++ {
		fmt.Fprintf(&unused, " /Unused%d null", i)
	}
	for _, subtype := range []string{
		"/Subtype /Image /Width 1 /Height 1 /BitsPerComponent 8 /ColorSpace /DeviceGray",
		"/Subtype /Form /BBox [0 0 1 1]",
	} {
		t.Run(subtype, func(t *testing.T) {
			data := semanticFixture(
				`<< /Type /Catalog /Pages 2 0 R >>`,
				`<< /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox [0 0 100 100] >>`,
				`<< /Type /Page /Parent 2 0 R /Contents 4 0 R /Resources << /XObject << /X 5 0 R >> >> >>`,
				semanticStream("", strings.Repeat("/X Do ", 10)),
				semanticStream(subtype+unused.String(), ""),
			)
			for _, limit := range []int{300, 2000} {
				doc, err := Parse(bytes.NewReader(data), int64(len(data)), ReadOptions{Limits: Limits{MaxSemanticObjects: limit}})
				if err != nil {
					t.Fatal(err)
				}
				pdf, err := BuildPDF(doc)
				if limit == 300 {
					if !errors.Is(err, ErrLimit) {
						t.Fatalf("repeated XObject dictionary scans must exhaust semantic work: %v", err)
					}
					if pdf == nil || len(pdf.Pages) != 1 || len(pdf.Pages[0].Operations) != 3 {
						t.Fatalf("expected dictionary limit on third invocation: %+v", pdf)
					}
				} else if err != nil {
					t.Fatalf("valid repeated XObject with sufficient budget: %v", err)
				}
				if len(pdf.Diagnostics) != 0 {
					t.Fatalf("unexpected diagnostics: %+v", pdf.Diagnostics)
				}
			}
		})
	}
}

func TestExtGStateDashExpansionConsumesValueBudget(t *testing.T) {
	for _, content := range []string{
		strings.Repeat("/G gs 0 0 1 1 re f ", 10),
		"/G gs " + strings.Repeat("0 0 1 1 re f ", 10),
	} {
		// Resource expansion and retained styles must both be bounded: the
		// second case applies the state once but would copy it for every basic graphic.
		data := graphicsStateFixture("<< /D [["+strings.Repeat("1 ", 100)+"] 0] >>", content)
		doc, err := Parse(bytes.NewReader(data), int64(len(data)), ReadOptions{Limits: Limits{MaxValues: 300}})
		if err != nil {
			t.Fatal(err)
		}
		pdf, err := BuildPDF(doc)
		if !errors.Is(err, ErrLimit) {
			t.Fatalf("repeated dash output must exhaust value budget: %v", err)
		}
		if pdf == nil || len(pdf.Graphics) != 1 {
			t.Fatalf("only the first bounded graphic should be retained: %+v", pdf)
		}
	}
}

func TestExtGStateValueBudgetBoundary(t *testing.T) {
	// One gs operand, 30 converted dash entries, four re operands, and
	// 32 style values (30 dash + two gray components) require exactly 67.
	data := graphicsStateFixture("<< /D [["+strings.Repeat("1 ", 30)+"] 0] >>", "/G gs 0 0 1 1 re f")
	for _, limit := range []int{66, 67} {
		doc, err := Parse(bytes.NewReader(data), int64(len(data)), ReadOptions{Limits: Limits{MaxValues: limit}})
		if err != nil {
			t.Fatal(err)
		}
		pdf, err := BuildPDF(doc)
		if limit == 66 && (!errors.Is(err, ErrLimit) || len(pdf.Graphics) != 0) {
			t.Fatalf("over-budget style retained: graphics=%d err=%v", len(pdf.Graphics), err)
		}
		if limit == 67 && (err != nil || len(pdf.Graphics) != 1) {
			t.Fatalf("exact value budget rejected: graphics=%d err=%v", len(pdf.Graphics), err)
		}
	}
}

func TestContentDiagnosticsConsumeBudget(t *testing.T) {
	data := semanticFixture(
		`<< /Type /Catalog /Pages 2 0 R >>`,
		`<< /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox [0 0 100 100] >>`,
		`<< /Type /Page /Parent 2 0 R /Contents 4 0 R >>`,
		semanticStream("", "1 i"))
	for _, limits := range []Limits{
		{MaxSemanticObjects: 4}, // two page visits, one stream, one operator, no diagnostic capacity
		{MaxDecodedBytes: 32},   // input fits, but the expanded diagnostic message does not
	} {
		doc, err := Parse(bytes.NewReader(data), int64(len(data)), ReadOptions{Limits: limits})
		if err != nil {
			t.Fatal(err)
		}
		pdf, err := BuildPDF(doc)
		if !errors.Is(err, ErrLimit) || pdf == nil || len(pdf.Diagnostics) != 0 || len(pdf.Pages[0].Operations) != 1 {
			t.Fatalf("diagnostic must be charged before retention: pdf=%+v err=%v", pdf, err)
		}
	}
}

func TestExtGStateResolvesNestedValues(t *testing.T) {
	// PDF Reference 1.6, 3.2.9: resource values may be indirect unless a
	// direct value is specifically required. Content operands remain direct.
	data := graphicsStateFixture(
		`<< /Font [6 0 R 7 0 R] /D [8 0 R 9 0 R] /LW 10 0 R /RI 11 0 R >>`,
		`q /G gs 0 0 1 1 re S BT (A) Tj ET Q 0 0 1 1 re S`,
		`<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /FirstChar 65 /Widths [600] >>`,
		`10`, `[12 0 R 2]`, `3`, `4`, `/RelativeColorimetric`, `1`)
	pdf, err := Read(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	if len(pdf.Texts) != 1 || pdf.Texts[0].Unicode != "A" || pdf.Texts[0].FontSize != 10 || pdf.Texts[0].Glyphs[0].Advance.X != 6 {
		t.Fatalf("indirect font values lost: %+v", pdf.Texts)
	}
	if len(pdf.Graphics) != 2 {
		t.Fatalf("graphics = %d", len(pdf.Graphics))
	}
	state := pdf.Graphics[0].State
	if !reflect.DeepEqual(state.Dash, []float64{1, 2}) || state.DashPhase != 3 || state.LineWidth != 4 || state.RenderingIntent != "RelativeColorimetric" {
		t.Fatalf("indirect graphics state values lost: %+v", state)
	}
	if restored := pdf.Graphics[1].State; len(restored.Dash) != 0 || restored.LineWidth != 1 {
		t.Fatalf("q/Q must restore the earlier state: %+v", restored)
	}
	loaded, err := pdf.Document.Load(ObjectID{Number: 5})
	if err != nil {
		t.Fatal(err)
	}
	font, err := loaded.Body.Value.(Dictionary).Get("Font")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := font.Value.(Array).Items[1].Value.(Reference); !ok {
		t.Fatal("semantic interpretation replaced the original reference")
	}
}

func TestExtGStateNullEntriesDoNotChangeState(t *testing.T) {
	// PDF Reference 1.6, 3.2.6: dictionary null values are absent entries.
	data := graphicsStateFixture(
		`<< /LW null /D 6 0 R /Font null /RI null /CA null /ca null /BM null /SMask null /Unknown null >>`,
		`2 w [1 2] 3 d /G gs 0 0 1 1 re S`, `null`)
	pdf, err := Read(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	state := pdf.Graphics[0].State
	if state.LineWidth != 2 || !reflect.DeepEqual(state.Dash, []float64{1, 2}) || state.DashPhase != 3 || !state.Complete || len(pdf.Diagnostics) != 0 {
		t.Fatalf("null entries changed graphics state: %+v; diagnostics=%+v", state, pdf.Diagnostics)
	}
}

func TestExtGStateInvalidNestedValues(t *testing.T) {
	for _, state := range []string{
		`<< /D [6 0 R 0] >>`,
		`<< /D [[1 2] 6 0 R] >>`,
		`<< /Font [7 0 R 6 0 R] >>`,
	} {
		for _, value := range []string{`/Bad`, `6 0 R`} {
			data := graphicsStateFixture(state, `/G gs`, value, `<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>`)
			if _, err := Read(bytes.NewReader(data), int64(len(data))); err == nil || errors.Is(err, ErrLimit) {
				t.Fatalf("malformed or cyclic resource must fail without masquerading as a limit: %v", err)
			}
		}
	}
}

func semanticFixture(objects ...string) []byte {
	return pdftest.File(objects, "")
}

func semanticStream(dict, content string) string {
	return pdftest.Stream(dict, content)
}

func semanticRead(t *testing.T, objects ...string) *DetailedPDF {
	t.Helper()
	data := semanticFixture(objects...)
	p, err := Read(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestContentMixedOrderUnicodeAndRepeatedImage(t *testing.T) {
	p := semanticRead(t,
		`<< /Type /Catalog /Pages 2 0 R >>`,
		`<< /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox [0 0 200 300] /Resources << /Font << /F1 5 0 R >> /XObject << /Im1 7 0 R >> >> >>`,
		`<< /Type /Page /Parent 2 0 R /Contents 4 0 R >>`,
		semanticStream("", `2 w 0 0 m 10 10 l S BT /F1 12 Tf 1 0 0 1 20 30 Tm <01> Tj ET q 2 0 0 3 10 20 cm /Im1 Do Q /Im1 Do`),
		`<< /Type /Font /Subtype /TrueType /BaseFont /Test /FirstChar 1 /Widths [500] /ToUnicode 6 0 R >>`,
		semanticStream("", `1 begincodespacerange <00> <FF> endcodespacerange 1 beginbfchar <01> <AC00> endbfchar`),
		semanticStream(`/Type /XObject /Subtype /Image /Width 1 /Height 1 /ColorSpace /DeviceGray /BitsPerComponent 8`, "A"))
	if len(p.Pages) != 1 || len(p.Texts) != 1 || len(p.Graphics) != 1 || len(p.Images) != 2 || len(p.ImageResources) != 1 {
		t.Fatalf("counts: pages %d texts %d graphics %d images %d resources %d", len(p.Pages), len(p.Texts), len(p.Graphics), len(p.Images), len(p.ImageResources))
	}
	if got := p.Texts[0]; got.Unicode != "가" || !got.DecodeComplete || !got.PositionComplete || got.Matrix[4] != 20 || got.Matrix[5] != 30 {
		t.Fatalf("text = %+v", got)
	}
	want := []ElementKind{ElementGraphic, ElementText, ElementImage, ElementImage}
	for i, kind := range want {
		if p.Pages[0].Items[i].Kind != kind {
			t.Fatalf("order = %+v", p.Pages[0].Items)
		}
	}
	if p.Images[0].Matrix != (Matrix{2, 0, 0, 3, 10, 20}) || p.Images[1].Matrix != IdentityMatrix() {
		t.Fatalf("image matrices: %+v", p.Images)
	}
	if p.Graphics[0].State.LineWidth != 2 || p.Pages[0].MediaBox.Max.Y != 300 {
		t.Fatal("graphics state or inherited MediaBox lost")
	}
	if p.Texts[0].Source.Spans[0].Source == p.Structure.File {
		t.Fatal("text provenance must point into decoded content")
	}
}

func TestContentStateAcrossStreamsAndTextAdvance(t *testing.T) {
	p := semanticRead(t, `<< /Type /Catalog /Pages 2 0 R >>`, `<< /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox [0 0 100 100] >>`, `<< /Type /Page /Parent 2 0 R /Resources << /Font << /F 6 0 R >> >> /Contents [4 0 R 5 0 R] >>`, semanticStream("", `q 3 w 0 0 m 1 1 l S Q BT /F 10 Tf 1 0 0 1 10 20 Tm`), semanticStream("", `[(A) 100 (A)] TJ (A) Tj ET 0 0 m 1 1 l S`), `<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding /FirstChar 65 /Widths [600] >>`)
	if len(p.Texts) != 2 || p.Texts[1].Matrix[4] != 21 {
		t.Fatalf("text advancement: %+v", p.Texts)
	}
	if p.Graphics[0].State.LineWidth != 3 || p.Graphics[1].State.LineWidth != 1 {
		t.Fatal("q/Q did not restore state")
	}
}

func TestContentFormsReuseAndCycle(t *testing.T) {
	objects := []string{`<< /Type /Catalog /Pages 2 0 R >>`, `<< /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox [0 0 100 100] >>`, `<< /Type /Page /Parent 2 0 R /Contents 4 0 R /Resources << /XObject << /Fm 5 0 R >> >> >>`, semanticStream("", `/Fm Do /Fm Do`), semanticStream(`/Type /XObject /Subtype /Form /BBox [0 0 1 1] /Matrix [2 0 0 2 0 0]`, `0 0 1 1 re f`)}
	p := semanticRead(t, objects...)
	if len(p.Graphics) != 2 || len(p.Graphics[0].Source.FormPath) != 1 || p.Graphics[0].State.CTM[0] != 2 {
		t.Fatalf("form placements = %+v", p.Graphics)
	}
	objects[4] = semanticStream(`/Type /XObject /Subtype /Form /BBox [0 0 1 1] /Resources << /XObject << /Fm 5 0 R >> >>`, `/Fm Do`)
	data := semanticFixture(objects...)
	_, err := Read(bytes.NewReader(data), int64(len(data)))
	if err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("cycle error = %v", err)
	}
}

func TestContentUnsupportedIsDiagnosedAndUnmappedCodesRetained(t *testing.T) {
	p := semanticRead(t, `<< /Type /Catalog /Pages 2 0 R >>`, `<< /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox [0 0 100 100] >>`, `<< /Type /Page /Parent 2 0 R /Contents 4 0 R /Resources << /Font << /F 5 0 R >> >> >>`, semanticStream("", `BT /F 10 Tf <FF> Tj ET /Unknown sh`), `<< /Type /Font /Subtype /TrueType /BaseFont /Unknown >>`)
	if p.Texts[0].DecodeComplete || !bytes.Equal(p.Texts[0].RawCodes, []byte{255}) || len(p.Diagnostics) == 0 || p.Pages[0].Complete {
		t.Fatalf("unsupported claims: %+v", p)
	}
}

func TestContentStandardEncodingQuotes(t *testing.T) {
	p := semanticRead(t, `<< /Type /Catalog /Pages 2 0 R >>`, `<< /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox [0 0 100 100] >>`, `<< /Type /Page /Parent 2 0 R /Contents 4 0 R /Resources << /Font << /F 5 0 R >> >> >>`, semanticStream("", `BT /F 10 Tf <2760> Tj ET`), `<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /StandardEncoding >>`)
	if p.Texts[0].Unicode != "’‘" || !p.Texts[0].DecodeComplete {
		t.Fatalf("StandardEncoding quotes = %q", p.Texts[0].Unicode)
	}
}

func TestContentHonorsGlyphAndOperandDepthLimits(t *testing.T) {
	for _, content := range []string{`BT /F 10 Tf (` + strings.Repeat("A", 101) + `) Tj ET`, strings.Repeat("[", 20) + strings.Repeat("]", 20) + ` DP`} {
		data := semanticFixture(`<< /Type /Catalog /Pages 2 0 R >>`, `<< /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox [0 0 100 100] >>`, `<< /Type /Page /Parent 2 0 R /Contents 4 0 R /Resources << /Font << /F 5 0 R >> >> >>`, semanticStream("", content), `<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>`)
		d, err := Parse(bytes.NewReader(data), int64(len(data)), ReadOptions{Limits: Limits{MaxObjects: 100, MaxDepth: 8}})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = BuildPDF(d); err == nil || !strings.Contains(err.Error(), "limit") {
			t.Fatalf("resource limit not enforced for %q: %v", content, err)
		}
	}
}

func TestContentBoundsRepeatedEmptyPageTreeNodes(t *testing.T) {
	data := semanticFixture(`<< /Type /Catalog /Pages 2 0 R >>`, `<< /Type /Pages /Kids [3 0 R 3 0 R] /Count 0 >>`, `<< /Type /Pages /Kids [4 0 R 4 0 R] /Count 0 >>`, `<< /Type /Pages /Kids [5 0 R 5 0 R] /Count 0 >>`, `<< /Type /Pages /Kids [6 0 R 6 0 R] /Count 0 >>`, `<< /Type /Pages /Kids [] /Count 0 >>`)
	d, err := Parse(bytes.NewReader(data), int64(len(data)), ReadOptions{Limits: Limits{MaxObjects: 20}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = BuildPDF(d); err == nil || !strings.Contains(err.Error(), "limit") {
		t.Fatalf("tree visit limit = %v", err)
	}
}

func TestContentRejectsDuplicateGraphicsStateEntries(t *testing.T) {
	data := semanticFixture(`<< /Type /Catalog /Pages 2 0 R >>`, `<< /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox [0 0 100 100] >>`, `<< /Type /Page /Parent 2 0 R /Contents 4 0 R /Resources << /ExtGState << /GS << /LW 1 /LW 2 >> >> >> >>`, semanticStream("", `/GS gs`))
	if _, err := Read(bytes.NewReader(data), int64(len(data))); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("duplicate graphics state = %v", err)
	}
}

func TestContentDoesNotClaimCustomCIDPositioning(t *testing.T) {
	for _, font := range []string{`<< /Type /Font /Subtype /Type0 /BaseFont /Test /Encoding /Custom-H /DescendantFonts [6 0 R] /ToUnicode 7 0 R >>`} {
		p := semanticRead(t, `<< /Type /Catalog /Pages 2 0 R >>`, `<< /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox [0 0 100 100] >>`, `<< /Type /Page /Parent 2 0 R /Contents 4 0 R /Resources << /Font << /F 5 0 R >> >> >>`, semanticStream("", `BT /F 10 Tf <01> Tj ET`), font, `<< /Type /Font /Subtype /CIDFontType2 /W [1 [500]] >>`, semanticStream("", `1 beginbfchar <01> <AC00> endbfchar`))
		if p.Texts[0].PositionComplete {
			t.Fatalf("claimed unsupported positioning for %s", font)
		}
	}
}

func TestContentBoundsExpandedFontMaps(t *testing.T) {
	data := semanticFixture(`<< /Type /Catalog /Pages 2 0 R >>`, `<< /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox [0 0 100 100] >>`, `<< /Type /Page /Parent 2 0 R /Contents 4 0 R /Resources << /Font << /F 5 0 R >> >> >>`, semanticStream("", `BT /F 10 Tf (A) Tj ET`), `<< /Type /Font /Subtype /TrueType /ToUnicode 6 0 R >>`, semanticStream("", `1 beginbfrange <00> <64> <0041> endbfrange`))
	d, err := Parse(bytes.NewReader(data), int64(len(data)), ReadOptions{Limits: Limits{MaxObjects: 100}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = BuildPDF(d); err == nil || !strings.Contains(err.Error(), "limit") {
		t.Fatalf("CMap entry budget = %v", err)
	}
}

func TestContentBoundsExpandedUnicodeText(t *testing.T) {
	data := semanticFixture(`<< /Type /Catalog /Pages 2 0 R >>`, `<< /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox [0 0 100 100] >>`, `<< /Type /Page /Parent 2 0 R /Contents 4 0 R /Resources << /Font << /F 5 0 R >> >> >>`, semanticStream("", `BT /F 10 Tf (`+strings.Repeat("A", 300)+`) Tj ET`), `<< /Type /Font /Subtype /TrueType /ToUnicode 6 0 R >>`, semanticStream("", `1 beginbfchar <41> <`+strings.Repeat("0041", 10)+`> endbfchar`))
	d, err := Parse(bytes.NewReader(data), int64(len(data)), ReadOptions{Limits: Limits{MaxDecodedBytes: 1024}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = BuildPDF(d); err == nil || !strings.Contains(err.Error(), "limit") {
		t.Fatalf("expanded Unicode budget = %v", err)
	}
}

func TestContentRejectsNonFiniteDerivedGeometry(t *testing.T) {
	huge := "1" + strings.Repeat("0", 200)
	for _, test := range []struct {
		name    string
		content string
		form    string
	}{
		{"path", huge + " 0 0 1 0 0 cm " + huge + " 0 m 1 1 l S", "0 0 1 1 re f"},
		{"text", "BT /F " + huge + " Tf " + huge + " 0 0 1 0 0 Tm (A) Tj ET", "0 0 1 1 re f"},
		{"form", huge + " 0 0 1 0 0 cm /Fm Do", "0 0 1 1 re f"},
	} {
		t.Run(test.name, func(t *testing.T) {
			data := semanticFixture(
				`<< /Type /Catalog /Pages 2 0 R >>`,
				`<< /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox [0 0 100 100] >>`,
				`<< /Type /Page /Parent 2 0 R /Contents 4 0 R /Resources << /Font << /F 5 0 R >> /XObject << /Fm 6 0 R >> >> >>`,
				semanticStream("", test.content),
				`<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding /FirstChar 65 /Widths [600] >>`,
				semanticStream(`/Type /XObject /Subtype /Form /BBox [0 0 1 1] /Matrix [`+huge+` 0 0 1 0 0]`, test.form))
			if _, err := Read(bytes.NewReader(data), int64(len(data))); err == nil {
				t.Fatal("accepted derived coordinates containing infinity or NaN")
			}
		})
	}
}

func TestContentBoundsCumulativeClipSnapshots(t *testing.T) {
	// There are only 45 operators, but copying the growing clip history requires
	// 105 previous clip references. The configured budget must bound that work.
	data := semanticFixture(
		`<< /Type /Catalog /Pages 2 0 R >>`,
		`<< /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox [0 0 100 100] >>`,
		`<< /Type /Page /Parent 2 0 R /Contents 4 0 R >>`,
		semanticStream("", strings.Repeat(`0 0 1 1 re W n `, 15)))
	doc, err := Parse(bytes.NewReader(data), int64(len(data)), ReadOptions{Limits: Limits{MaxObjects: 100}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := BuildPDF(doc); err == nil || !strings.Contains(err.Error(), "limit") {
		t.Fatalf("cumulative clip snapshot budget not enforced: %v", err)
	}
}

func TestSyntheticPDFClassificationAndProvenance(t *testing.T) {
	const path = "../../testdata/synthetic.pdf"
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
	p, err := ParsePDF("../../testdata/synthetic.pdf")
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
