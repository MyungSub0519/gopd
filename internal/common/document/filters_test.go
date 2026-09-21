package document

import (
	"bytes"
	"compress/zlib"
	"encoding/ascii85"
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/MyungSub0519/gopd/internal/common/pdfmodel"
)

func TestDocumentDecodesFlateAndCachesSource(t *testing.T) {
	var compressed bytes.Buffer
	w := zlib.NewWriter(&compressed)
	_, _ = w.Write([]byte("BT (hello) Tj ET"))
	_ = w.Close()
	data := makeTestPDF([]string{"<< /Type /Catalog >>", fmt.Sprintf("<< /Length %d /Filter /FlateDecode >>\nstream\n%s\nendstream", compressed.Len(), compressed.String())}, "")
	d := parseTestPDF(t, data)
	obj, err := d.Resolve(pdfmodel.Reference{ID: pdfmodel.ObjectID{Number: 2}})
	if err != nil {
		t.Fatal(err)
	}
	src, err := d.DecodeStream(obj.Value.(pdfmodel.Stream))
	if err != nil {
		t.Fatal(err)
	}
	got, err := d.Bytes(pdfmodel.Span{Source: src.ID, End: src.Size})
	if err != nil || string(got) != "BT (hello) Tj ET" {
		t.Fatalf("%q %v", got, err)
	}
	next, err := d.DecodeStream(obj.Value.(pdfmodel.Stream))
	if err != nil || next.ID != src.ID {
		t.Fatal("decoded source not cached", err)
	}
}

func FuzzDecodeFilters(f *testing.F) {
	f.Add(uint8(0), []byte("61 62 6>"))
	f.Add(uint8(1), []byte("BOu!rDZ~>"))
	f.Add(uint8(2), []byte{2, 'a', 'b', 'c', 128})
	var compressed bytes.Buffer
	w := zlib.NewWriter(&compressed)
	if _, err := w.Write([]byte("filter seed")); err != nil {
		f.Fatal(err)
	}
	if err := w.Close(); err != nil {
		f.Fatal(err)
	}
	f.Add(uint8(3), compressed.Bytes())
	// Exercise valid TIFF and PNG parameter dictionaries, not only the
	// no-predictor path (Adobe PDF Reference 1.6, section 3.3.3).
	for _, seed := range []struct {
		filter uint8
		data   []byte
	}{
		{4, []byte{10, 10, 10}},
		{5, []byte{1, 10, 10, 10, 2, 5, 5, 5}},
	} {
		var encoded bytes.Buffer
		writer := zlib.NewWriter(&encoded)
		if _, err := writer.Write(seed.data); err != nil {
			f.Fatal(err)
		}
		if err := writer.Close(); err != nil {
			f.Fatal(err)
		}
		f.Add(seed.filter, encoded.Bytes())
	}
	f.Fuzz(func(t *testing.T, filter uint8, data []byte) {
		if len(data) > 4096 {
			t.Skip()
		}
		names := [...]pdfmodel.Name{"ASCIIHexDecode", "ASCII85Decode", "RunLengthDecode", "FlateDecode", "FlateDecode", "FlateDecode"}
		selected := int(filter) % len(names)
		var params *pdfmodel.Object
		if selected >= 4 {
			predictor := pdfmodel.Integer("2")
			if selected == 5 {
				predictor = "15"
			}
			params = &pdfmodel.Object{Value: pdfmodel.Dictionary{Entries: []pdfmodel.DictionaryEntry{
				{Key: "Predictor", Value: pdfmodel.Object{Value: predictor}},
				{Key: "Columns", Value: pdfmodel.Object{Value: pdfmodel.Integer("3")}},
				{Key: "Colors", Value: pdfmodel.Object{Value: pdfmodel.Null{}}},
				{Key: "BitsPerComponent", Value: pdfmodel.Object{Value: pdfmodel.Null{}}},
			}}}
		}
		const limit = 32768
		out, err := decodeFilter(names[selected], data, params, limit)
		if err == nil && len(out) > limit {
			t.Fatalf("successful filter exceeded budget: %d > %d", len(out), limit)
		}
	})
}

