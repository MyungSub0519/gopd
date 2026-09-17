package document

import (
	"bytes"
	"compress/zlib"
	"errors"
	"fmt"
	"testing"

	"github.com/MyungSub0519/gopd/internal/pdfmodel"
)

func TestResolveObjectHopBoundary(t *testing.T) {
	// MaxDepth counts reference hops, not the terminal direct value.
	for _, tc := range []struct {
		name      string
		start     pdfmodel.Value
		limit     int
		wantError bool
		wantLimit bool
	}{
		{"direct", pdfmodel.Integer("42"), 0, false, false},
		{"exact", pdfmodel.Reference{ID: pdfmodel.ObjectID{Number: 1}}, 2, false, false},
		{"over", pdfmodel.Reference{ID: pdfmodel.ObjectID{Number: 1}}, 1, true, true},
		{"cycle", pdfmodel.Reference{ID: pdfmodel.ObjectID{Number: 3}}, 2, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := parseTestPDF(t, makeTestPDF([]string{"2 0 R", "42", "3 0 R"}, ""))
			doc.Options.Limits.MaxDepth = tc.limit
			got, err := doc.ResolveObject(pdfmodel.Object{Value: tc.start})
			if tc.wantError {
				if err == nil || errors.Is(err, pdfmodel.ErrLimit) != tc.wantLimit {
					t.Fatalf("resolution error = %v, want limit=%v", err, tc.wantLimit)
				}
				return
			}
			if err != nil || got.Value != pdfmodel.Integer("42") {
				t.Fatalf("resolution = %+v, %v; want 42", got, err)
			}
		})
	}
}

func TestXRefOptionalDirectNull(t *testing.T) {
	// Adobe PDF Reference 1.6, section 3.2.6: a null dictionary value
	// is equivalent to omission. Bootstrap entries remain direct values.
	for _, form := range []string{"table", "stream"} {
		keys := []string{"Prev", "XRefStm"}
		if form == "stream" {
			keys = append(keys, "Index")
		}
		for _, key := range keys {
			for _, tc := range []struct {
				name, value string
				wantError   bool
			}{
				{"null", "null", false},
				{"duplicate", "null /" + key + " null", true},
				{"reference", "3 0 R", true},
			} {
				t.Run(form+"/"+key+"/"+tc.name, func(t *testing.T) {
					entry := "/" + key + " " + tc.value
					data := makeTestPDF([]string{"<< /Type /Catalog >>", "null", "null"}, entry)
					if form == "stream" {
						data = xrefStreamFile(map[int]string{1: "<< /Type /Catalog >>", 2: "null", 3: "null"}, nil, 4)
						data = bytes.Replace(data, []byte("/W ["), []byte(entry+" /W ["), 1)
					}
					doc, err := Parse(bytes.NewReader(data), int64(len(data)))
					if tc.wantError {
						if err == nil {
							t.Fatal("invalid bootstrap entry accepted")
						}
						return
					}
					if err != nil {
						t.Fatal(err)
					}
					if _, err := doc.Catalog(); err != nil {
						t.Fatal(err)
					}
					section := doc.Structure.XRefs[0]
					if section.Prev != nil || section.XRefStm != nil {
						t.Fatal("null created an xref link")
					}
					raw, err := section.Trailer.Value.(pdfmodel.Dictionary).Get(pdfmodel.Name(key))
					if err != nil {
						t.Fatal(err)
					}
					if _, ok := raw.Value.(pdfmodel.Null); !ok {
						t.Fatal("raw null syntax lost")
					}
				})
			}
		}
	}
}

