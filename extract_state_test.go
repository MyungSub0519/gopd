package gopd

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"
)

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
	got, err := ExtractReader(bytes.NewReader(data), int64(len(data)), ExtractOptions{})
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
	got, err := ExtractReader(bytes.NewReader(data), int64(len(data)), ExtractOptions{Glyphs: true, Styles: true, Provenance: true})
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
	got, err := ExtractReader(bytes.NewReader(data), int64(len(data)), ExtractOptions{Content: ContentGraphics})
	if err != nil || len(got.Pages[0].Graphics) != 3 {
		t.Fatalf("reused Form: %v", err)
	}
	_, err = ExtractReader(bytes.NewReader(data), int64(len(data)), ExtractOptions{
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
	got, err := ExtractReader(bytes.NewReader(data), int64(len(data)), ExtractOptions{Content: ContentAnnotations})
	if err != nil {
		t.Fatalf("annotation-only extraction read page drawing: %v", err)
	}
	if len(got.Pages[0].Annotations) != 1 || got.Pages[0].Annotations[0].Subtype != "Link" {
		t.Fatal("missing annotation")
	}
}

func TestExtractImageSource(t *testing.T) {
	got, err := Extract("testdata/synthetic.pdf", ExtractOptions{Content: ContentImages, Provenance: true})
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
	got, err := ExtractReader(bytes.NewReader(data), int64(len(data)), ExtractOptions{})
	if got == nil || got.Document != nil || err == nil {
		t.Fatalf("unexpected inline result: %v", err)
	}
	if strings.Contains(err.Error(), "source is retained") {
		t.Fatal("error claims unavailable source access")
	}
}
