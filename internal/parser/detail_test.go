package parser

import (
	"bytes"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/MyungSub0519/gopd/internal/document"
	"github.com/MyungSub0519/gopd/internal/pdfmodel"
)

func TestDetailFullMatchesDetailedAnalysis(t *testing.T) {
	legacy, err := Open("../../testdata/synthetic.pdf")
	t.Cleanup(func() {
		if err := legacy.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := ParseFile("../../testdata/synthetic.pdf", ParseOptions{Content: ContentAll, Detail: DetailFull})
	t.Cleanup(func() {
		if err := result.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	// Each entry point owns a distinct file and input buffer. Compare the
	// semantic result and source coordinates without comparing reader state.
	got, want := *result.Details(), *legacy
	got.Document, want.Document = nil, nil
	if !reflect.DeepEqual(got, want) {
		t.Fatal("Full changed the existing detailed analysis")
	}
}

func TestDetailFullSelectionsAndIndexes(t *testing.T) {
	for mask := ContentKind(1); mask <= ContentAll; mask++ {
		t.Run(strconv.Itoa(int(mask)), func(t *testing.T) {
			result, err := ParseFile("../../testdata/synthetic.pdf", ParseOptions{Content: mask, Detail: DetailFull})
			t.Cleanup(func() {
				if err := result.Close(); err != nil {
					t.Errorf("Close: %v", err)
				}
			})
			if err != nil {
				t.Fatal(err)
			}
			detail := result.Details()
			if detail == nil || detail.Document != result.Document || len(detail.Pages) != len(result.Pages) {
				t.Fatal("missing shared detailed view")
			}
			for _, tc := range []struct {
				kind      ContentKind
				got, want int
			}{
				{ContentText, len(detail.Texts), 4},
				{ContentGraphics, len(detail.Graphics), 3},
				{ContentImages, len(detail.Images), 2},
				{ContentAnnotations, len(detail.Annotations), 0},
			} {
				want := 0
				if mask&tc.kind != 0 {
					want = tc.want
				}
				if tc.got != want {
					t.Fatalf("content %d: count=%d want=%d", tc.kind, tc.got, want)
				}
			}
			for i, page := range detail.Pages {
				if !page.Complete || !reflect.DeepEqual(page.Operations, result.Pages[i].Operations) {
					t.Fatal("page views disagree")
				}
				for _, item := range page.Items {
					var source ElementSource
					switch item.Kind {
					case ElementText:
						text := detail.Texts[item.Index]
						source = text.Source
						if len(text.RawCodes) == 0 || len(text.Glyphs) == 0 || text.Font < 0 || text.Font >= len(detail.Fonts) {
							t.Fatal("lost full text data or font index")
						}
					case ElementGraphic:
						source = detail.Graphics[item.Index].Source
					case ElementImage:
						image := detail.Images[item.Index]
						source = image.Source
						if image.Resource < 0 || image.Resource >= len(detail.ImageResources) || image.Resource >= len(result.ImageResources) {
							t.Fatal("image resource index out of range")
						}
					default:
						t.Fatal("unknown item kind")
					}
					if source.Page != i {
						t.Fatal("item points at wrong page")
					}
					for _, index := range source.Operations {
						if index < 0 || index >= len(page.Operations) {
							t.Fatal("operation index out of range")
						}
					}
				}
				for _, index := range page.Annotations {
					if detail.Annotations[index].Page != i {
						t.Fatal("annotation index points at wrong page")
					}
				}
			}
		})
	}
}

func TestDetailAnnotationsSkipPageDrawing(t *testing.T) {
	// Required annotation data is valid; drawing-only references cycle. ISO
	// 32000-1:2008, 12.5.2 describes annotation dictionaries independently of
	// page content streams. No level executes annotation appearances.
	data := semanticFixture(
		`<< /Type /Catalog /Pages 2 0 R >>`,
		`<< /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox [0 0 100 100] >>`,
		`<< /Type /Page /Parent 2 0 R /Contents 5 0 R /Resources 5 0 R /Annots [4 0 R] >>`,
		`<< /Subtype /Link /Rect [10 20 30 40] >>`, "5 0 R",
	)
	for _, level := range []DetailLevel{DetailBasic, DetailLayout, DetailFull} {
		r := &extractionReader{Reader: bytes.NewReader(data)}
		result, err := ParseReader(r, int64(len(data)), ParseOptions{Content: ContentAnnotations, Detail: level})
		if err != nil || result == nil || len(result.Pages) != 1 {
			t.Fatalf("level %d read drawing-only references: %v", level, err)
		}
		page := result.Pages[0]
		if r.reads != 1 || !page.Complete || len(page.Annotations) != 1 || len(page.Operations) != 0 {
			t.Fatalf("level %d: unexpected annotation output: %+v", level, page)
		}
		if (page.Annotations[0].Source != nil) != (level == DetailFull) {
			t.Fatal("annotation provenance disagrees with level")
		}
		if level == DetailFull && (len(result.Details().Annotations) != 1 || len(result.Details().Pages[0].Operations) != 0) {
			t.Fatal("Full annotation view unexpectedly scanned drawing")
		}
		if level == DetailFull {
			detail := result.Details()
			indexes := detail.Pages[0].Annotations
			if len(indexes) != 1 || indexes[0] != 0 || detail.Annotations[0].Page != 0 || detail.Annotations[0].Object.Span != *page.Annotations[0].Source {
				t.Fatal("annotation index or source differs between result views")
			}
		}
	}
}

func TestDetailFullUsesExistingStyleBudget(t *testing.T) {
	// One gs operand + 30 expanded dash entries + four re operands + 32
	// retained style values = 67, as in TestExtGStateValueBudgetBoundary.
	data := graphicsStateFixture("<< /D [["+strings.Repeat("1 ", 30)+"] 0] >>", "/G gs 0 0 1 1 re f")
	for _, level := range []DetailLevel{DetailLayout, DetailFull} {
		for _, limit := range []int{66, 67} {
			result, err := ParseReader(bytes.NewReader(data), int64(len(data)), ParseOptions{
				Content: ContentGraphics, Detail: level,
				ReadOptions: document.ReadOptions{Limits: pdfmodel.Limits{MaxValues: limit}},
			})
			if limit == 66 {
				if !errors.Is(err, pdfmodel.ErrLimit) || result == nil || result.Pages[0].Complete {
					t.Fatalf("limit not enforced: level=%d err=%v", level, err)
				}
				continue
			}
			if err != nil || len(result.Pages[0].Graphics) != 1 {
				t.Fatalf("exact budget rejected: level=%d err=%v", level, err)
			}
			if level == DetailFull && len(result.Details().Graphics) != 1 {
				t.Fatal("full view missing at exact budget")
			}
		}
	}
}
