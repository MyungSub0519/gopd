package document

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/MyungSub0519/gopd/internal/common/pdfmodel"
	"github.com/MyungSub0519/gopd/internal/common/pdftest"
)

func TestJoinedContentSourceReadAtAndProvenance(t *testing.T) {
	d := &Document{
		Sources: map[pdfmodel.SourceID]pdfmodel.Source{
			1: {ID: 1, Reader: bytes.NewReader([]byte("ab")), Size: 2},
			2: {ID: 2, Reader: bytes.NewReader([]byte("XYZ")), Size: 3},
			3: {ID: 3, Reader: bytes.NewReader(nil)},
		},
		Options: ReadOptions{Limits: pdfmodel.Limits{MaxContentBytes: 6, MaxSemanticObjects: 4}},
	}
	inputs := []pdfmodel.Span{{Source: 1, End: 2}, {Source: 3}, {Source: 2, Start: 1, End: 3}, {Source: 1, End: 2}}
	source, err := JoinContentSources(d, inputs)
	if err != nil {
		t.Fatal(err)
	}
	inputs[0].End = 0
	if source.Origin.Inputs[0].End != 2 || source.Size != 6 {
		t.Fatalf("incorrect or aliased provenance: %+v", source)
	}
	want := bytes.NewReader([]byte("abYZab"))
	for offset := int64(0); offset <= 7; offset++ {
		for length := 1; length <= 8; length++ {
			actual, expected := make([]byte, length), make([]byte, length)
			n, err := source.Reader.ReadAt(actual, offset)
			wn, we := want.ReadAt(expected, offset)
			if n != wn || !bytes.Equal(actual, expected) || errors.Is(err, io.EOF) != errors.Is(we, io.EOF) {
				t.Fatalf("offset=%d length=%d: (%q, %d, %v), want (%q, %d, %v)", offset, length, actual, n, err, expected, wn, we)
			}
		}
	}
	if _, err := source.Reader.ReadAt(make([]byte, 1), -1); err == nil {
		t.Fatal("negative offset accepted")
	}
	if n, err := source.Reader.ReadAt(nil, 0); n != 0 || err != nil {
		t.Fatalf("empty read: %d, %v", n, err)
	}
	part, err := d.Bytes(pdfmodel.Span{Source: source.ID, Start: 1, End: 5})
	if err != nil || string(part) != "bYZa" {
		t.Fatalf("cross-source Bytes = %q, %v", part, err)
	}
}

func TestJoinedContentSourceRejectsInvalidOrOverBudgetInputs(t *testing.T) {
	for _, inputs := range [][]pdfmodel.Span{
		nil,
		{{Source: 2}},
		{{Source: 1, Start: -1, End: 1}},
		{{Source: 1, Start: 2, End: 1}},
		{{Source: 1, End: 4}},
		{{Source: 1, End: 3}, {Source: 1, End: 1}},
		{{Source: 1}, {Source: 1}, {Source: 1}},
	} {
		d := &Document{
			Sources: map[pdfmodel.SourceID]pdfmodel.Source{1: {ID: 1, Reader: bytes.NewReader([]byte("abc")), Size: 3}},
			Options: ReadOptions{Limits: pdfmodel.Limits{MaxContentBytes: 3, MaxSemanticObjects: 2}},
		}
		if _, err := JoinContentSources(d, inputs); err == nil {
			t.Fatalf("invalid inputs accepted: %+v", inputs)
		}
		if len(d.Sources) != 1 {
			t.Fatal("failure registered a partial source")
		}
	}
}

func makeTestPDF(objects []string, trailer string) []byte {
	return pdftest.File(objects, " "+trailer)
}

func parseTestPDF(t *testing.T, data []byte) *Document {
	t.Helper()
	d, err := Read(bytes.NewReader(data), int64(len(data)))
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
			doc, err := Read(reader, int64(len(data)))
			if !errors.Is(err, test.wantErr) || (doc == nil) != (test.wantErr != nil) {
				t.Fatalf("result present=%t, error=%v; want error=%v", doc != nil, err, test.wantErr)
			}
			if reader.closed {
				t.Fatal("Read closed the caller-owned reader")
			}
		})
	}
}

