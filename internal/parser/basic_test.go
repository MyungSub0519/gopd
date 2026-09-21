package parser

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func parseBasicFixture(t *testing.T, objects ...string) *PDF {
	t.Helper()
	path := filepath.Join(t.TempDir(), "input.pdf")
	if err := os.WriteFile(path, semanticFixture(objects...), 0600); err != nil {
		t.Fatal(err)
	}
	p, err := ParsePDF(path)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestBasicContentGroupedByPage(t *testing.T) {
	p := parseBasicFixture(t,
		`<< /Type /Catalog /Pages 2 0 R >>`,
		`<< /Type /Pages /Kids [3 0 R 4 0 R 5 0 R] /Count 3 /MediaBox [0 0 100 100] /Resources << /Font << /F 8 0 R >> >> >>`,
		`<< /Type /Page /Parent 2 0 R /Contents 6 0 R >>`,
		`<< /Type /Page /Parent 2 0 R >>`,
		`<< /Type /Page /Parent 2 0 R /Contents 7 0 R >>`,
		semanticStream("", `0 0 m 1 1 l S BT /F 12 Tf (A) Tj (B) Tj ET`),
		semanticStream("", `BT /F 12 Tf (C) Tj ET 10 10 20 20 re f`),
		`<< /Type /Font /Subtype /Type1 /BaseFont /Test /Encoding /WinAnsiEncoding /FirstChar 65 /Widths [600 600 600] >>`)
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatal(err)
	}
	if len(root) != 2 || root["Texts"] == nil || root["Graphics"] == nil {
		t.Fatalf("basic response must expose only Texts and Graphics; got %d fields", len(root))
	}
	var result struct {
		Texts    [][]Text
		Graphics [][]Graphic
	}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("content must be grouped by page: %v", err)
	}
	if len(result.Texts) != 3 || len(result.Graphics) != 3 {
		t.Fatal("page slots were lost")
	}
	wantText := [][]string{{"A", "B"}, {}, {"C"}}
	wantGraphics := []int{1, 0, 1}
	for page := range result.Texts {
		if result.Texts[page] == nil || result.Graphics[page] == nil {
			t.Fatalf("page %d must contain arrays, including empty pages", page)
		}
		if len(result.Texts[page]) != len(wantText[page]) || len(result.Graphics[page]) != wantGraphics[page] {
			t.Fatalf("wrong content grouping on page %d", page)
		}
		for i, text := range result.Texts[page] {
			if text.Unicode != wantText[page][i] || text.Page != page {
				t.Fatalf("page %d text %d = %+v", page, i, text)
			}
		}
		for _, graphic := range result.Graphics[page] {
			if graphic.Page != page {
				t.Fatalf("graphic assigned to wrong page: %+v", graphic)
			}
		}
	}
	if !result.Graphics[0][0].Stroke || !result.Graphics[2][0].Fill {
		t.Fatal("page-specific graphics were reordered")
	}
	if p.Texts[0][0].Font != p.Texts[2][0].Font {
		t.Fatal("shared font identity was lost across pages")
	}
}

