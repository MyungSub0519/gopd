package document

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"testing"

	"github.com/MyungSub0519/gopd/internal/pdfmodel"
)

// sparsePDF supplies an original-file address space without allocating it.
type sparsePDF struct {
	size, tailAt int64
	head, tail   []byte
	readBytes    int64
	maxRead      int
}

func (r *sparsePDF) ReadAt(p []byte, off int64) (int, error) {
	r.maxRead = max(r.maxRead, len(p))
	if off < 0 || off >= r.size {
		return 0, io.EOF
	}
	n := int(min(int64(len(p)), r.size-off))
	clear(p[:n])
	for _, part := range []struct {
		at   int64
		data []byte
	}{{0, r.head}, {r.tailAt, r.tail}} {
		start, end := max(off, part.at), min(off+int64(n), part.at+int64(len(part.data)))
		if start < end {
			copy(p[start-off:end-off], part.data[start-part.at:end-part.at])
		}
	}
	r.readBytes += int64(n)
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

// ISO 32000-1:2008, 7.3.8, 7.5.4: /Length and xref offsets allow
// skipping encoded data until the caller actually requests that stream.
func TestBufferedReadSkipsLargeStreamPayload(t *testing.T) {
	const fileSize = int64(500 << 20)
	const tailAt = fileSize - 512
	prefix := []byte("%PDF-1.7\n1 0 obj\n<< /Type /Catalog >>\nendobj\n")
	streamOffset := len(prefix)
	streamHeader := []byte("2 0 obj\n<< /Length 0000000000 >>\nstream\n")
	payloadStart := len(prefix) + len(streamHeader)
	payloadSize := tailAt - int64(payloadStart)
	streamHeader = []byte(fmt.Sprintf("2 0 obj\n<< /Length %010d >>\nstream\n", payloadSize))
	head := append(prefix, streamHeader...)
	endStream := "\nendstream\nendobj\n"
	xref := tailAt + int64(len(endStream))
	tail := []byte(fmt.Sprintf("%sxref\n0 3\n0000000000 65535 f \n0000000009 00000 n \n%010d 00000 n \ntrailer\n<< /Size 3 /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", endStream, streamOffset, xref))
	for _, size := range []int{1, 7, 64 << 10, 256 << 10} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			r := &sparsePDF{size: fileSize, tailAt: tailAt, head: head, tail: tail}
			d, err := Read(r, fileSize, ReadOptions{MaxFileBytes: 1 << 30, BufferSize: size})
			if err != nil {
				t.Fatal(err)
			}
			defer d.Close()
			obj, err := d.Load(pdfmodel.ObjectID{Number: 2})
			if err != nil {
				t.Fatal(err)
			}
			stream := obj.Body.Value.(pdfmodel.Stream)
			if stream.Encoded.Start != int64(payloadStart) || stream.Encoded.End != tailAt {
				t.Fatalf("encoded=%+v", stream.Encoded)
			}
			if r.maxRead > size || r.readBytes > 2<<20 {
				t.Fatalf("buffer=%d max read=%d total bytes=%d", size, r.maxRead, r.readBytes)
			}
			t.Logf("500 MiB input: buffer=%d, largest backing read=%d, total read=%d", size, r.maxRead, r.readBytes)
		})
	}
}

func TestBufferedReadObjectBoundaries(t *testing.T) {
	payload := "abc\x00endstream\xff"
	data := makeTestPDF([]string{"<< /Type /Catalog /Value [(escaped\\)string) <0102> /Name#20Here 4 0 R] >>", "<< /Length 3 0 R >>\nstream\n" + payload + "\nendstream", fmt.Sprint(len(payload)), "[true false null 1.25]"}, "")
	for _, size := range []int{1, 2, 3, 7, 16, 64 << 10} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			d, err := Read(bytes.NewReader(data), int64(len(data)), ReadOptions{BufferSize: size})
			if err != nil {
				t.Fatal(err)
			}
			defer d.Close()
			if _, err := d.Catalog(); err != nil {
				t.Fatal(err)
			}
			obj, err := d.Load(pdfmodel.ObjectID{Number: 2})
			if err != nil {
				t.Fatal(err)
			}
			stream := obj.Body.Value.(pdfmodel.Stream)
			got, err := d.Bytes(*stream.Encoded)
			if err != nil || string(got) != payload {
				t.Fatalf("payload=%q err=%v", got, err)
			}
		})
	}
}