func TestParseRejectsInvalidInputBeforeReading(t *testing.T) {
	if doc, err := Read(nil, 1); doc != nil || err == nil {
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
			if doc, err := Read(reader, test.size, test.options...); doc != nil || err == nil {
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
	doc, err := Read(bytes.NewReader(data), int64(len(data)))
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
	if _, err := Read(bytes.NewReader(b.Bytes()), int64(b.Len())); err == nil {
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
	if _, err := Read(strings.NewReader("%PDF"), -1); err == nil {
		t.Fatal("negative size accepted")
	}
	data := []byte("%PDF-1.7\nxref\n0 4294967295\ntrailer\n<< /Size 4294967295 >>\nstartxref\n9\n%%EOF\n")
	if _, err := Read(bytes.NewReader(data), int64(len(data))); err == nil {
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
		d, err := Read(bytes.NewReader(data), int64(len(data)), ReadOptions{MaxFileBytes: 65536, Limits: pdfmodel.Limits{MaxDepth: 32, MaxObjects: 128, MaxXRefSections: 16, MaxDecodedBytes: 65536}})
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
	d, err := Read(bytes.NewReader(data), int64(len(data)), ReadOptions{Limits: pdfmodel.Limits{MaxObjects: 2}})
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
	d, err := Read(bytes.NewReader(file.Bytes()), int64(file.Len()), ReadOptions{Limits: pdfmodel.Limits{MaxObjects: 8}})
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

func xrefStreamFile(objects map[int]string, compressed map[int][2]int, last int) []byte {
	var file bytes.Buffer
	file.WriteString("%PDF-1.7\n")
	offsets := make(map[int]int)
	for number := 1; number < last; number++ {
		if body, ok := objects[number]; ok {
			offsets[number] = file.Len()
			fmt.Fprintf(&file, "%d 0 obj\n%s\nendobj\n", number, body)
		}
	}
	offsets[last] = file.Len()
	var records bytes.Buffer
	for number := 0; number <= last; number++ {
		kind, offset, generation := byte(1), uint32(offsets[number]), uint16(0)
		if number == 0 {
			kind, generation = 0, 65535
		}
		if pair, ok := compressed[number]; ok {
			kind, offset, generation = 2, uint32(pair[0]), uint16(pair[1])
		}
		records.WriteByte(kind)
		_ = binary.Write(&records, binary.BigEndian, offset)
		_ = binary.Write(&records, binary.BigEndian, generation)
	}
	fmt.Fprintf(&file, "%d 0 obj\n<< /Type /XRef /Size %d /Root 1 0 R /W [1 4 2] /Length %d >>\nstream\n%s\nendstream\nendobj\nstartxref\n%d\n%%%%EOF\n", last, last+1, records.Len(), records.String(), offsets[last])
	return file.Bytes()
}

func TestXRefConfiguredLimitsAreClassifiable(t *testing.T) {
	stream := xrefStreamFile(map[int]string{1: "<< /Type /Catalog >>"}, nil, 2)
	hybrid := bytes.NewBuffer(bytes.Clone(stream))
	tableOffset := hybrid.Len()
	streamOffset := bytes.Index(stream, []byte("2 0 obj\n"))
	fmt.Fprintf(hybrid, "xref\n0 1\n0000000000 65535 f \ntrailer\n<< /Size 3 /Root 1 0 R /XRefStm %d >>\nstartxref\n%d\n%%%%EOF\n", streamOffset, tableOffset)
	for _, test := range []struct {
		name   string
		data   []byte
		limits pdfmodel.Limits
	}{
		{"table records", makeTestPDF([]string{"<< /Type /Catalog >>"}, ""), pdfmodel.Limits{MaxObjects: 1}},
		{"stream records", stream, pdfmodel.Limits{MaxObjects: 2}},
		{"hybrid sections", hybrid.Bytes(), pdfmodel.Limits{MaxXRefSections: 1}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := Read(bytes.NewReader(test.data), int64(len(test.data))); err != nil {
				t.Fatalf("fixture without limit: %v", err)
			}
			_, err := Read(bytes.NewReader(test.data), int64(len(test.data)), ReadOptions{Limits: test.limits})
			if !errors.Is(err, pdfmodel.ErrLimit) {
				t.Fatalf("configured xref budget error = %v", err)
			}
		})
	}
}

func TestMalformedXRefIsNotAConfiguredLimit(t *testing.T) {
	stream := xrefStreamFile(map[int]string{1: "<< /Type /Catalog >>"}, nil, 2)
	stream = bytes.Replace(stream, []byte("/Size 3"), []byte("/Size 2 /Index [0 3]"), 1)
	for _, data := range [][]byte{
		[]byte("%PDF-1.7\nxref\n4294967295 2\n0000000000 65535 f \n0000000000 65535 f \ntrailer\n<< /Size 1 >>\nstartxref\n9\n%%EOF\n"),
		[]byte("%PDF-1.7\nxref\n0 1\n0000000000 65535 f \ntrailer\n<< /Size 1 /XRefStm 9 >>\nstartxref\n9\n%%EOF\n"),
		stream,
	} {
		_, err := Read(bytes.NewReader(data), int64(len(data)))
		if err == nil || errors.Is(err, pdfmodel.ErrLimit) {
			t.Fatalf("malformed xref error = %v", err)
		}
	}
}

type countingSourceReader struct {
	io.ReaderAt
	bytes int
}

func (r *countingSourceReader) ReadAt(data []byte, offset int64) (int, error) {
	r.bytes += len(data)
	return r.ReaderAt.ReadAt(data, offset)
}

func TestCompressedObjectsReadHeaderOnceAndSelectedRanges(t *testing.T) {
	const payload = "3 0 4 3 5 6 10 20 30"
	data := xrefStreamFile(map[int]string{
		1: "<< /Type /Catalog >>",
		2: fmt.Sprintf("<< /Type /ObjStm /N 3 /First 12 /Length %d >>\nstream\n%s\nendstream", len(payload), payload),
	}, map[int][2]int{3: {2, 0}, 4: {2, 1}, 5: {2, 2}}, 6)
	doc := parseTestPDF(t, data)
	container, err := doc.Load(pdfmodel.ObjectID{Number: 2})
	if err != nil {
		t.Fatal(err)
	}
	source, err := doc.DecodeStream(container.Body.Value.(pdfmodel.Stream))
	if err != nil {
		t.Fatal(err)
	}
	counter := &countingSourceReader{ReaderAt: source.Reader}
	source.Reader = counter
	doc.Sources[source.ID] = source
	for _, number := range []uint32{5, 3, 4} {
		object, err := doc.Load(pdfmodel.ObjectID{Number: number})
		if err != nil {
			t.Fatal(err)
		}
		value, err := pdfmodel.Int(object.Body)
		if err != nil || value != int64(number-2)*10 {
			t.Fatalf("object %d = %d, %v", number, value, err)
		}
		if object.Body.Span.Source != source.ID {
			t.Fatal("lost decoded source provenance")
		}
	}
	if counter.bytes > len(payload) {
		t.Fatalf("reread %d bytes for %d-byte container", counter.bytes, len(payload))
	}
}

func TestRegionUpdatesDoNotReallocateForEveryObject(t *testing.T) {
	doc := regionTestDocument(256)
	reallocations := 0
	for offset := int64(0); offset < 256; offset += 2 {
		// Count replacements of the region backing array itself. Total heap
		// allocations also include compiler/race instrumentation overhead.
		previous := &doc.Structure.Regions[0]
		if err := doc.markRegion(pdfmodel.RegionIndirectObject, pdfmodel.Span{Source: 1, Start: offset, End: offset + 1}); err != nil {
			t.Fatal(err)
		}
		if &doc.Structure.Regions[0] != previous {
			reallocations++
		}
	}
	if len(doc.Structure.Regions) != 256 {
		t.Fatal("incomplete partition")
	}
	if reallocations > 32 {
		t.Fatalf("128 ordered object regions replaced the backing array %d times", reallocations)
	}
}

func TestRegionUpdatesPreservePartition(t *testing.T) {
	doc := regionTestDocument(20)
	doc.markRegion(pdfmodel.RegionIndirectObject, pdfmodel.Span{Source: 1, Start: 10, End: 15})
	doc.markRegion(pdfmodel.RegionHeader, pdfmodel.Span{Source: 1, End: 5})
	doc.markRegion(pdfmodel.RegionXRef, pdfmodel.Span{Source: 1, Start: 3, End: 12})
	want := []pdfmodel.FileRegion{
		{Kind: pdfmodel.RegionHeader, Span: pdfmodel.Span{Source: 1, End: 3}},
		{Kind: pdfmodel.RegionXRef, Span: pdfmodel.Span{Source: 1, Start: 3, End: 12}},
		{Kind: pdfmodel.RegionIndirectObject, Span: pdfmodel.Span{Source: 1, Start: 12, End: 15}},
		{Kind: pdfmodel.RegionUnknown, Span: pdfmodel.Span{Source: 1, Start: 15, End: 20}},
	}
	if len(doc.Structure.Regions) != len(want) {
		t.Fatalf("partition=%+v", doc.Structure.Regions)
	}
	for i, region := range want {
		if doc.Structure.Regions[i] != region {
			t.Fatalf("partition=%+v", doc.Structure.Regions)
		}
	}
}

func regionTestDocument(size int64) *Document {
	options, _ := normalizeOptions(nil)
	return &Document{Options: options, Structure: pdfmodel.Structure{Regions: []pdfmodel.FileRegion{{Kind: pdfmodel.RegionUnknown, Span: pdfmodel.Span{Source: 1, End: size}}}}}
}

func TestRegionWorkBudgetStopsReverseObjectLoads(t *testing.T) {
	objects := make([]string, 32)
	for i := range objects {
		objects[i] = "42"
	}
	data := makeTestPDF(objects, "")
	doc := parseTestPDF(t, data)
	doc.Options.Limits.MaxRegionWork = 100
	var loadErr error
	for number := len(objects); number > 0; number-- {
		before := len(doc.Structure.Objects)
		_, loadErr = doc.Load(pdfmodel.ObjectID{Number: uint32(number)})
		if loadErr != nil {
			if len(doc.Structure.Objects) != before {
				t.Fatal("failed region update registered an object")
			}
			if _, ok := doc.cache[pdfmodel.ObjectID{Number: uint32(number)}]; ok {
				t.Fatal("failed region update cached an object")
			}
			break
		}
	}
	if loadErr == nil {
		t.Fatal("reverse object loads ignored region work budget")
	}
	if !errors.Is(loadErr, pdfmodel.ErrLimit) {
		t.Fatalf("region work error does not identify ErrLimit: %v", loadErr)
	}
	var end int64
	for _, region := range doc.Structure.Regions {
		if region.Span.Start != end || region.Span.End <= end {
			t.Fatalf("corrupt partial partition: %+v", doc.Structure.Regions)
		}
		end = region.Span.End
	}
	if end != int64(len(data)) {
		t.Fatal("partial document lost file coverage")
	}
}

func BenchmarkLoadObjectRegions(b *testing.B) {
	for _, count := range []int{100, 500} {
		objects := make([]string, count)
		for i := range objects {
			objects[i] = "42"
		}
		data := makeTestPDF(objects, "")
		for _, reverse := range []bool{false, true} {
			b.Run(fmt.Sprintf("objects=%d/reverse=%v", count, reverse), func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					doc, err := Read(bytes.NewReader(data), int64(len(data)))
					if err != nil {
						b.Fatal(err)
					}
					for i := range count {
						number := i + 1
						if reverse {
							number = count - i
						}
						if _, err := doc.Load(pdfmodel.ObjectID{Number: uint32(number)}); err != nil {
							b.Fatal(err)
						}
					}
				}
			})
		}
	}
}

func TestFileBudgetErrorIsClassifiable(t *testing.T) {
	r := &controlledReaderAt{}
	_, err := Read(r, 2, ReadOptions{MaxFileBytes: 1})
	if !errors.Is(err, pdfmodel.ErrLimit) || r.calls != 0 {
		t.Fatalf("expected classified limit before reading, got %v, calls=%d", err, r.calls)
	}
}

func TestParseRejectsNegativeResourceLimitsBeforeReading(t *testing.T) {
	for _, limits := range []pdfmodel.Limits{
		{MaxValues: -1}, {MaxContentBytes: -1}, {MaxSemanticObjects: -1}, {MaxRegionWork: -1},
	} {
		r := &controlledReaderAt{}
		if _, err := Read(r, 1, ReadOptions{Limits: limits}); err == nil || r.calls != 0 {
			t.Fatalf("limits=%+v err=%v reader calls=%d", limits, err, r.calls)
		}
	}
}

func TestDocumentCumulativeDirectValueBudget(t *testing.T) {
	data := makeTestPDF([]string{"[1 2]", "[3 4]", "[5 6]"}, "")
	d, err := Read(bytes.NewReader(data), int64(len(data)), ReadOptions{Limits: pdfmodel.Limits{MaxValues: 9}})
	if err != nil {
		t.Fatal(err)
	}
	// The trailer is three values, then each array is another three.
	for _, n := range []uint32{1, 2} {
		if _, err := d.Load(pdfmodel.ObjectID{Number: n}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := d.Load(pdfmodel.ObjectID{Number: 1}); err != nil {
		t.Fatal("cache hit consumed budget:", err)
	}
	if _, err := d.Load(pdfmodel.ObjectID{Number: 3}); err == nil {
		t.Fatal("separate objects bypassed cumulative direct value budget")
	}
}

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
					doc, err := Read(bytes.NewReader(data), int64(len(data)))
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
