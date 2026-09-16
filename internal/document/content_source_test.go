package document

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/MyungSub0519/gopd/internal/pdfmodel"
)

func TestJoinedContentSourceReadAtAndProvenance(t *testing.T) {
	d := &Document{
		Sources: map[pdfmodel.SourceID]pdfmodel.Source{
			1: {ID: 1, Reader: bytes.NewReader([]byte("ab")), Size: 2},
			2: {ID: 2, Reader: bytes.NewReader([]byte("XYZ")), Size: 3},
			3: {ID: 3, Reader: bytes.NewReader(nil)},
		},
		Options: ReadOptions{Limits: pdfmodel.Limits{MaxContentBytes: 6, MaxSemanticObjects: 4}},
	}
	inputs := []pdfmodel.Span{{Source: 1, End: 2}, {Source: 3}, {Source: 2, Start: 1, End: 3}, {Source: 1, End: 2}}
	source, err := JoinContentSources(d, inputs)
	if err != nil {
		t.Fatal(err)
	}
	inputs[0].End = 0
	if source.Origin.Inputs[0].End != 2 || source.Size != 6 {
		t.Fatalf("incorrect or aliased provenance: %+v", source)
	}
	want := bytes.NewReader([]byte("abYZab"))
	for offset := int64(0); offset <= 7; offset++ {
		for length := 1; length <= 8; length++ {
			actual, expected := make([]byte, length), make([]byte, length)
			n, err := source.Reader.ReadAt(actual, offset)
			wn, we := want.ReadAt(expected, offset)
			if n != wn || !bytes.Equal(actual, expected) || errors.Is(err, io.EOF) != errors.Is(we, io.EOF) {
				t.Fatalf("offset=%d length=%d: (%q, %d, %v), want (%q, %d, %v)", offset, length, actual, n, err, expected, wn, we)
			}
		}
	}
	if _, err := source.Reader.ReadAt(make([]byte, 1), -1); err == nil {
		t.Fatal("negative offset accepted")
	}
	if n, err := source.Reader.ReadAt(nil, 0); n != 0 || err != nil {
		t.Fatalf("empty read: %d, %v", n, err)
	}
	part, err := d.Bytes(pdfmodel.Span{Source: source.ID, Start: 1, End: 5})
	if err != nil || string(part) != "bYZa" {
		t.Fatalf("cross-source Bytes = %q, %v", part, err)
	}
}

func TestJoinedContentSourceRejectsInvalidOrOverBudgetInputs(t *testing.T) {
	for _, inputs := range [][]pdfmodel.Span{
		nil,
		{{Source: 2}},
		{{Source: 1, Start: -1, End: 1}},
		{{Source: 1, Start: 2, End: 1}},
		{{Source: 1, End: 4}},
		{{Source: 1, End: 3}, {Source: 1, End: 1}},
		{{Source: 1}, {Source: 1}, {Source: 1}},
	} {
		d := &Document{
			Sources: map[pdfmodel.SourceID]pdfmodel.Source{1: {ID: 1, Reader: bytes.NewReader([]byte("abc")), Size: 3}},
			Options: ReadOptions{Limits: pdfmodel.Limits{MaxContentBytes: 3, MaxSemanticObjects: 2}},
		}
		if _, err := JoinContentSources(d, inputs); err == nil {
			t.Fatalf("invalid inputs accepted: %+v", inputs)
		}
		if len(d.Sources) != 1 {
			t.Fatal("failure registered a partial source")
		}
	}
}
