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
	f.Fuzz(func(t *testing.T, filter uint8, data []byte) {
		if len(data) > 4096 {
			t.Skip()
		}
		names := [...]pdfmodel.Name{"ASCIIHexDecode", "ASCII85Decode", "RunLengthDecode", "FlateDecode"}
		const limit = 32768
		out, err := decodeFilter(names[int(filter)%len(names)], data, nil, limit)
		if err == nil && len(out) > limit {
			t.Fatalf("successful filter exceeded budget: %d > %d", len(out), limit)
		}
	})
}
