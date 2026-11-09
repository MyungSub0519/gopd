package gopd_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/MyungSub0519/gopd"
	"github.com/MyungSub0519/gopd/internal/pdftest"
)

func TestPublicDetailLevels(t *testing.T) {
	path, data := publicFixture(t)
	entries := map[string]func(gopd.ParseOptions) (*gopd.Result, error){
		"file": func(options gopd.ParseOptions) (*gopd.Result, error) {
			return gopd.ParseFile(path, options)
		},
		"reader": func(options gopd.ParseOptions) (*gopd.Result, error) {
			return gopd.ParseReader(bytes.NewReader(data), int64(len(data)), options)
		},
	}
	for name, parse := range entries {
		for _, tc := range []struct {
			name  string
			level gopd.DetailLevel
		}{
			{"basic", gopd.DetailBasic},
			{"layout", gopd.DetailLayout},
			{"full", gopd.DetailFull},
		} {
			t.Run(name+"/"+tc.name, func(t *testing.T) {
				result, err := parse(gopd.ParseOptions{Content: gopd.ContentAll, Detail: tc.level})
				closePublicInputOnCleanup(t, result)
				if err != nil {
					t.Fatal(err)
				}
				if result.Detail != tc.level || len(result.Pages) != 1 {
					t.Fatalf("unexpected result: %+v", result)
				}
				page := result.Pages[0]
				if len(page.Texts) != 1 || len(page.Graphics) != 1 || !page.Complete {
					t.Fatalf("lost selected content: %+v", page)
				}
				text := page.Texts[0]
				if text.Unicode != "A" || text.Font == nil || text.Font.BaseFont != "Helvetica" {
					t.Fatalf("lost basic text: %+v", text)
				}
				layout := tc.level != gopd.DetailBasic
				full := tc.level == gopd.DetailFull
				if (text.Position != nil) != layout || (text.Style != nil) != layout || (page.Graphics[0].Style != nil) != layout {
					t.Fatal("positions/styles disagree with level")
				}
				if layout && (text.Position.Matrix[4] != 20 || text.Position.Matrix[5] != 30 || text.Position.FontSize != 12) {
					t.Fatalf("unexpected placement: %+v", text.Position)
				}
				if (len(text.Glyphs) != 0) != full || (text.Source != nil) != full || (result.Document != nil) != full || (result.Details() != nil) != full || (len(page.Operations) != 0) != full {
					t.Fatal("full information disagrees with level")
				}
				if full {
					checkPublicDetail(t, result.Details())
					if result.Details().Document != result.Document || string(result.Details().Texts[0].RawCodes) != "A" {
						t.Fatal("details lost the shared document or raw codes")
					}
					for _, index := range text.Source.Operations {
						op := page.Operations[index]
						raw, err := result.Document.Bytes(op.Span)
						if err != nil || string(raw) != "Tj" {
							t.Fatalf("invalid operation source: %q, %v", raw, err)
						}
					}
				}
				encoded, err := json.Marshal(result)
				if err != nil {
					t.Fatal(err)
				}
				if bytes.Contains(encoded, []byte(`"Document"`)) || bytes.Contains(encoded, []byte(`"Details"`)) {
					t.Fatal("JSON retained the document or duplicate detailed view")
				}
				if bytes.Contains(encoded, []byte(`"Operations"`)) != full || bytes.Contains(encoded, []byte(`"Position"`)) != layout {
					t.Fatalf("JSON disagrees with detail level: %s", encoded)
				}
			})
		}
	}
	var absent *gopd.Result
	if absent.Details() != nil {
		t.Fatal("nil result must have nil details")
	}
}

func TestPublicDetailValidationBeforeInput(t *testing.T) {
	options := gopd.ParseOptions{Detail: gopd.DetailLevel(255)}
	if result, err := gopd.ParseReader(nil, 0, options); result != nil || err == nil {
		t.Fatal("unknown detail accepted")
	}
	if result, err := gopd.ParseFile("missing-detail-validation.pdf", options); result != nil || err == nil || errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unknown detail was not a configuration error: %v", err)
	}
}

