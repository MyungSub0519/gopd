package document

import (
	"bytes"
	"compress/zlib"
	"encoding/ascii85"
	"fmt"
	"math"
	"testing"

	"github.com/MyungSub0519/gopd/internal/pdfmodel"
)

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
