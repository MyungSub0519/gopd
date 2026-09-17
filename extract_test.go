package gopd

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strconv"
	"testing"
)

func TestExtractSelections(t *testing.T) {
	full, err := Open("testdata/synthetic.pdf")
	if err != nil {
		t.Fatal(err)
	}
	for mask := ContentKind(1); mask <= ContentAll; mask++ {
		t.Run(strconv.Itoa(int(mask)), func(t *testing.T) {
			got, err := Extract("testdata/synthetic.pdf", ExtractOptions{
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
	got, err := Extract("testdata/synthetic.pdf", ExtractOptions{})
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
	for _, options := range []ExtractOptions{
		{Content: ContentAll << 1}, {Content: ContentImages, Glyphs: true}, {ReadOptions: ReadOptions{MaxFileBytes: -1}},
	} {
		if result, err := ExtractReader(r, int64(len(data)), options); err == nil || result != nil {
			t.Fatal("invalid options accepted")
		}
	}
	if r.reads != 0 {
		t.Fatal("read input before validating options")
	}
	if _, err := ExtractReader(r, int64(len(data)), ExtractOptions{Content: ContentAll}); err != nil {
		t.Fatal(err)
	}
	if r.reads != 1 {
		t.Fatalf("input snapshots=%d want=1", r.reads)
	}
}

func TestExtractGlyphsAndProvenance(t *testing.T) {
	got, err := Extract("testdata/synthetic.pdf", ExtractOptions{Glyphs: true, Provenance: true})
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
	got, err := ExtractReader(bytes.NewReader(data), int64(len(data)), ExtractOptions{Content: ContentGraphics})
	if err != nil || len(got.Pages[0].Graphics) != 1 {
		t.Fatalf("unused resources loaded: %v", err)
	}
	if !got.Pages[0].Complete {
		t.Fatal("intentionally skipped data is not incomplete")
	}
}

func TestExtractErrorsAndLimits(t *testing.T) {
	if got, err := Extract("does-not-exist.pdf", ExtractOptions{}); err == nil || got != nil {
		t.Fatal("missing file accepted")
	}
	data := semanticFixture(
		`<< /Type /Catalog /Pages 2 0 R >>`,
		`<< /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox [0 0 10 10] >>`,
		`<< /Type /Page /Parent 2 0 R /Contents 4 0 R >>`,
		semanticStream("", `0 0 1 1 re f Q`),
	)
	got, err := ExtractReader(bytes.NewReader(data), int64(len(data)), ExtractOptions{Content: ContentGraphics})
	if err == nil || got == nil || len(got.Pages[0].Graphics) != 1 {
		t.Fatal("missing partial result")
	}
	if got.Pages[0].Complete {
		t.Fatal("failed page marked complete")
	}
	_, err = ExtractReader(bytes.NewReader(data), int64(len(data)), ExtractOptions{ReadOptions: ReadOptions{Limits: Limits{MaxContentBytes: 1}}})
	if !errors.Is(err, ErrLimit) {
		t.Fatalf("limit error=%v", err)
	}
	_, err = ExtractReader(bytes.NewReader(nil), 1, ExtractOptions{})
	if !errors.Is(err, io.EOF) {
		t.Fatalf("short read=%v", err)
	}
}
