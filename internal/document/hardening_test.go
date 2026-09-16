package document

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/MyungSub0519/gopd/internal/pdfmodel"
)

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
			if _, err := Parse(bytes.NewReader(test.data), int64(len(test.data))); err != nil {
				t.Fatalf("fixture without limit: %v", err)
			}
			_, err := Parse(bytes.NewReader(test.data), int64(len(test.data)), ReadOptions{Limits: test.limits})
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
		_, err := Parse(bytes.NewReader(data), int64(len(data)))
		if err == nil || errors.Is(err, pdfmodel.ErrLimit) {
			t.Fatalf("malformed xref error = %v", err)
		}
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
					doc, err := Parse(bytes.NewReader(data), int64(len(data)))
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
