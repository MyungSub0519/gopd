package structure

import (
	"bytes"
	"compress/zlib"
	"encoding/ascii85"
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/MyungSub0519/gopd/internal/model"
)

func reviewDecodeWithLimit(t *testing.T, filter string, payload []byte, limit int64) ([]byte, error) {
	t.Helper()
	data := makeTestPDF([]string{"<< /Type /Catalog >>", fmt.Sprintf("<< /Length %d /Filter /%s >>\nstream\n%s\nendstream", len(payload), filter, payload)}, "")
	d, err := Parse(bytes.NewReader(data), int64(len(data)), ReadOptions{Limits: model.Limits{MaxDecodedBytes: limit}})
	if err != nil {
		return nil, err
	}
	o, err := d.Resolve(model.Reference{ID: model.ObjectID{Number: 2}})
	if err != nil {
		return nil, err
	}
	s, err := d.DecodeStream(o.Value.(model.Stream))
	if err != nil {
		return nil, err
	}
	return d.Bytes(model.Span{Source: s.ID, End: s.Size})
}

func TestReviewReaderLargeDecodedLimit(t *testing.T) {
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
			got, err := reviewDecodeWithLimit(t, tc.name, tc.payload, math.MaxInt64)
			if err != nil || string(got) != "hello" {
				t.Errorf("decoded %q, error %v; want hello", got, err)
			}
		})
	}
}

func TestReviewReaderASCII85AtBudget(t *testing.T) {
	for size := 1; size <= 10; size++ {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			want := bytes.Repeat([]byte{'a'}, size)
			encoded := make([]byte, ascii85.MaxEncodedLen(size))
			n := ascii85.Encode(encoded, want)
			payload := append(encoded[:n], '~', '>')
			got, err := reviewDecodeWithLimit(t, "ASCII85Decode", payload, int64(size))
			if err != nil || !bytes.Equal(got, want) {
				t.Errorf("decoded %q, error %v; want %q within %d-byte budget", got, err, want, size)
			}
		})
	}
}

func TestReviewReaderRejectsFalseStreamBoundary(t *testing.T) {
	data := makeTestPDF([]string{"<< /Type /Catalog >>", "<< /Length 1 >>\nstream\nA%discarded payload\nendstream"}, "")
	d := parseTestPDF(t, data)
	o, err := d.Load(model.ObjectID{Number: 2})
	if err == nil {
		s := o.Body.Value.(model.Stream)
		got, _ := d.Bytes(*s.Encoded)
		t.Fatalf("accepted length cutting before a non-EOL payload byte; encoded bytes %q", got)
	}
}

func TestReviewReaderEmptyXRefRangesNeedBudget(t *testing.T) {
	data := []byte("%PDF-1.7\nxref\n" + strings.Repeat("0 0\n", 10000) + "trailer\n<< /Size 0 >>\nstartxref\n9\n%%EOF\n")
	d, err := Parse(bytes.NewReader(data), int64(len(data)), ReadOptions{Limits: model.Limits{MaxObjects: 2}})
	if err == nil {
		t.Fatalf("retained %d xref ranges from %d bytes despite MaxObjects=2", len(d.Structure.XRefs[0].Ranges), len(data))
	}
}

func TestReviewReaderHistoricalXRefRecordsNeedBudget(t *testing.T) {
	var file bytes.Buffer
	file.WriteString("%PDF-1.7\n")
	previous := -1
	for section := 1; section <= 4; section++ {
		here := file.Len()
		prev := ""
		if previous >= 0 {
			prev = fmt.Sprintf(" /Prev %d", previous)
		}
		fmt.Fprintf(&file, "%d 0 obj\n<< /Type /XRef /Size 8 /W [1 0 0] /Length 8%s >>\nstream\n%s\nendstream\nendobj\n", section, prev, strings.Repeat("\x00", 8))
		previous = here
	}
	fmt.Fprintf(&file, "startxref\n%d\n%%%%EOF\n", previous)
	d, err := Parse(bytes.NewReader(file.Bytes()), int64(file.Len()), ReadOptions{Limits: model.Limits{MaxObjects: 8}})
	if err == nil {
		total := 0
		for _, section := range d.Structure.XRefs {
			for _, r := range section.Ranges {
				total += len(r.Records)
			}
		}
		t.Fatalf("retained %d historical records despite MaxObjects=8; source size %d decoded bytes %d", total, file.Len(), d.decodedBytes)
	}
}
