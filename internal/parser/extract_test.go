package parser

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// File entry points compare the user-visible APIs on the checked-in fixture.
func BenchmarkExtract(b *testing.B) {
	for _, mode := range []string{"legacy", "text", "all"} {
		b.Run(mode, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				var err error
				if mode == "legacy" {
					_, err = ParsePDF("../../testdata/synthetic.pdf")
				} else {
					options := ParseOptions{}
					if mode == "all" {
						options = ParseOptions{Content: ContentAll, Positions: true, Styles: true, Glyphs: true}
					}
					_, err = ParseFile("../../testdata/synthetic.pdf", options)
				}
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkExtractDense(b *testing.B) {
	data := semanticFixture(
		`<< /Type /Catalog /Pages 2 0 R >>`,
		`<< /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox [0 0 100 100] >>`,
		`<< /Type /Page /Parent 2 0 R /Contents 4 0 R /Resources << /Font << /F 5 0 R >> >> >>`,
		semanticStream("", strings.Repeat(`BT /F 12 Tf (AAAAAAAAAAAAAAAAAAAA) Tj ET 0 0 10 10 re f `, 2000)),
		`<< /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding /FirstChar 65 /Widths [600] >>`,
	)
	for _, mode := range []string{"legacy", "text", "all"} {
		b.Run(mode, func(b *testing.B) {
			parse := func() (any, error) {
				reader := bytes.NewReader(data)
				if mode == "legacy" {
					detail, err := Read(reader, int64(len(data)))
					return basicPDF(detail), err
				}
				options := ParseOptions{}
				if mode == "all" {
					options = ParseOptions{Content: ContentAll, Positions: true, Styles: true, Glyphs: true}
				}
				return ParseReader(reader, int64(len(data)), options)
			}
			output, err := parse()
			if err != nil {
				b.Fatal(err)
			}
			encoded, err := json.Marshal(output)
			if err != nil {
				b.Fatal(err)
			}
			jsonBytes := len(encoded)
			b.ReportAllocs()
			for b.Loop() {
				if _, err := parse(); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportMetric(float64(jsonBytes), "json-bytes")
		})
	}
}

func FuzzExtractReader(f *testing.F) {
	f.Add([]byte(`BT /F 10 Tf [(A) 100 (A)] TJ ET`), uint16(17), byte(1), byte(0))
	f.Add([]byte(`q 2 0 0 2 0 0 cm /Fm Do Q /Im Do`), uint16(10), byte(7), byte(15))
	f.Add([]byte(`0 0 m 10 10 l S (unterminated`), uint16(9), byte(2), byte(8))
	f.Add([]byte(`[1 2 3] 0 d /Sh sh`), uint16(4), byte(1), byte(0))
	f.Fuzz(func(t *testing.T, content []byte, splitAt uint16, mask, flags byte) {
		if len(content) > 4096 {
			return
		}
		split := int(splitAt) % (len(content) + 1)
		data := semanticFixture(
			`<< /Type /Catalog /Pages 2 0 R >>`,
			`<< /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox [0 0 100 100] >>`,
			`<< /Type /Page /Parent 2 0 R /Contents [4 0 R 5 0 R] /Resources << /Font << /F 7 0 R >> /XObject << /Fm 6 0 R /Im 8 0 R >> >> >>`,
			semanticStream("", string(content[:split])), semanticStream("", string(content[split:])),
			semanticStream(`/Subtype /Form /BBox [0 0 10 10]`, `BT /F 10 Tf (A) Tj ET 0 0 1 1 re f`),
			`<< /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding /FirstChar 65 /Widths [600] >>`,
			semanticStream(`/Subtype /Image /Width 1 /Height 1 /ColorSpace /DeviceGray /BitsPerComponent 8`, "A"),
		)
		kind := ContentKind(mask) & ContentAll
		if kind == 0 {
			kind = ContentText
		}
		options := ParseOptions{
			Content: kind, Positions: flags&1 != 0, Styles: flags&2 != 0,
			Glyphs: flags&4 != 0 && kind&ContentText != 0, Provenance: flags&8 != 0,
			ReadOptions: ReadOptions{Limits: Limits{
				MaxDepth: 12, MaxObjects: 128, MaxValues: 2048, MaxSemanticObjects: 256,
				MaxContentBytes: 32 << 10, MaxDecodedBytes: 32 << 10, MaxTokenBytes: 4096,
			}},
		}
		result, err := ParseReader(bytes.NewReader(data), int64(len(data)), options)
		if result == nil {
			t.Fatalf("valid container lost partial result: %v", err)
		}
		if (result.Document != nil) != options.Provenance {
			t.Fatal("source retention disagrees with options")
		}
		for _, page := range result.Pages {
			if kind&ContentText == 0 && len(page.Texts) != 0 {
				t.Fatal("unselected text retained")
			}
			if kind&ContentGraphics == 0 && len(page.Graphics) != 0 {
				t.Fatal("unselected graphics retained")
			}
			if kind&ContentImages == 0 && len(page.Images) != 0 {
				t.Fatal("unselected images retained")
			}
			for _, text := range page.Texts {
				if text.Position != nil && !finiteMatrix(text.Position.Matrix) {
					t.Fatal("nonfinite text matrix")
				}
				for _, glyph := range text.Glyphs {
					if !finitePoint(glyph.Origin) || !finitePoint(glyph.Advance) {
						t.Fatal("nonfinite glyph")
					}
				}
			}
			for _, operation := range page.Operations {
				raw, e := result.Document.Bytes(operation.Span)
				if e != nil || string(raw) != operation.Operator {
					t.Fatal("invalid operation source")
				}
			}
		}
	})
}

func TestExtractReviewSkipsExtGStateWithoutTextOrStyles(t *testing.T) {
	data := semanticFixture(
		`<< /Type /Catalog /Pages 2 0 R >>`,
		`<< /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox [0 0 10 10] >>`,
		`<< /Type /Page /Parent 2 0 R /Contents 4 0 R /Resources << /ExtGState << /GS 99 0 R >> >> >>`,
		semanticStream("", `/GS gs 0 0 1 1 re f`),
	)
	got, err := ParseReader(bytes.NewReader(data), int64(len(data)), ParseOptions{Content: ContentGraphics})
	if err != nil {
		t.Fatalf("graphics without styles resolved irrelevant ExtGState: %v", err)
	}
	if len(got.Pages[0].Graphics) != 1 || !got.Pages[0].Complete {
		t.Fatalf("requested graphic is incomplete: %+v", got)
	}
}

func TestExtractReviewSkipsUnrequestedShading(t *testing.T) {
	data := fontFixture(`<< /Subtype /Type1 /BaseFont /Helvetica >>`, `/Sh sh BT /F 12 Tf (A) Tj ET`)
	got, err := ParseReader(bytes.NewReader(data), int64(len(data)), ParseOptions{})
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
	got, err := ParseReader(bytes.NewReader(data), int64(len(data)), ParseOptions{
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
	got, err := ParseReader(bytes.NewReader(data), int64(len(data)), ParseOptions{Content: ContentGraphics})
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
	got, err := ParseReader(bytes.NewReader(data), int64(len(data)), ParseOptions{Provenance: true})
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

func TestExtractUnicodeSkipsTransformsAndTransparency(t *testing.T) {
	huge := "1" + strings.Repeat("0", 308)
	data := semanticFixture(
		`<< /Type /Catalog /Pages 2 0 R >>`,
		`<< /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox [0 0 100 100] >>`,
		`<< /Type /Page /Parent 2 0 R /Contents 4 0 R /Resources << /XObject << /Fm 5 0 R >> >> >>`,
		semanticStream("", huge+` 0 0 1 0 0 cm /Fm Do`),
		semanticStream(`/Subtype /Form /Matrix [`+huge+` 0 0 1 0 0] /BBox [0 0 1 1] /Group 99 0 R /Resources << /Font << /F 6 0 R >> >>`, `BT /F 12 Tf (A) Tj ET`),
		`<< /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding /FirstChar 65 /Widths [600] >>`,
	)
	got, err := ParseReader(bytes.NewReader(data), int64(len(data)), ParseOptions{})
	if err != nil {
		t.Fatalf("Unicode-only extraction evaluated unused effects: %v", err)
	}
	if got.Pages[0].Texts[0].Unicode != "A" || !got.Pages[0].Complete {
		t.Fatal("requested Unicode is incomplete")
	}
}

func TestExtractSplitContentsAndClipping(t *testing.T) {
	data := semanticFixture(
		`<< /Type /Catalog /Pages 2 0 R >>`,
		`<< /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox [0 0 100 100] >>`,
		`<< /Type /Page /Parent 2 0 R /Contents [4 0 R 5 0 R] /Resources << /Font << /F 6 0 R >> >> >>`,
		semanticStream("", `q 2 0 0 3 10 20 cm 0 0 10 10 re W n BT /F 12 Tf [(A) -100 `),
		semanticStream("", `(A)] TJ ET Q BT /F 12 Tf (A) Tj ET`),
		`<< /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding /FirstChar 65 /Widths [600] >>`,
	)
	full, err := Read(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	got, err := ParseReader(bytes.NewReader(data), int64(len(data)), ParseOptions{Glyphs: true, Styles: true, Provenance: true})
	if err != nil {
		t.Fatal(err)
	}
	for i, text := range got.Pages[0].Texts {
		if !reflect.DeepEqual(text.Glyphs, full.Texts[i].Glyphs) || !reflect.DeepEqual(*text.Style, basicStyle(full.Texts[i].State)) {
			t.Fatal("cross-stream state changed")
		}
		for _, span := range text.Source.Spans {
			if _, err := got.Document.Bytes(span); err != nil {
				t.Fatal(err)
			}
		}
	}
	if !got.Pages[0].Texts[0].Style.Clipped || got.Pages[0].Texts[1].Style.Clipped {
		t.Fatal("clipping not restored")
	}
	if len(got.Pages[0].Graphics) != 0 {
		t.Fatal("clipping emitted unrequested graphics")
	}
}

func TestExtractFormLimitsWithoutProvenance(t *testing.T) {
	data := semanticFixture(
		`<< /Type /Catalog /Pages 2 0 R >>`,
		`<< /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox [0 0 100 100] >>`,
		`<< /Type /Page /Parent 2 0 R /Contents 4 0 R /Resources << /XObject << /Fm 5 0 R >> >> >>`,
		semanticStream("", `/Fm Do /Fm Do /Fm Do`),
		semanticStream(`/Subtype /Form /BBox [0 0 1 1]`, `0 0 1 1 re f`),
	)
	got, err := ParseReader(bytes.NewReader(data), int64(len(data)), ParseOptions{Content: ContentGraphics})
	if err != nil || len(got.Pages[0].Graphics) != 3 {
		t.Fatalf("reused Form: %v", err)
	}
	_, err = ParseReader(bytes.NewReader(data), int64(len(data)), ParseOptions{
		ReadOptions: ReadOptions{Limits: Limits{MaxContentBytes: 30}},
	})
	if !errors.Is(err, ErrLimit) {
		t.Fatalf("skipped drawing bypassed cumulative Form bytes: %v", err)
	}
}

func TestExtractAnnotationsOnly(t *testing.T) {
	data := semanticFixture(
		`<< /Type /Catalog /Pages 2 0 R >>`,
		`<< /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox [0 0 100 100] >>`,
		`<< /Type /Page /Parent 2 0 R /Contents 99 0 R /Resources 98 0 R /Annots [4 0 R] >>`,
		`<< /Subtype /Link /Rect [10 20 30 40] >>`,
	)
	got, err := ParseReader(bytes.NewReader(data), int64(len(data)), ParseOptions{Content: ContentAnnotations})
	if err != nil {
		t.Fatalf("annotation-only extraction read page drawing: %v", err)
	}
	if len(got.Pages[0].Annotations) != 1 || got.Pages[0].Annotations[0].Subtype != "Link" {
		t.Fatal("missing annotation")
	}
}

func TestExtractImageSource(t *testing.T) {
	got, err := ParseFile("../../testdata/synthetic.pdf", ParseOptions{Content: ContentImages, Provenance: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.ImageResources) != 1 || len(got.Pages[0].Images) != 1 || len(got.Pages[1].Images) != 1 {
		t.Fatal("image resources are not shared")
	}
	resource := got.ImageResources[0]
	if resource.Object == nil || resource.Width != 2 || resource.Height != 2 {
		t.Fatal("missing image metadata")
	}
	stream, ok := resource.Object.Value.(Stream)
	if !ok {
		t.Fatal("image source is not a stream")
	}
	source, err := got.Document.DecodeStream(stream)
	if err != nil {
		t.Fatal(err)
	}
	data, err := got.Document.Bytes(Span{Source: source.ID, End: source.Size})
	if err != nil || !bytes.Equal(data, []byte{255, 0, 0, 0, 255, 0, 0, 0, 255, 255, 255, 255}) {
		t.Fatalf("image data: %x, %v", data, err)
	}
}

func TestExtractInlineImageErrorWithoutProvenance(t *testing.T) {
	data := semanticFixture(
		`<< /Type /Catalog /Pages 2 0 R >>`,
		`<< /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox [0 0 10 10] >>`,
		`<< /Type /Page /Parent 2 0 R /Contents 4 0 R >>`,
		semanticStream("", `BI /W 1 /H 1 ID x EI`),
	)
	got, err := ParseReader(bytes.NewReader(data), int64(len(data)), ParseOptions{})
	if got == nil || got.Document != nil || err == nil {
		t.Fatalf("unexpected inline result: %v", err)
	}
	if strings.Contains(err.Error(), "source is retained") {
		t.Fatal("error claims unavailable source access")
	}
}

func TestExtractSelections(t *testing.T) {
	full, err := Open("../../testdata/synthetic.pdf")
	if err != nil {
		t.Fatal(err)
	}
	for mask := ContentKind(1); mask <= ContentAll; mask++ {
		t.Run(strconv.Itoa(int(mask)), func(t *testing.T) {
			got, err := ParseFile("../../testdata/synthetic.pdf", ParseOptions{
				Content: mask, Positions: true, Styles: true,
			})
			if err != nil {
				t.Fatal(err)
			}
			if got.Document != nil || len(got.Pages) != len(full.Pages) {
				t.Fatal("unexpected retained document or page count")
			}
			var nt, ng, ni int
			for _, page := range got.Pages {
				if !page.Complete {
					t.Fatalf("incomplete page: %+v", got.Diagnostics)
				}
				for _, text := range page.Texts {
					want := full.Texts[nt]
					if text.Unicode != want.Unicode || text.Position == nil || text.Position.Matrix != want.Matrix ||
						text.Position.FontSize != want.FontSize || !reflect.DeepEqual(*text.Style, basicStyle(want.State)) {
						t.Fatalf("text %d mismatch: %+v", nt, text)
					}
					nt++
				}
				for _, graphic := range page.Graphics {
					want := full.Graphics[ng]
					if graphic.Paint != want.Paint || len(graphic.Segments) != len(want.Segments) || !reflect.DeepEqual(*graphic.Style, basicStyle(want.State)) {
						t.Fatalf("graphic %d mismatch", ng)
					}
					for i, segment := range graphic.Segments {
						if !reflect.DeepEqual(segment.Points, want.Segments[i].Points) {
							t.Fatal("path coordinates changed")
						}
					}
					ng++
				}
				for _, image := range page.Images {
					if image.Matrix == nil || *image.Matrix != full.Images[ni].Matrix {
						t.Fatal("image placement changed")
					}
					ni++
				}
			}
			for _, kind := range []struct {
				flag      ContentKind
				got, want int
			}{
				{ContentText, nt, len(full.Texts)}, {ContentGraphics, ng, len(full.Graphics)}, {ContentImages, ni, len(full.Images)},
			} {
				want := 0
				if mask&kind.flag != 0 {
					want = kind.want
				}
				if kind.got != want {
					t.Fatalf("kind %d count=%d want=%d", kind.flag, kind.got, want)
				}
			}
		})
	}
}

func TestExtractDefaultAndJSON(t *testing.T) {
	got, err := ParseFile("../../testdata/synthetic.pdf", ParseOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Content != ContentText || len(got.Pages[0].Texts) != 2 {
		t.Fatal("zero options must extract text")
	}
	text := got.Pages[0].Texts[0]
	if text.Position != nil || text.Style != nil || text.Source != nil || len(text.Glyphs) != 0 {
		t.Fatal("unexpected optional text data")
	}
	if got.Pages[1].Texts[1].Unicode != "Reusable form" {
		t.Fatal("text-only extraction skipped Form")
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"Graphics", "Images", "Annotations", "Operations", "Position", "Style", "Glyphs", "Source", "Document", "ImageResources"} {
		if bytes.Contains(encoded, []byte(`"`+field+`":`)) {
			t.Fatalf("JSON contains disabled field %s", field)
		}
	}
}

type extractionReader struct {
	*bytes.Reader
	reads int
}

func (r *extractionReader) ReadAt(p []byte, off int64) (int, error) {
	r.reads++
	return r.Reader.ReadAt(p, off)
}

func TestExtractReaderOptionsAndSnapshot(t *testing.T) {
	data := semanticFixture(`<< /Type /Catalog /Pages 2 0 R >>`, `<< /Type /Pages /Kids [] /Count 0 >>`)
	r := &extractionReader{Reader: bytes.NewReader(data)}
	for _, options := range []ParseOptions{
		{Content: ContentAll << 1}, {Content: ContentImages, Glyphs: true}, {ReadOptions: ReadOptions{MaxFileBytes: -1}},
	} {
		if result, err := ParseReader(r, int64(len(data)), options); err == nil || result != nil {
			t.Fatal("invalid options accepted")
		}
	}
	if r.reads != 0 {
		t.Fatal("read input before validating options")
	}
	if _, err := ParseReader(r, int64(len(data)), ParseOptions{Content: ContentAll}); err != nil {
		t.Fatal(err)
	}
	if r.reads != 1 {
		t.Fatalf("input snapshots=%d want=1", r.reads)
	}
}

func TestExtractGlyphsAndProvenance(t *testing.T) {
	got, err := ParseFile("../../testdata/synthetic.pdf", ParseOptions{Glyphs: true, Provenance: true})
	if err != nil {
		t.Fatal(err)
	}
	if got.Document == nil {
		t.Fatal("provenance needs source access")
	}
	text := got.Pages[1].Texts[1]
	if text.Position == nil || len(text.Glyphs) == 0 || text.Source == nil || len(text.Source.FormPath) != 1 {
		t.Fatal("missing detailed text fields")
	}
	for _, span := range text.Source.Spans {
		data, err := got.Document.Bytes(span)
		if err != nil || len(data) == 0 {
			t.Fatalf("source unavailable: %v", err)
		}
	}
	for _, index := range text.Source.Operations {
		if got.Pages[1].Operations[index].Operator != "Tj" {
			t.Fatal("incorrect operation index")
		}
	}
}

func TestExtractSkipsUnrequestedResources(t *testing.T) {
	data := semanticFixture(
		`<< /Type /Catalog /Pages 2 0 R >>`,
		`<< /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox [0 0 100 100] >>`,
		`<< /Type /Page /Parent 2 0 R /Contents 4 0 R /Resources << /Font << /F 99 0 R >> /XObject << /Im 5 0 R >> >> /Annots 98 0 R >>`,
		semanticStream("", `BT /F 12 Tf (ignored) Tj ET 0 0 10 10 re f /Im Do`),
		semanticStream(`/Subtype /Image /Width (bad) /Height 2`, "ignored"),
	)
	got, err := ParseReader(bytes.NewReader(data), int64(len(data)), ParseOptions{Content: ContentGraphics})
	if err != nil || len(got.Pages[0].Graphics) != 1 {
		t.Fatalf("unused resources loaded: %v", err)
	}
	if !got.Pages[0].Complete {
		t.Fatal("intentionally skipped data is not incomplete")
	}
}

func TestExtractErrorsAndLimits(t *testing.T) {
	if got, err := ParseFile("does-not-exist.pdf", ParseOptions{}); err == nil || got != nil {
		t.Fatal("missing file accepted")
	}
	data := semanticFixture(
		`<< /Type /Catalog /Pages 2 0 R >>`,
		`<< /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox [0 0 10 10] >>`,
		`<< /Type /Page /Parent 2 0 R /Contents 4 0 R >>`,
		semanticStream("", `0 0 1 1 re f Q`),
	)
	got, err := ParseReader(bytes.NewReader(data), int64(len(data)), ParseOptions{Content: ContentGraphics})
	if err == nil || got == nil || len(got.Pages[0].Graphics) != 1 {
		t.Fatal("missing partial result")
	}
	if got.Pages[0].Complete {
		t.Fatal("failed page marked complete")
	}
	_, err = ParseReader(bytes.NewReader(data), int64(len(data)), ParseOptions{ReadOptions: ReadOptions{Limits: Limits{MaxContentBytes: 1}}})
	if !errors.Is(err, ErrLimit) {
		t.Fatalf("limit error=%v", err)
	}
	_, err = ParseReader(bytes.NewReader(nil), 1, ParseOptions{})
	if !errors.Is(err, io.EOF) {
		t.Fatalf("short read=%v", err)
	}
}
