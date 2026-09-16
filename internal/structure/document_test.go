package structure

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"strings"
	"testing"

	"github.com/MyungSub0519/gopd/internal/model"
)

// This fixture writer is deliberately independent of the reader under test.
func makeTestPDF(objects []string, trailer string) []byte {
	var b bytes.Buffer
	b.WriteString("%PDF-1.7\n")
	offsets := make([]int, len(objects)+1)
	for i, object := range objects {
		offsets[i+1] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, object)
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(offsets))
	for _, off := range offsets[1:] {
		fmt.Fprintf(&b, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R %s >>\nstartxref\n%d\n%%%%EOF\n", len(offsets), trailer, xref)
	return b.Bytes()
}

func parseTestPDF(t *testing.T, data []byte) *Document {
	t.Helper()
	d, err := Parse(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestDocumentResolvesIndirectLengthAndPreservesBinary(t *testing.T) {
	payload := "abc endstream xyz\x00\xff"
	data := makeTestPDF([]string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Count 0 /Kids [] >>",
		"<< /Length 4 0 R >>\nstream\n" + payload + "\nendstream",
		fmt.Sprint(len(payload)),
	}, "")
	d := parseTestPDF(t, data)
	obj, err := d.Resolve(model.Reference{ID: model.ObjectID{Number: 3}})
	if err != nil {
		t.Fatal(err)
	}
	s := obj.Value.(model.Stream)
	if s.Boundary != model.StreamFromLength || s.Encoded == nil {
		t.Fatalf("unresolved stream: %#v", s)
	}
	raw, err := d.Bytes(*s.Encoded)
	if err != nil || string(raw) != payload {
		t.Fatalf("payload %q, err %v", raw, err)
	}
	decoded, err := d.DecodeStream(s)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.ID == d.Structure.File || decoded.Origin == nil {
		t.Fatal("decoded source has no distinct provenance")
	}
	// Parse takes a snapshot: caller modifications cannot change source spans.
	for i := range data {
		data[i] = 0
	}
	again, err := d.Bytes(*s.Encoded)
	if err != nil || string(again) != payload {
		t.Fatal("input snapshot changed")
	}
}

func TestDocumentDecodesFlateAndCachesSource(t *testing.T) {
	var compressed bytes.Buffer
	w := zlib.NewWriter(&compressed)
	_, _ = w.Write([]byte("BT (hello) Tj ET"))
	_ = w.Close()
	data := makeTestPDF([]string{"<< /Type /Catalog >>", fmt.Sprintf("<< /Length %d /Filter /FlateDecode >>\nstream\n%s\nendstream", compressed.Len(), compressed.String())}, "")
	d := parseTestPDF(t, data)
	obj, err := d.Resolve(model.Reference{ID: model.ObjectID{Number: 2}})
	if err != nil {
		t.Fatal(err)
	}
	src, err := d.DecodeStream(obj.Value.(model.Stream))
	if err != nil {
		t.Fatal(err)
	}
	got, err := d.Bytes(model.Span{Source: src.ID, End: src.Size})
	if err != nil || string(got) != "BT (hello) Tj ET" {
		t.Fatalf("%q %v", got, err)
	}
	next, err := d.DecodeStream(obj.Value.(model.Stream))
	if err != nil || next.ID != src.ID {
		t.Fatal("decoded source not cached", err)
	}
}

func TestDocumentIncrementalFreeEntryShadowsOlderObject(t *testing.T) {
	base := makeTestPDF([]string{"<< /Type /Catalog >>", "(old)"}, "")
	prev := bytes.Index(base, []byte("xref\n"))
	var b bytes.Buffer
	b.Write(base)
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n2 1\n0000000000 00001 f \ntrailer\n<< /Size 3 /Root 1 0 R /Prev %d >>\nstartxref\n%d\n%%%%EOF\n", prev, xref)
	d := parseTestPDF(t, b.Bytes())
	if len(d.Structure.XRefs) != 2 {
		t.Fatalf("sections=%d", len(d.Structure.XRefs))
	}
	if _, err := d.Resolve(model.Reference{ID: model.ObjectID{Number: 2}}); err == nil {
		t.Fatal("deleted object resurrected from older xref")
	}
}

func TestDocumentRejectsGenerationMismatchAndXRefCycle(t *testing.T) {
	base := makeTestPDF([]string{"<< /Type /Catalog >>"}, "")
	d := parseTestPDF(t, base)
	if _, err := d.Resolve(model.Reference{ID: model.ObjectID{Number: 1, Generation: 1}}); err == nil {
		t.Fatal("generation mismatch accepted")
	}
	var b bytes.Buffer
	b.Write(base)
	off := b.Len()
	fmt.Fprintf(&b, "xref\n0 1\n0000000000 65535 f \ntrailer\n<< /Size 2 /Root 1 0 R /Prev %d >>\nstartxref\n%d\n%%%%EOF\n", off, off)
	if _, err := Parse(bytes.NewReader(b.Bytes()), int64(b.Len())); err == nil {
		t.Fatal("xref cycle accepted")
	}
}

func TestDocumentReadsXRefAndObjectStreams(t *testing.T) {
	var b bytes.Buffer
	b.WriteString("%PDF-1.7\n")
	off1 := b.Len()
	b.WriteString("1 0 obj\n<< /Type /Catalog /Pages 3 0 R >>\nendobj\n")
	payload := "3 0 << /Type /Pages /Count 0 /Kids [] >>"
	off2 := b.Len()
	fmt.Fprintf(&b, "2 0 obj\n<< /Type /ObjStm /N 1 /First 4 /Length %d >>\nstream\n%s\nendstream\nendobj\n", len(payload), payload)
	off4 := b.Len()
	var records bytes.Buffer
	for _, fields := range [][3]uint32{{0, 0, 65535}, {1, uint32(off1), 0}, {1, uint32(off2), 0}, {2, 2, 0}, {1, uint32(off4), 0}} {
		records.WriteByte(byte(fields[0]))
		_ = binary.Write(&records, binary.BigEndian, fields[1])
		_ = binary.Write(&records, binary.BigEndian, uint16(fields[2]))
	}
	fmt.Fprintf(&b, "4 0 obj\n<< /Type /XRef /Size 5 /Root 1 0 R /W [1 4 2] /Length %d >>\nstream\n%s\nendstream\nendobj\nstartxref\n%d\n%%%%EOF\n", records.Len(), records.String(), off4)
	d := parseTestPDF(t, b.Bytes())
	obj, err := d.Load(model.ObjectID{Number: 3})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := obj.Origin.(model.ObjectStreamOrigin); !ok {
		t.Fatalf("origin %T", obj.Origin)
	}
	if obj.Body.Span.Source == d.Structure.File {
		t.Fatal("compressed object mapped to file offset")
	}
	dict := obj.Body.Value.(model.Dictionary)
	count, err := dict.Get("Count")
	if err != nil {
		t.Fatal(err)
	}
	n, err := model.Int(count)
	if err != nil || n != 0 {
		t.Fatal("wrong compressed object value", err)
	}
}

func TestDocumentRejectsInvalidBoundsAndHugeCounts(t *testing.T) {
	if _, err := Parse(strings.NewReader("%PDF"), -1); err == nil {
		t.Fatal("negative size accepted")
	}
	data := []byte("%PDF-1.7\nxref\n0 4294967295\ntrailer\n<< /Size 4294967295 >>\nstartxref\n9\n%%EOF\n")
	if _, err := Parse(bytes.NewReader(data), int64(len(data))); err == nil {
		t.Fatal("huge xref accepted")
	}
	d := parseTestPDF(t, makeTestPDF([]string{"<< /Type /Catalog >>"}, ""))
	for _, span := range []model.Span{{Source: 1, Start: -1, End: 1}, {Source: 1, Start: 2, End: 1}, {Source: 999, End: 1}, {Source: 1, End: 1 << 62}} {
		if _, err := d.Bytes(span); err == nil {
			t.Fatalf("accepted span %+v", span)
		}
	}
}

func FuzzDocument(f *testing.F) {
	f.Add(makeTestPDF([]string{"<< /Type /Catalog /Pages 2 0 R >>", "<< /Type /Pages /Count 0 /Kids [] >>"}, ""))
	f.Add(makeTestPDF([]string{"<< /Type /Catalog >>", "<< /Length 3 >>\nstream\nabc\nendstream"}, ""))
	f.Add([]byte("%PDF-1.7\nstartxref\n0\n%%EOF"))
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 65536 {
			t.Skip()
		}
		d, err := Parse(bytes.NewReader(data), int64(len(data)), ReadOptions{MaxFileBytes: 65536, Limits: model.Limits{MaxDepth: 32, MaxObjects: 128, MaxXRefSections: 16, MaxDecodedBytes: 65536}})
		if err != nil {
			return
		}
		for number, record := range d.entries {
			var id model.ObjectID
			switch e := record.Entry.(type) {
			case model.InUseEntry:
				id = model.ObjectID{Number: number, Generation: e.Generation}
			case model.CompressedEntry:
				id = model.ObjectID{Number: number}
			default:
				continue
			}
			obj, e := d.Load(id)
			if e != nil {
				continue
			}
			if _, e = d.Bytes(obj.Body.Span); e != nil {
				t.Fatalf("successful object has invalid source span: %v", e)
			}
			if stream, ok := obj.Body.Value.(model.Stream); ok {
				_, _ = d.DecodeStream(stream)
			}
		}
		var end int64
		for _, r := range d.Structure.Regions {
			if r.Span.Start != end || r.Span.End <= end || r.Span.Source != 1 {
				t.Fatal("invalid file partition")
			}
			end = r.Span.End
		}
		if end != int64(len(data)) {
			t.Fatal("incomplete file partition")
		}
	})
}
