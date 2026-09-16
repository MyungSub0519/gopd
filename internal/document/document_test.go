package document

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/MyungSub0519/gopd/internal/pdfmodel"
	"github.com/MyungSub0519/gopd/internal/pdftest"
)

func makeTestPDF(objects []string, trailer string) []byte {
	return pdftest.File(objects, " "+trailer)
}

func parseTestPDF(t *testing.T, data []byte) *Document {
	t.Helper()
	d, err := Parse(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	return d
}

type controlledReaderAt struct {
	data   []byte
	err    error
	calls  int
	closed bool
}

func (r *controlledReaderAt) ReadAt(p []byte, offset int64) (int, error) {
	r.calls++
	n, _ := bytes.NewReader(r.data).ReadAt(p, offset)
	return n, r.err
}

func (r *controlledReaderAt) Close() error {
	r.closed = true
	return nil
}

func TestParseReaderAtErrorContract(t *testing.T) {
	data := makeTestPDF([]string{"<< /Type /Catalog >>"}, "")
	readError := errors.New("test read failure")
	for _, test := range []struct {
		name    string
		data    []byte
		readErr error
		wantErr error
	}{
		{"complete", data, nil, nil},
		{"complete with EOF", data, io.EOF, nil},
		{"short without error", data[:len(data)-1], nil, io.ErrUnexpectedEOF},
		{"short with EOF", data[:len(data)-1], io.EOF, io.EOF},
		{"short with cause", data[:len(data)-1], readError, readError},
		{"complete with failure", data, readError, readError},
	} {
		t.Run(test.name, func(t *testing.T) {
			reader := &controlledReaderAt{data: test.data, err: test.readErr}
			doc, err := Parse(reader, int64(len(data)))
			if !errors.Is(err, test.wantErr) || (doc == nil) != (test.wantErr != nil) {
				t.Fatalf("result present=%t, error=%v; want error=%v", doc != nil, err, test.wantErr)
			}
			if reader.closed {
				t.Fatal("Parse closed the caller-owned reader")
			}
		})
	}
}

func TestParseRejectsInvalidInputBeforeReading(t *testing.T) {
	if doc, err := Parse(nil, 1); doc != nil || err == nil {
		t.Fatal("nil reader must be rejected")
	}
	for _, test := range []struct {
		name    string
		size    int64
		options []ReadOptions
	}{
		{"empty", 0, nil},
		{"negative size", -1, nil},
		{"file budget", 2, []ReadOptions{{MaxFileBytes: 1}}},
		{"negative limit", 1, []ReadOptions{{Limits: pdfmodel.Limits{MaxDepth: -1}}}},
		{"multiple options", 1, []ReadOptions{{}, {}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			reader := &controlledReaderAt{}
			if doc, err := Parse(reader, test.size, test.options...); doc != nil || err == nil {
				t.Fatal("invalid input must be rejected")
			}
			if reader.calls != 0 {
				t.Fatal("invalid input reached ReaderAt")
			}
		})
	}
}

func TestDocumentBytesCopiesAndValidatesRanges(t *testing.T) {
	data := makeTestPDF([]string{"<< /Type /Catalog >>"}, "")
	doc := parseTestPDF(t, data)
	span := pdfmodel.Span{Source: doc.Structure.File, Start: 1, End: 4}
	first, err := doc.Bytes(span)
	if err != nil || string(first) != "PDF" {
		t.Fatalf("Bytes = %q, %v", first, err)
	}
	first[0] = 'X'
	again, err := doc.Bytes(span)
	if err != nil || string(again) != "PDF" {
		t.Fatal("changing returned bytes changed the source")
	}
	for _, invalid := range []pdfmodel.Span{
		{Source: 999, End: 1},
		{Source: span.Source, Start: -1},
		{Source: span.Source, Start: 2, End: 1},
		{Source: span.Source, End: int64(len(data) + 1)},
	} {
		if _, err := doc.Bytes(invalid); err == nil {
			t.Fatalf("accepted invalid span %+v", invalid)
		}
	}
	empty, err := doc.Bytes(pdfmodel.Span{Source: span.Source, Start: int64(len(data)), End: int64(len(data))})
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("empty boundary range = %v, %v", empty, err)
	}
}