func TestStreamFiltersAndPredictors(t *testing.T) {
	for _, tt := range []struct{ name, filter, payload, want, params string }{
		{"hex", "ASCIIHexDecode", "61 62 6>", "ab`", ""},
		{"ascii85", "ASCII85Decode", "BOu!rDZ~>", "hello", ""},
		{"runlength", "RunLengthDecode", string([]byte{2, 'a', 'b', 'c', 254, 'x', 128}), "abcxxx", ""},
		{"unsupported", "UnknownDecode", "123", "", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			data := makeTestPDF([]string{"<< /Type /Catalog >>", fmt.Sprintf("<< /Length %d /Filter /%s %s >>\nstream\n%s\nendstream", len(tt.payload), tt.filter, tt.params, tt.payload)}, "")
			d := parseTestPDF(t, data)
			obj, err := d.Resolve(pdfmodel.Reference{ID: pdfmodel.ObjectID{Number: 2}})
			if err != nil {
				t.Fatal(err)
			}
			src, err := d.DecodeStream(obj.Value.(pdfmodel.Stream))
			if tt.name == "unsupported" {
				if err == nil {
					t.Fatal("unknown filter accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			got, err := d.Bytes(pdfmodel.Span{Source: src.ID, End: src.Size})
			if err != nil || string(got) != tt.want {
				t.Fatalf("%q want %q, %v", got, tt.want, err)
			}
		})
	}
	var compressed bytes.Buffer
	w := zlib.NewWriter(&compressed)
	_, _ = w.Write([]byte{1, 10, 10, 10, 2, 5, 5, 5})
	_ = w.Close()
	data := makeTestPDF([]string{"<< /Type /Catalog >>", fmt.Sprintf("<< /Length %d /Filter /FlateDecode /DecodeParms << /Predictor 15 /Columns 3 >> >>\nstream\n%s\nendstream", compressed.Len(), compressed.String())}, "")
	d := parseTestPDF(t, data)
	obj, err := d.Resolve(pdfmodel.Reference{ID: pdfmodel.ObjectID{Number: 2}})
	if err != nil {
		t.Fatal(err)
	}
	src, err := d.DecodeStream(obj.Value.(pdfmodel.Stream))
	if err != nil {
		t.Fatal(err)
	}
	got, err := d.Bytes(pdfmodel.Span{Source: src.ID, End: src.Size})
	want := []byte{10, 20, 30, 15, 25, 35}
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("%v want %v (%v)", got, want, err)
	}
}

func TestStreamDecodeBudgetAndWrongLength(t *testing.T) {
	data := makeTestPDF([]string{"<< /Type /Catalog >>", "<< /Length 5 >>\nstream\nabc\nendstream"}, "")
	d := parseTestPDF(t, data)
	if _, err := d.Load(pdfmodel.ObjectID{Number: 2}); err == nil {
		t.Fatal("incorrect Length accepted")
	}
	var compressed bytes.Buffer
	w := zlib.NewWriter(&compressed)
	_, _ = w.Write(bytes.Repeat([]byte{'a'}, 1024))
	_ = w.Close()
	data = makeTestPDF([]string{"<< /Type /Catalog >>", fmt.Sprintf("<< /Length %d /Filter /FlateDecode >>\nstream\n%s\nendstream", compressed.Len(), compressed.String())}, "")
	d, err := Parse(bytes.NewReader(data), int64(len(data)), ReadOptions{Limits: pdfmodel.Limits{MaxDecodedBytes: 64}})
	if err != nil {
		t.Fatal(err)
	}
	obj, err := d.Resolve(pdfmodel.Reference{ID: pdfmodel.ObjectID{Number: 2}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = d.DecodeStream(obj.Value.(pdfmodel.Stream)); err == nil {
		t.Fatal("decompression budget ignored")
	}
}

func decodeWithLimit(t *testing.T, filter string, payload []byte, limit int64) ([]byte, error) {
	t.Helper()
	data := makeTestPDF([]string{"<< /Type /Catalog >>", fmt.Sprintf("<< /Length %d /Filter /%s >>\nstream\n%s\nendstream", len(payload), filter, payload)}, "")
	d, err := Parse(bytes.NewReader(data), int64(len(data)), ReadOptions{Limits: pdfmodel.Limits{MaxDecodedBytes: limit}})
	if err != nil {
		return nil, err
	}
	o, err := d.Resolve(pdfmodel.Reference{ID: pdfmodel.ObjectID{Number: 2}})
	if err != nil {
		return nil, err
	}
	s, err := d.DecodeStream(o.Value.(pdfmodel.Stream))
	if err != nil {
		return nil, err
	}
	return d.Bytes(pdfmodel.Span{Source: s.ID, End: s.Size})
}

func TestStreamLargeDecodedLimit(t *testing.T) {
	var compressed bytes.Buffer
	w := zlib.NewWriter(&compressed)
	_, _ = w.Write([]byte("hello"))
	_ = w.Close()
	for _, tc := range []struct {
		name    string
		payload []byte
	}{
		{"ASCIIHexDecode", []byte("68656c6c6f>")},
		{"FlateDecode", compressed.Bytes()},
		{"ASCII85Decode", []byte("BOu!rDZ~>")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("public DecodeStream panicked: %v", r)
				}
			}()
			got, err := decodeWithLimit(t, tc.name, tc.payload, math.MaxInt64)
			if err != nil || string(got) != "hello" {
				t.Errorf("decoded %q, error %v; want hello", got, err)
			}
		})
	}
}

