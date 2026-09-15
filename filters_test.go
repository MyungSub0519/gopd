package pdf

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"testing"
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
			obj, err := d.Resolve(Reference{ID: ObjectID{Number: 2}})
			if err != nil {
				t.Fatal(err)
			}
			src, err := d.DecodeStream(obj.Value.(Stream))
			if tt.name == "unsupported" {
				if err == nil {
					t.Fatal("unknown filter accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			got, err := d.Bytes(Span{Source: src.ID, End: src.Size})
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
	obj, err := d.Resolve(Reference{ID: ObjectID{Number: 2}})
	if err != nil {
		t.Fatal(err)
	}
	src, err := d.DecodeStream(obj.Value.(Stream))
	if err != nil {
		t.Fatal(err)
	}
	got, err := d.Bytes(Span{Source: src.ID, End: src.Size})
	want := []byte{10, 20, 30, 15, 25, 35}
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("%v want %v (%v)", got, want, err)
	}
}

func TestStreamDecodeBudgetAndWrongLength(t *testing.T) {
	data := makeTestPDF([]string{"<< /Type /Catalog >>", "<< /Length 5 >>\nstream\nabc\nendstream"}, "")
	d := parseTestPDF(t, data)
	if _, err := d.Load(ObjectID{Number: 2}); err == nil {
		t.Fatal("incorrect Length accepted")
	}
	var compressed bytes.Buffer
	w := zlib.NewWriter(&compressed)
	_, _ = w.Write(bytes.Repeat([]byte{'a'}, 1024))
	_ = w.Close()
	data = makeTestPDF([]string{"<< /Type /Catalog >>", fmt.Sprintf("<< /Length %d /Filter /FlateDecode >>\nstream\n%s\nendstream", compressed.Len(), compressed.String())}, "")
	d, err := Parse(bytes.NewReader(data), int64(len(data)), ReadOptions{Limits: Limits{MaxDecodedBytes: 64}})
	if err != nil {
		t.Fatal(err)
	}
	obj, err := d.Resolve(Reference{ID: ObjectID{Number: 2}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = d.DecodeStream(obj.Value.(Stream)); err == nil {
		t.Fatal("decompression budget ignored")
	}
}
