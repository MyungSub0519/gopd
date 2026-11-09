package document

import (
	"bytes"
	"compress/zlib"
	"errors"
	"fmt"
	"testing"

	"github.com/MyungSub0519/gopd/internal/pdfmodel"
	"github.com/MyungSub0519/gopd/internal/pdftest"
)

// ISO 32000-1:2008, 7.3.10 and 7.4.4 Table 8 permit indirect parameters
// in ordinary streams. Use prediction to exercise all four integer parameters.
func TestIndirectPredictorParameters(t *testing.T) {
	var encoded bytes.Buffer
	zw := zlib.NewWriter(&encoded)
	if _, err := zw.Write([]byte{10, 10, 10}); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, params string }{
		{"direct", "/Predictor 2 /Colors 1 /Columns 3 /BitsPerComponent 8"},
		{"indirect", "/Predictor 3 0 R /Colors 4 0 R /Columns 5 0 R /BitsPerComponent 6 0 R"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := pdftest.File([]string{"<< /Type /Catalog >>", pdftest.Stream(fmt.Sprintf("/Filter /FlateDecode /DecodeParms << %s >>", tc.params), encoded.String()), "2", "1", "3", "8"}, "")
			d := parseTestPDF(t, data)
			obj, err := d.Resolve(pdfmodel.Reference{ID: pdfmodel.ObjectID{Number: 2}})
			if err != nil {
				t.Fatal(err)
			}
			source, err := d.DecodeStream(obj.Value.(pdfmodel.Stream))
			if err != nil {
				t.Fatal(err)
			}
			got, err := d.Bytes(pdfmodel.Span{Source: source.ID, End: source.Size})
			if err != nil || !bytes.Equal(got, []byte{10, 20, 30}) {
				t.Fatalf("decoded %v, %v", got, err)
			}
			if tc.name == "indirect" {
				params := source.Origin.Steps[0].Params.Value.(pdfmodel.Dictionary)
				predictor, err := params.Get("Predictor")
				if err != nil {
					t.Fatal(err)
				}
				if _, ok := predictor.Value.(pdfmodel.Reference); !ok {
					t.Fatal("decoding replaced the original predictor reference in provenance")
				}
			}
		})
	}
}

func TestPredictorReferencesPreserveErrorsAndLimits(t *testing.T) {
	for _, tc := range []struct {
		name, kind, target string
		depth              int
		limit              bool
	}{
		{"cycle", "", "3 0 R", 0, false},
		{"depth", "", "4 0 R", 1, true},
		{"xref_direct_only", "/Type /XRef", "1", 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var compressed bytes.Buffer
			w := zlib.NewWriter(&compressed)
			if _, err := w.Write([]byte("abc")); err != nil {
				t.Fatal(err)
			}
			if err := w.Close(); err != nil {
				t.Fatal(err)
			}
			data := pdftest.File([]string{"<< /Type /Catalog >>", pdftest.Stream(tc.kind+" /Filter /FlateDecode /DecodeParms << /Predictor 3 0 R >>", compressed.String()), tc.target, "1"}, "")
			d := parseTestPDF(t, data)
			obj, err := d.Resolve(pdfmodel.Reference{ID: pdfmodel.ObjectID{Number: 2}})
			if err != nil {
				t.Fatal(err)
			}
			if tc.depth != 0 {
				d.Options.Limits.MaxDepth = tc.depth
			}
			_, err = d.DecodeStream(obj.Value.(pdfmodel.Stream))
			if err == nil || errors.Is(err, pdfmodel.ErrLimit) != tc.limit {
				t.Fatalf("unexpected error: %v (limit=%v)", err, tc.limit)
			}
		})
	}
}

func TestResolveUndefinedReferences(t *testing.T) {
	d := parseTestPDF(t, pdftest.File([]string{"<< /Type /Catalog >>", "42"}, ""))
	for _, id := range []pdfmodel.ObjectID{{Number: 99}, {Number: 0}, {Number: 2, Generation: 1}} {
		obj, err := d.Resolve(pdfmodel.Reference{ID: id})
		if err != nil {
			t.Errorf("%v: %v", id, err)
			continue
		}
		if _, ok := obj.Value.(pdfmodel.Null); !ok {
			t.Errorf("%v: expected null, got %T", id, obj.Value)
		}
	}
	// Physical loading remains strict; only semantic resolution maps absence to null.
	if _, err := d.Load(pdfmodel.ObjectID{Number: 99}); err == nil {
		t.Fatal("Load should report missing physical object")
	}
	d.entries[2] = pdfmodel.XRefRecord{Number: 2, Entry: pdfmodel.InUseEntry{Offset: 1}}
	if _, err := d.Resolve(pdfmodel.Reference{ID: pdfmodel.ObjectID{Number: 2}}); err == nil {
		t.Fatal("corrupt in-use object must not become null")
	}
}