func TestPredictorOptionalDirectNull(t *testing.T) {
	// Adobe PDF Reference 1.6, sections 3.2.6 and 3.3.3 (predictor defaults).
	for _, key := range []pdfmodel.Name{"Predictor", "Colors", "Columns", "BitsPerComponent"} {
		t.Run(string(key), func(t *testing.T) {
			entries := []pdfmodel.DictionaryEntry{{Key: key, Value: pdfmodel.Object{Value: pdfmodel.Null{}}}}
			data, want := []byte{1, 10}, []byte{10}
			if key == "Predictor" {
				data, want = []byte{42}, []byte{42}
			} else {
				entries = append(entries, pdfmodel.DictionaryEntry{Key: "Predictor", Value: pdfmodel.Object{Value: pdfmodel.Integer("15")}})
			}
			params := pdfmodel.Object{Value: pdfmodel.Dictionary{Entries: entries}}
			got, err := applyPredictor(data, &params, 16)
			if err != nil || !bytes.Equal(got, want) {
				t.Fatalf("predictor = %v, %v; want %v", got, err, want)
			}
			entries = append(entries, entries[0])
			params.Value = pdfmodel.Dictionary{Entries: entries}
			if _, err := applyPredictor(data, &params, 16); err == nil {
				t.Fatal("duplicate null accepted")
			}
		})
	}
}

func TestDecodeStreamExhaustedBudget(t *testing.T) {
	// A cumulative byte limit permits exactly the limit, including subsequent
	// empty streams. Failed decodes must not consume budget or enter the cache.
	for _, tc := range []struct{ name, dictionary, empty, positive string }{
		{"raw", "", "", "b"},
		{"hex", "/Filter /ASCIIHexDecode", ">", "62>"},
		{"ascii85", "/Filter /ASCII85Decode", "~>", "@K~>"},
		{"runlength", "/Filter /RunLengthDecode", string([]byte{128}), string([]byte{0, 'b', 128})},
		{"flate", "/Filter /FlateDecode", compressedTestBytes(t, nil), compressedTestBytes(t, []byte{'b'})},
		{"predictor", "/Filter /FlateDecode /DecodeParms << /Predictor 15 >>", compressedTestBytes(t, nil), compressedTestBytes(t, []byte{0, 'b'})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			objects := []string{"<< /Type /Catalog >>", "<< /Length 1 >>\nstream\na\nendstream"}
			for _, payload := range []string{tc.empty, tc.positive} {
				objects = append(objects, fmt.Sprintf("<< /Length %d %s >>\nstream\n%s\nendstream", len(payload), tc.dictionary, payload))
			}
			data := makeTestPDF(objects, "")
			doc, err := Parse(bytes.NewReader(data), int64(len(data)), ReadOptions{Limits: pdfmodel.Limits{MaxDecodedBytes: 1}})
			if err != nil {
				t.Fatal(err)
			}
			for _, number := range []uint32{2, 3, 3, 4, 4} {
				object, err := doc.Load(pdfmodel.ObjectID{Number: number})
				if err != nil {
					t.Fatal(err)
				}
				stream := object.Body.Value.(pdfmodel.Stream)
				source, err := doc.DecodeStream(stream)
				if number == 4 {
					if !errors.Is(err, pdfmodel.ErrLimit) {
						t.Fatalf("positive output error = %v", err)
					}
					if _, cached := doc.decoded[*stream.Encoded]; cached {
						t.Fatal("failed stream cached")
					}
				} else {
					wantSize := int64(0)
					if number == 2 {
						wantSize = 1
					}
					if err != nil || source.Size != wantSize {
						t.Fatalf("object %d: size=%d, err=%v", number, source.Size, err)
					}
				}
				if doc.decodedBytes != 1 {
					t.Fatalf("charged %d bytes; want 1", doc.decodedBytes)
				}
			}
			if len(doc.Sources) != 3 || len(doc.decoded) != 2 {
				t.Fatal("cache retry added a source")
			}
		})
	}
}

func compressedTestBytes(t *testing.T, data []byte) string {
	t.Helper()
	var out bytes.Buffer
	writer := zlib.NewWriter(&out)
	if _, err := writer.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return out.String()
}
