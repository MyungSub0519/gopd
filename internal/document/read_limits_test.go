package document

import (
	"bytes"
	"errors"
	"testing"

	"github.com/MyungSub0519/gopd/internal/pdfmodel"
)

func TestFileBudgetErrorIsClassifiable(t *testing.T) {
	r := &controlledReaderAt{}
	_, err := Parse(r, 2, ReadOptions{MaxFileBytes: 1})
	if !errors.Is(err, pdfmodel.ErrLimit) || r.calls != 0 {
		t.Fatalf("expected classified limit before reading, got %v, calls=%d", err, r.calls)
	}
}

func TestParseRejectsNegativeResourceLimitsBeforeReading(t *testing.T) {
	for _, limits := range []pdfmodel.Limits{
		{MaxValues: -1}, {MaxContentBytes: -1}, {MaxSemanticObjects: -1}, {MaxRegionWork: -1},
	} {
		r := &controlledReaderAt{}
		if _, err := Parse(r, 1, ReadOptions{Limits: limits}); err == nil || r.calls != 0 {
			t.Fatalf("limits=%+v err=%v reader calls=%d", limits, err, r.calls)
		}
	}
}

func TestDocumentCumulativeDirectValueBudget(t *testing.T) {
	data := makeTestPDF([]string{"[1 2]", "[3 4]", "[5 6]"}, "")
	d, err := Parse(bytes.NewReader(data), int64(len(data)), ReadOptions{Limits: pdfmodel.Limits{MaxValues: 9}})
	if err != nil {
		t.Fatal(err)
	}
	// The trailer is three values, then each array is another three.
	for _, n := range []uint32{1, 2} {
		if _, err := d.Load(pdfmodel.ObjectID{Number: n}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := d.Load(pdfmodel.ObjectID{Number: 1}); err != nil {
		t.Fatal("cache hit consumed budget:", err)
	}
	if _, err := d.Load(pdfmodel.ObjectID{Number: 3}); err == nil {
		t.Fatal("separate objects bypassed cumulative direct value budget")
	}
}