func TestParseStructuralFailureReturnsPartialDocument(t *testing.T) {
	data := []byte("%PDF-1.7\n")
	doc, err := Parse(bytes.NewReader(data), int64(len(data)))
	if err == nil || doc == nil || doc.Structure.Header == nil {
		t.Fatal("missing xref must retain the parsed header and an error")
	}
	raw, readErr := doc.Bytes(pdfmodel.Span{Source: doc.Structure.File, End: int64(len(data))})
	if readErr != nil || !bytes.Equal(raw, data) {
		t.Fatal("partial document lost its source bytes")
	}
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
	obj, err := d.Resolve(pdfmodel.Reference{ID: pdfmodel.ObjectID{Number: 3}})
	if err != nil {
		t.Fatal(err)
	}
	s := obj.Value.(pdfmodel.Stream)
	if s.Boundary != pdfmodel.StreamFromLength || s.Encoded == nil {
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
	if _, err := d.Resolve(pdfmodel.Reference{ID: pdfmodel.ObjectID{Number: 2}}); err == nil {
		t.Fatal("deleted object resurrected from older xref")
	}
}

func TestDocumentRejectsGenerationMismatchAndXRefCycle(t *testing.T) {
	base := makeTestPDF([]string{"<< /Type /Catalog >>"}, "")
	d := parseTestPDF(t, base)
	if _, err := d.Resolve(pdfmodel.Reference{ID: pdfmodel.ObjectID{Number: 1, Generation: 1}}); err == nil {
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
	obj, err := d.Load(pdfmodel.ObjectID{Number: 3})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := obj.Origin.(pdfmodel.ObjectStreamOrigin); !ok {
		t.Fatalf("origin %T", obj.Origin)
	}
	if obj.Body.Span.Source == d.Structure.File {
		t.Fatal("compressed object mapped to file offset")
	}
	dict := obj.Body.Value.(pdfmodel.Dictionary)
	count, err := dict.Get("Count")
	if err != nil {
		t.Fatal(err)
	}
	n, err := pdfmodel.Int(count)
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
	for _, span := range []pdfmodel.Span{{Source: 1, Start: -1, End: 1}, {Source: 1, Start: 2, End: 1}, {Source: 999, End: 1}, {Source: 1, End: 1 << 62}} {
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
		d, err := Parse(bytes.NewReader(data), int64(len(data)), ReadOptions{MaxFileBytes: 65536, Limits: pdfmodel.Limits{MaxDepth: 32, MaxObjects: 128, MaxXRefSections: 16, MaxDecodedBytes: 65536}})
		if err != nil {
			return
		}
		numbers := make([]uint32, 0, len(d.entries))
		for number := range d.entries {
			numbers = append(numbers, number)
		}
		slices.Sort(numbers)
		for _, number := range numbers {
			record := d.entries[number]
			var id pdfmodel.ObjectID
			switch e := record.Entry.(type) {
			case pdfmodel.InUseEntry:
				id = pdfmodel.ObjectID{Number: number, Generation: e.Generation}
			case pdfmodel.CompressedEntry:
				id = pdfmodel.ObjectID{Number: number}
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
			if stream, ok := obj.Body.Value.(pdfmodel.Stream); ok {
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

func TestDocumentRejectsFalseStreamBoundary(t *testing.T) {
	data := makeTestPDF([]string{"<< /Type /Catalog >>", "<< /Length 1 >>\nstream\nA%discarded payload\nendstream"}, "")
	d := parseTestPDF(t, data)
	o, err := d.Load(pdfmodel.ObjectID{Number: 2})
	if err == nil {
		s := o.Body.Value.(pdfmodel.Stream)
		got, _ := d.Bytes(*s.Encoded)
		t.Fatalf("accepted length cutting before a non-EOL payload byte; encoded bytes %q", got)
	}
}

func TestDocumentEmptyXRefRangesNeedBudget(t *testing.T) {
	data := []byte("%PDF-1.7\nxref\n" + strings.Repeat("0 0\n", 10000) + "trailer\n<< /Size 0 >>\nstartxref\n9\n%%EOF\n")
	d, err := Parse(bytes.NewReader(data), int64(len(data)), ReadOptions{Limits: pdfmodel.Limits{MaxObjects: 2}})
	if err == nil {
		t.Fatalf("retained %d xref ranges from %d bytes despite MaxObjects=2", len(d.Structure.XRefs[0].Ranges), len(data))
	}
}

func TestDocumentHistoricalXRefRecordsNeedBudget(t *testing.T) {
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
	d, err := Parse(bytes.NewReader(file.Bytes()), int64(file.Len()), ReadOptions{Limits: pdfmodel.Limits{MaxObjects: 8}})
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
