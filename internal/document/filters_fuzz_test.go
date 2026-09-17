package document

import (
	"bytes"
	"compress/zlib"
	"testing"

	"github.com/MyungSub0519/gopd/internal/pdfmodel"
)

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