func TestPublicFullRespectsContentSelection(t *testing.T) {
	path, _ := publicFixture(t)
	for _, kind := range []gopd.ContentKind{gopd.ContentText, gopd.ContentGraphics, gopd.ContentImages} {
		result, err := gopd.ParseFile(path, gopd.ParseOptions{Content: kind, Detail: gopd.DetailFull})
		closePublicInputOnCleanup(t, result)
		if err != nil {
			t.Fatal(err)
		}
		detail := result.Details()
		if detail == nil || (len(detail.Texts) != 0) != (kind == gopd.ContentText) || (len(detail.Graphics) != 0) != (kind == gopd.ContentGraphics) {
			t.Fatalf("full details ignored content selection %d", kind)
		}
		if kind != gopd.ContentText && len(detail.Fonts) != 0 {
			t.Fatal("loaded unrequested text resources")
		}
	}
}

func TestPublicDetailPartialErrorsAndLimits(t *testing.T) {
	for _, level := range []gopd.DetailLevel{gopd.DetailBasic, gopd.DetailLayout, gopd.DetailFull} {
		for _, tail := range []string{"UnknownOperator", "(unterminated"} {
			data := pdftest.File([]string{
				`<< /Type /Catalog /Pages 2 0 R >>`,
				`<< /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox [0 0 100 100] >>`,
				`<< /Type /Page /Parent 2 0 R /Contents 4 0 R >>`,
				pdftest.Stream("", "0 0 10 10 re f "+tail),
			}, "")
			result, err := gopd.ParseReader(bytes.NewReader(data), int64(len(data)), gopd.ParseOptions{Content: gopd.ContentGraphics, Detail: level})
			closePublicInputOnCleanup(t, result)
			if (err != nil) != (tail == "(unterminated") || result == nil || len(result.Pages) != 1 || len(result.Pages[0].Graphics) != 1 || result.Pages[0].Complete {
				t.Fatalf("level %d lost partial/incomplete state: %+v, %v", level, result, err)
			}
			if tail == "UnknownOperator" && len(result.Diagnostics) == 0 {
				t.Fatal("unsupported content silently succeeded")
			}
			if level == gopd.DetailFull && (result.Details() == nil || result.Details().Pages[0].Complete || len(result.Details().Graphics) != 1) {
				t.Fatal("detailed partial result disagrees with page output")
			}
			if level == gopd.DetailFull {
				page := result.Pages[0]
				detail := result.Details()
				items := detail.Pages[0].Items
				if len(items) != 1 || items[0].Kind != gopd.ElementGraphic || items[0].Index != 0 || len(detail.Pages[0].Operations) != len(page.Operations) {
					t.Fatal("partial detailed indices disagree with retained output")
				}
				for _, index := range page.Graphics[0].Source.Operations {
					if index < 0 || index >= len(page.Operations) {
						t.Fatal("partial source points outside retained operations")
					}
					op := page.Operations[index]
					raw, e := result.Document.Bytes(op.Span)
					if e != nil || string(raw) != op.Operator || detail.Pages[0].Operations[index].Span != op.Span {
						t.Fatal("partial views lost matching source provenance")
					}
				}
				if tail == "UnknownOperator" && len(detail.Diagnostics) != len(result.Diagnostics) {
					t.Fatal("partial views lost matching diagnostics")
				}
			}
			result, err = gopd.ParseReader(bytes.NewReader(data), int64(len(data)), gopd.ParseOptions{
				Content: gopd.ContentGraphics, Detail: level,
				ReadOptions: gopd.ReadOptions{Limits: gopd.Limits{MaxContentBytes: 1}},
			})
			closePublicInputOnCleanup(t, result)
			if !errors.Is(err, gopd.ErrLimit) || result == nil || len(result.Pages) != 1 || result.Pages[0].Complete {
				t.Fatalf("level %d lost limit error/partial result: %v", level, err)
			}
		}
	}
}