func TestBasicContentOrderAndSharedResources(t *testing.T) {
	p := parseBasicFixture(t,
		`<< /Type /Catalog /Pages 2 0 R >>`,
		`<< /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox [0 0 200 300] >>`,
		`<< /Type /Page /Parent 2 0 R /Contents 4 0 R /Resources << /Font << /F1 5 0 R /F2 7 0 R >> /XObject << /Im 6 0 R >> >> >>`,
		semanticStream("", `2 0 0 3 10 20 cm 2 w 0 0 m 10 10 l S BT /F1 12 Tf 1 0 0 1 20 30 Tm (A) Tj (A) Tj /F2 14 Tf (A) Tj ET /Im Do /Im Do`),
		`<< /Type /Font /Subtype /Type1 /BaseFont /SameName /Encoding /WinAnsiEncoding /FirstChar 65 /Widths [600] >>`,
		semanticStream(`/Type /XObject /Subtype /Image /Width 1 /Height 1 /ColorSpace /DeviceGray /BitsPerComponent 8`, "A"),
		`<< /Type /Font /Subtype /Type1 /BaseFont /SameName /Encoding /WinAnsiEncoding /FirstChar 65 /Widths [700] >>`)
	if len(p.Texts) != 1 || len(p.Graphics) != 1 || len(p.Texts[0]) != 3 || len(p.Graphics[0]) != 1 {
		t.Fatal("classified content was lost")
	}
	detail := p.Details()
	if detail == nil || len(detail.Pages) != 1 || len(detail.Images) != 2 {
		t.Fatal("detailed page or image content was discarded")
	}
	wantItems := []ElementRef{{ElementGraphic, 0}, {ElementText, 0}, {ElementText, 1}, {ElementText, 2}, {ElementImage, 0}, {ElementImage, 1}}
	if !reflect.DeepEqual(detail.Pages[0].Items, wantItems) {
		t.Fatalf("drawing order = %+v", detail.Pages[0].Items)
	}
	first := p.Texts[0][0]
	if first.Page != 0 || first.Unicode != "A" || first.Font == nil || first.Font.BaseFont != "SameName" || first.FontSize != 12 || !first.DecodeComplete || !first.PositionComplete {
		t.Fatalf("basic text = %+v", first)
	}
	if first.Matrix != (Matrix{2, 0, 0, 3, 50, 110}) {
		t.Fatalf("text placement = %v", first.Matrix)
	}
	if first.Font != p.Texts[0][1].Font || first.Font == p.Texts[0][2].Font {
		t.Fatal("fonts must share by resource identity, not by name")
	}
	graphic := p.Graphics[0][0]
	if graphic.Page != 0 || !graphic.Stroke || graphic.Fill || graphic.Style.LineWidth != 2 || graphic.Segments[1].Points[0] != (Point{X: 30, Y: 50}) {
		t.Fatalf("graphic placement or style changed: %+v", graphic)
	}
	if detail.Images[0].Resource != detail.Images[1].Resource || detail.Images[0].Matrix != (Matrix{2, 0, 0, 3, 10, 20}) {
		t.Fatal("image identity or placement changed")
	}
	if len(detail.Fonts) != 2 || len(detail.ImageResources) != 1 || len(detail.Pages[0].Operations) == 0 {
		t.Fatal("detailed parsing information was discarded")
	}
	if raw, err := detail.Document.Bytes(detail.Texts[0].Source.Spans[0]); err != nil || len(raw) == 0 {
		t.Fatal("detailed byte provenance was lost")
	}
	assertBasicJSON(t, p)

	// Editing a projected slice must not corrupt the detailed snapshot.
	p.Graphics[0][0].Segments[1].Points[0].X = 999
	p.Graphics[0][0].Style.Stroke.Components[0] = 0.5
	if detail.Graphics[0].Segments[1].Points[0].X != 30 || detail.Graphics[0].State.Stroke.Components[0] != 0 {
		t.Fatal("basic mutable slices alias detailed data")
	}
}

func assertBasicJSON(t *testing.T, p *PDF) {
	t.Helper()
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatal(err)
	}
	if len(root) != 2 {
		t.Fatalf("basic root exposes %d fields", len(root))
	}
	for _, key := range []string{"Texts", "Graphics"} {
		if len(root[key]) == 0 || root[key][0] != '[' {
			t.Fatalf("%s must be a JSON array", key)
		}
	}
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatal(err)
	}
	var check func(any)
	check = func(value any) {
		switch value := value.(type) {
		case map[string]any:
			for key, child := range value {
				switch key {
				case "Document", "Structure", "Sources", "Source", "Span", "Object", "Operations", "RawCodes", "Glyphs", "ToUnicode", "Stream", "Fonts", "ImageResources", "Annotations":
					t.Errorf("detailed field %s leaked into basic JSON", key)
				}
				check(child)
			}
		case []any:
			for _, child := range value {
				check(child)
			}
		}
	}
	check(value)
}

func TestBasicEmptyContentUsesArrays(t *testing.T) {
	p := parseBasicFixture(t, `<< /Type /Catalog /Pages 2 0 R >>`, `<< /Type /Pages /Kids [] /Count 0 >>`)
	assertBasicJSON(t, p)
	if p.Texts == nil || p.Graphics == nil || len(p.Texts) != 0 || len(p.Graphics) != 0 {
		t.Fatal("a document without pages must return empty arrays")
	}
	if p.Details() == nil || p.Details().Structure.Header.Version.Major != 1 {
		t.Fatal("empty content lost its detailed file structure")
	}
}