func TestBufferOptionValidation(t *testing.T) {
	r := &controlledReaderAt{}
	if _, err := Read(r, 1, ReadOptions{BufferSize: -1}); err == nil || r.calls != 0 {
		t.Fatal("negative buffer reached input")
	}
	o, err := normalizeOptions(nil)
	if err != nil || o.BufferSize != 64<<10 {
		t.Fatalf("default=%d err=%v", o.BufferSize, err)
	}
}

func TestCloseInvalidatesRetainedSources(t *testing.T) {
	data := makeTestPDF([]string{"<< /Type /Catalog >>", "<< /Length 3 >>\nstream\nabc\nendstream"}, "")
	d := parseTestPDF(t, data)
	obj, err := d.Load(pdfmodel.ObjectID{Number: 2})
	if err != nil {
		t.Fatal(err)
	}
	stream := obj.Body.Value.(pdfmodel.Stream)
	decoded, err := d.DecodeStream(stream)
	if err != nil {
		t.Fatal(err)
	}
	joined, err := JoinContentSources(d, []pdfmodel.Span{{Source: decoded.ID, End: 3}, {Source: decoded.ID, End: 3}})
	if err != nil {
		t.Fatal(err)
	}
	sources := []pdfmodel.Source{d.Sources[1], decoded, joined}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	for _, source := range sources {
		var b [1]byte
		if _, err := source.Reader.ReadAt(b[:], 0); !errors.Is(err, os.ErrClosed) {
			t.Fatalf("source %d after close: %v", source.ID, err)
		}
	}
	if _, err := d.DecodeStream(stream); !errors.Is(err, os.ErrClosed) {
		t.Fatalf("cached decode after close: %v", err)
	}
	if _, err := d.Load(pdfmodel.ObjectID{Number: 2}); !errors.Is(err, os.ErrClosed) {
		t.Fatalf("cached object after close: %v", err)
	}
}

func TestBufferedReadLargeOffsets(t *testing.T) {
	// Original-file offsets are int64 even on 32-bit builds. Tail overlap
	// arithmetic must not overflow at the ReaderAt address-space boundary.
	for _, size := range []int64{3 << 30, math.MaxInt64} {
		r := &sparsePDF{
			size: size, tailAt: size - 256,
			head: []byte("%PDF-1.7\n1 0 obj\n<< /Type /Catalog >>\nendobj\n"),
		}
		r.tail = []byte(fmt.Sprintf("xref\n0 2\n0000000000 65535 f \n0000000009 00000 n \ntrailer\n<< /Size 2 /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", r.tailAt))
		d, err := Read(r, size, ReadOptions{MaxFileBytes: math.MaxInt64, BufferSize: 64})
		if d != nil {
			defer d.Close()
		}
		if err != nil {
			t.Fatalf("size %d: %v", size, err)
		}
		if _, err := d.Catalog(); err != nil {
			t.Fatal(err)
		}
		if d.Structure.XRefs[0].Offset != r.tailAt {
			t.Fatal("xref offset truncated")
		}
	}
}

func TestBytesAddressableAllocationLimit(t *testing.T) {
	if strconv.IntSize != 32 {
		t.Skip("int64 source ranges exceed addressable slices on 32-bit builds")
	}
	r := &controlledReaderAt{}
	d := &Document{Sources: map[pdfmodel.SourceID]pdfmodel.Source{1: {ID: 1, Reader: r, Size: 3 << 30}}}
	if _, err := d.Bytes(pdfmodel.Span{Source: 1, End: 3 << 30}); !errors.Is(err, pdfmodel.ErrLimit) {
		t.Fatalf("addressable range limit: %v", err)
	}
	if r.calls != 0 {
		t.Fatal("allocation limit reached input")
	}
}

func TestUnfilteredStreamLimitBeforePayloadRead(t *testing.T) {
	data := makeTestPDF([]string{"<< /Type /Catalog >>", "<< /Length 6 >>\nstream\nabcdef\nendstream"}, "")
	r := &controlledReaderAt{data: data}
	d, err := Read(r, int64(len(data)), ReadOptions{BufferSize: 8, Limits: pdfmodel.Limits{MaxDecodedBytes: 3}})
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	obj, err := d.Load(pdfmodel.ObjectID{Number: 2})
	if err != nil {
		t.Fatal(err)
	}
	// A known unfiltered length is already the decoded length. Reject it
	// before allocating or requesting encoded data that cannot fit its budget.
	r.err = errors.New("unexpected payload read")
	d.input.start = -1
	reads := r.calls
	if _, err := d.DecodeStream(obj.Body.Value.(pdfmodel.Stream)); !errors.Is(err, pdfmodel.ErrLimit) {
		t.Fatalf("decoded budget: %v", err)
	}
	if r.calls != reads {
		t.Fatal("read payload before rejecting decoded size")
	}
}