func TestStreamASCII85AtBudget(t *testing.T) {
	for size := 1; size <= 10; size++ {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			want := bytes.Repeat([]byte{'a'}, size)
			encoded := make([]byte, ascii85.MaxEncodedLen(size))
			n := ascii85.Encode(encoded, want)
			payload := append(encoded[:n], '~', '>')
			got, err := decodeWithLimit(t, "ASCII85Decode", payload, int64(size))
			if err != nil || !bytes.Equal(got, want) {
				t.Errorf("decoded %q, error %v; want %q within %d-byte budget", got, err, want, size)
			}
		})
	}
}

func TestDecodeStreamAccountsForCompressedDependencies(t *testing.T) {
	for _, dependency := range []struct{ name, dictionary, body string }{
		{"filter", "/Filter 4 0 R", "/FlateDecode"},
		{"filter array item", "/Filter [4 0 R]", "/FlateDecode"},
		{"parameters", "/Filter /FlateDecode /DecodeParms 4 0 R", "<< /Predictor 1 >>"},
		{"parameter array item", "/Filter [/FlateDecode] /DecodeParms [4 0 R]", "<< /Predictor 1 >>"},
	} {
		t.Run(dependency.name, func(t *testing.T) {
			var compressed bytes.Buffer
			writer := zlib.NewWriter(&compressed)
			_, _ = writer.Write(bytes.Repeat([]byte{'a'}, 60))
			_ = writer.Close()
			payload := "4 0 " + dependency.body + strings.Repeat(" ", 61-4-len(dependency.body))
			data := xrefStreamFile(map[int]string{
				1: "<< /Type /Catalog >>",
				2: fmt.Sprintf("<< /Length %d %s >>\nstream\n%s\nendstream", compressed.Len(), dependency.dictionary, compressed.String()),
				3: fmt.Sprintf("<< /Type /ObjStm /N 1 /First 4 /Length 61 >>\nstream\n%s\nendstream", payload),
			}, map[int][2]int{4: {3, 0}}, 5)
			for _, limit := range []int64{130, 163} {
				doc, err := Parse(bytes.NewReader(data), int64(len(data)), ReadOptions{Limits: pdfmodel.Limits{MaxDecodedBytes: limit}})
				if err != nil {
					t.Fatal(err)
				}
				object, err := doc.Load(pdfmodel.ObjectID{Number: 2})
				if err != nil {
					t.Fatal(err)
				}
				source, err := doc.DecodeStream(object.Body.Value.(pdfmodel.Stream))
				if limit == 130 && err == nil {
					t.Errorf("decoded %d bytes with limit %d", doc.decodedBytes, limit)
				}
				if limit == 130 && !errors.Is(err, pdfmodel.ErrLimit) {
					t.Errorf("decode budget error does not identify ErrLimit: %v", err)
				}
				if doc.decodedBytes > limit {
					t.Errorf("cumulative decoded bytes %d exceed %d", doc.decodedBytes, limit)
				}
				if limit == 163 && (err != nil || source.Size != 60) {
					t.Errorf("exact budget: source=%+v err=%v", source, err)
				}
			}
		})
	}
}

func TestDecodeStreamOptionalNullEntries(t *testing.T) {
	for _, dictionary := range []string{"/Filter null", "/F null", "/DecodeParms null", "/Filter 3 0 R", "/F 3 0 R", "/DecodeParms 3 0 R"} {
		t.Run(dictionary, func(t *testing.T) {
			data := makeTestPDF([]string{"<< /Type /Catalog >>", "<< /Length 3 " + dictionary + " >>\nstream\nabc\nendstream", "null"}, "")
			doc := parseTestPDF(t, data)
			object, err := doc.Load(pdfmodel.ObjectID{Number: 2})
			if err != nil {
				t.Fatal(err)
			}
			source, err := doc.DecodeStream(object.Body.Value.(pdfmodel.Stream))
			if err != nil {
				t.Fatal(err)
			}
			decoded, err := doc.Bytes(pdfmodel.Span{Source: source.ID, End: source.Size})
			if err != nil || string(decoded) != "abc" {
				t.Fatalf("decoded %q, error %v", decoded, err)
			}
		})
	}
}

func TestDecodeStreamPreservesIndirectFilterSyntax(t *testing.T) {
	data := makeTestPDF([]string{"<< /Type /Catalog >>", "<< /Length 3 /Filter [3 0 R] >>\nstream\n61>\nendstream", "/ASCIIHexDecode"}, "")
	doc := parseTestPDF(t, data)
	object, err := doc.Load(pdfmodel.ObjectID{Number: 2})
	if err != nil {
		t.Fatal(err)
	}
	stream := object.Body.Value.(pdfmodel.Stream)
	if _, err := doc.DecodeStream(stream); err != nil {
		t.Fatal(err)
	}
	filter, err := stream.Dictionary.Get("Filter")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := filter.Value.(pdfmodel.Array).Items[0].Value.(pdfmodel.Reference); !ok {
		t.Fatal("decoding replaced the original indirect filter syntax")
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