func TestBasicDiagnosticsKeepPageForRepeatedForms(t *testing.T) {
	p := parseBasicFixture(t,
		`<< /Type /Catalog /Pages 2 0 R >>`,
		`<< /Type /Pages /Kids [3 0 R 4 0 R] /Count 2 /MediaBox [0 0 100 100] /Resources << /XObject << /Fm 6 0 R >> >> >>`,
		`<< /Type /Page /Parent 2 0 R /Contents 5 0 R >>`,
		`<< /Type /Page /Parent 2 0 R /Contents 5 0 R >>`,
		semanticStream("", `/Fm Do`),
		semanticStream(`/Type /XObject /Subtype /Form /BBox [0 0 10 10]`, `1 i 0 0 1 1 re f`))
	detail := p.Details()
	if len(detail.Diagnostics) != 2 || !reflect.DeepEqual(detail.diagnosticPages, []int{0, 1}) {
		t.Fatalf("shared source diagnostic pages = %+v", detail.diagnosticPages)
	}
	for i, issue := range detail.Diagnostics {
		if issue.Severity != SeverityWarning || issue.Code != "unsupported-content-effect" || detail.Pages[i].Complete || !p.Graphics[i][0].Style.Clipped || p.Graphics[i][0].Style.Complete {
			t.Fatalf("diagnostic or state was lost: %+v", issue)
		}
	}
	if len(p.Details().Graphics[0].State.Clip) == 0 {
		t.Fatal("clipping geometry must remain available in Details")
	}
}

func TestBasicFontDiagnosticKeepsByteDetailsInDetailedResult(t *testing.T) {
	p := parseBasicFixture(t,
		`<< /Type /Catalog /Pages 2 0 R >>`,
		`<< /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox [0 0 100 100] >>`,
		`<< /Type /Page /Parent 2 0 R /Contents 4 0 R /Resources << /Font << /F 5 0 R >> >> >>`,
		semanticStream("", `BT /F 12 Tf (A) Tj ET`),
		`<< /Type /Font /Subtype /TrueType /BaseFont /Test /Encoding /WinAnsiEncoding /FirstChar 65 /Widths [600] /ToUnicode 6 0 R >>`,
		semanticStream("", "("))
	detail := p.Details()
	if len(detail.Diagnostics) != 1 || detail.Diagnostics[0].Code != "unsupported-tounicode" {
		t.Fatalf("font diagnostic missing: %+v", detail.Diagnostics)
	}
	if !strings.Contains(p.Details().Diagnostics[0].Message, "unterminated literal string") || !strings.Contains(p.Details().Diagnostics[0].Message, "byte") {
		t.Fatal("detailed diagnostic lost its original cause/location")
	}
	if p.Texts[0][0].Unicode != "A" || !p.Texts[0][0].DecodeComplete {
		t.Fatal("supported encoding fallback was lost")
	}
	assertBasicJSON(t, p)
}

func TestParsePDFErrorsAndPartialResult(t *testing.T) {
	if p, err := ParsePDF(filepath.Join(t.TempDir(), "missing.pdf")); p != nil || !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing file: result=%v error=%v", p, err)
	}
	path := filepath.Join(t.TempDir(), "bad.pdf")
	if err := os.WriteFile(path, []byte("not a PDF"), 0600); err != nil {
		t.Fatal(err)
	}
	if p, err := ParsePDF(path); p != nil || err == nil {
		t.Fatal("malformed input must return an error")
	}
	data := semanticFixture(`<< /Type /Catalog /Pages 2 0 R >>`, `<< /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox [0 0 100 100] >>`, `<< /Type /Page /Parent 2 0 R /Contents 4 0 R >>`, semanticStream("", `0 0 m 1 1 l S BI`))
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	p, err := ParsePDF(path)
	if err == nil || !strings.Contains(err.Error(), "inline image") || p == nil || len(p.Graphics) != 1 || len(p.Graphics[0]) != 1 || p.Details() == nil {
		t.Fatalf("semantic partial failure was hidden: result=%v error=%v", p, err)
	}
}
