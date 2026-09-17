package gopd

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestExtractReviewSkipsExtGStateWithoutTextOrStyles(t *testing.T) {
	data := semanticFixture(
		`<< /Type /Catalog /Pages 2 0 R >>`,
		`<< /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox [0 0 10 10] >>`,
		`<< /Type /Page /Parent 2 0 R /Contents 4 0 R /Resources << /ExtGState << /GS 99 0 R >> >> >>`,
		semanticStream("", `/GS gs 0 0 1 1 re f`),
	)
	got, err := ExtractReader(bytes.NewReader(data), int64(len(data)), ExtractOptions{Content: ContentGraphics})
	if err != nil {
		t.Fatalf("graphics without styles resolved irrelevant ExtGState: %v", err)
	}
	if len(got.Pages[0].Graphics) != 1 || !got.Pages[0].Complete {
		t.Fatalf("requested graphic is incomplete: %+v", got)
	}
}

func TestExtractReviewSkipsUnrequestedShading(t *testing.T) {
	data := fontFixture(`<< /Subtype /Type1 /BaseFont /Helvetica >>`, `/Sh sh BT /F 12 Tf (A) Tj ET`)
	got, err := ExtractReader(bytes.NewReader(data), int64(len(data)), ExtractOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Pages[0].Texts) != 1 || got.Pages[0].Texts[0].Unicode != "A" || !got.Pages[0].Complete {
		t.Fatalf("unselected shading affected Unicode completeness: %+v", got.Diagnostics)
	}
}

func TestExtractReviewValueBudgetOnSkippedStyleOperands(t *testing.T) {
	data := semanticFixture(
		`<< /Type /Catalog /Pages 2 0 R >>`,
		`<< /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox [0 0 10 10] >>`,
		`<< /Type /Page /Parent 2 0 R /Contents 4 0 R >>`,
		semanticStream("", strings.Repeat(`[1 2] 0 d `, 17)),
	)
	got, err := ExtractReader(bytes.NewReader(data), int64(len(data)), ExtractOptions{
		Provenance:  true,
		ReadOptions: ReadOptions{Limits: Limits{MaxValues: 64}},
	})
	if !errors.Is(err, ErrLimit) || got == nil || len(got.Pages) != 1 {
		t.Fatalf("skipped operands bypassed budget: result=%+v err=%v", got, err)
	}
	if got.Pages[0].Complete || len(got.Pages[0].Operations) != 16 {
		t.Fatalf("unexpected partial state: %+v", got.Pages[0])
	}
}

func TestExtractReviewLexicalErrorKeepsPartialOutput(t *testing.T) {
	data := semanticFixture(
		`<< /Type /Catalog /Pages 2 0 R >>`,
		`<< /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox [0 0 10 10] >>`,
		`<< /Type /Page /Parent 2 0 R /Contents 4 0 R >>`,
		semanticStream("", `0 0 1 1 re f (unterminated`),
	)
	got, err := ExtractReader(bytes.NewReader(data), int64(len(data)), ExtractOptions{Content: ContentGraphics})
	if err == nil || got == nil || len(got.Pages) != 1 || len(got.Pages[0].Graphics) != 1 || got.Pages[0].Complete {
		t.Fatalf("expected graphic before lexical error: result=%+v err=%v", got, err)
	}
	legacy, err := Read(bytes.NewReader(data), int64(len(data)))
	if err == nil || legacy == nil || len(legacy.Graphics) != 0 {
		t.Fatalf("legacy full-lex behavior changed: result=%+v err=%v", legacy, err)
	}
}

func TestExtractReviewProvenanceOwnsSnapshot(t *testing.T) {
	data := fontFixture(`<< /Subtype /Type1 /BaseFont /Helvetica >>`, `BT /F 12 Tf (A) Tj ET`)
	got, err := ExtractReader(bytes.NewReader(data), int64(len(data)), ExtractOptions{Provenance: true})
	if err != nil {
		t.Fatal(err)
	}
	clear(data)
	text := got.Pages[0].Texts[0]
	if text.Unicode != "A" || text.Source == nil || got.Document == nil {
		t.Fatalf("missing owned extraction: %+v", text)
	}
	operand, err := got.Document.Bytes(text.Source.Spans[0])
	if err != nil || string(operand) != "(A)" {
		t.Fatalf("source snapshot changed with caller buffer: %q, %v", operand, err)
	}
}
