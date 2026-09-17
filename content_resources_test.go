package gopd

import (
	"bytes"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

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
