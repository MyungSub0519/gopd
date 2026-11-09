package document

import (
	"bytes"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/MyungSub0519/gopd/internal/pdfmodel"
	"github.com/MyungSub0519/gopd/internal/syntax"
)

type inputReadRecorder struct {
	reader  io.ReaderAt
	maxRead int
	calls   int
}

func (r *inputReadRecorder) ReadAt(p []byte, off int64) (int, error) {
	r.maxRead = max(r.maxRead, len(p))
	r.calls++
	return r.reader.ReadAt(p, off)
}

func TestBufferedInputBoundsReadsAndReusesBlock(t *testing.T) {
	r := &inputReadRecorder{reader: strings.NewReader("0123456789abcdef")}
	in := newBufferedInput(r, 16, 3)
	p := make([]byte, 11)
	if n, err := in.ReadAt(p, 2); n != 11 || err != nil || string(p) != "23456789abc" {
		t.Fatalf("ReadAt = %q, %d, %v", p, n, err)
	}
	before := r.calls
	if n, err := in.ReadAt(p[:1], 12); n != 1 || err != nil || string(p[:1]) != "c" {
		t.Fatalf("cached read = %q, %d, %v", p[:1], n, err)
	}
	if r.calls != before || r.maxRead > 3 {
		t.Fatalf("calls %d -> %d, max read %d", before, r.calls, r.maxRead)
	}
	if n, err := in.ReadAt(p, 14); n != 2 || !errors.Is(err, io.EOF) {
		t.Fatalf("EOF = %d, %v", n, err)
	}
}

type brokenInputReader struct {
	data []byte
	err  error
}

func (r brokenInputReader) ReadAt(p []byte, off int64) (int, error) {
	return copy(p, r.data[off:]), r.err
}

func TestBufferedInputReadErrors(t *testing.T) {
	sentinel := errors.New("input failure")
	for _, tc := range []struct {
		name      string
		data      []byte
		err, want error
	}{
		{"short nil", []byte("x"), nil, io.ErrUnexpectedEOF},
		{"premature EOF", []byte("x"), io.EOF, io.EOF},
		{"partial cause", []byte("x"), sentinel, sentinel},
		{"full cause", []byte("xxxx"), sentinel, sentinel},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := newBufferedInput(brokenInputReader{tc.data, tc.err}, 4, 4)
			_, err := in.ReadAt(make([]byte, 4), 0)
			if !errors.Is(err, tc.want) {
				t.Fatalf("error = %v; want %v", err, tc.want)
			}
		})
	}
}

func TestBufferedInputClosedAndInvalidOffsets(t *testing.T) {
	in := newBufferedInput(strings.NewReader("abc"), 3, 1)
	if _, err := in.ReadAt(make([]byte, 1), -1); err == nil {
		t.Fatal("negative offset accepted")
	}
	if n, err := in.ReadAt(nil, 0); n != 0 || err != nil {
		t.Fatalf("empty read = %d, %v", n, err)
	}
	in.closed = true
	if _, err := in.ReadAt(make([]byte, 1), 0); err == nil {
		t.Fatal("closed input is readable")
	}
}

func newCursorTestDocument(t *testing.T, data string, buffer int, limits pdfmodel.Limits) *Document {
	t.Helper()
	o, err := normalizeOptions([]ReadOptions{{Limits: limits}})
	if err != nil {
		t.Fatal(err)
	}
	return &Document{input: newBufferedInput(strings.NewReader(data), int64(len(data)), buffer), size: int64(len(data)), Options: o}
}

// ISO 32000-1:2008, 7.2 and 7.3: token boundaries, strings, containers,
// indirect references and source positions are independent of read boundaries.
func TestInputCursorObjectsAcrossBlocks(t *testing.T) {
	for _, raw := range []string{
		"%lead\r\n << /A [12 0 R (a\\(b(c)\\)d) <a b c> /A#20B true null] /B -1.25 >>",
		"[1 2 3]", "1 0 R", "+12 +0 R", "12 0 endobj", "12 (unterminated", "12 /broken#GG", "12 0 Rubbish", "/Name", "(a\\\r\nb)",
	} {
		for _, size := range []int{1, 2, 3, 7} {
			d := newCursorTestDocument(t, raw+" endobj", size, pdfmodel.Limits{})
			c := d.cursor(0)
			want, n, err := syntax.ParseObject([]byte(raw+" endobj"), 1, 0)
			if err != nil {
				t.Fatal(err)
			}
			got, err := c.object()
			if err != nil || !reflect.DeepEqual(got, want) || c.pos != int64(n) {
				t.Fatalf("buffer %d, %q: object=%+v position=%d error=%v; want %+v at %d", size, raw, got, c.pos, err, want, n)
			}
		}
	}
}

func TestInputCursorWordsAndIndependentPositions(t *testing.T) {
	d := newCursorTestDocument(t, "%comment\r\n 123/Name", 1, pdfmodel.Limits{})
	c := d.cursor(0)
	n, start, err := c.uint(32)
	if n != 123 || start != 11 || err != nil || c.pos != 14 {
		t.Fatalf("uint = %d at %d, position %d: %v", n, start, c.pos, err)
	}
	if yes, err := c.prefix("/Name"); !yes || err != nil || c.pos != 14 {
		t.Fatalf("prefix = %v, %v, position %d", yes, err, c.pos)
	}
	other := d.cursor(0)
	if _, err := other.take(); err != nil {
		t.Fatal(err)
	}
	if ch, err := c.peek(); ch != '/' || err != nil {
		t.Fatalf("peek = %q, %v", ch, err)
	}
}

func TestInputCursorLiteralSpans(t *testing.T) {
	d := newCursorTestDocument(t, "ignored % comment\n[ /A#20B (x\\(y\\)) 2 0 R ] endobj", 1, pdfmodel.Limits{})
	c := d.cursor(8)
	object, err := c.object()
	if err != nil {
		t.Fatal(err)
	}
	want := pdfmodel.Object{
		Span: pdfmodel.Span{Source: 1, Start: 18, End: 43},
		Value: pdfmodel.Array{Items: []pdfmodel.Object{
			{Span: pdfmodel.Span{Source: 1, Start: 20, End: 26}, Value: pdfmodel.Name("A B")},
			{Span: pdfmodel.Span{Source: 1, Start: 27, End: 35}, Value: pdfmodel.PDFString{Form: pdfmodel.StringLiteral, Bytes: []byte("x(y)")}},
			{Span: pdfmodel.Span{Source: 1, Start: 36, End: 41}, Value: pdfmodel.Reference{ID: pdfmodel.ObjectID{Number: 2}}},
		}},
	}
	if !reflect.DeepEqual(object, want) || c.pos != 43 {
		t.Fatalf("object=%+v, position=%d; want %+v", object, c.pos, want)
	}
}

func TestInputCursorPreservesIOErrors(t *testing.T) {
	sentinel := errors.New("input interrupted")
	d := newCursorTestDocument(t, "12 0 R", 1, pdfmodel.Limits{})
	d.input = newBufferedInput(inputErrorAt{data: []byte("12 0 R"), off: 3, err: sentinel}, d.size, 1)
	if _, err := d.cursor(0).object(); !errors.Is(err, sentinel) {
		t.Fatalf("speculative read error=%v", err)
	}
	d.input = newBufferedInput(strings.NewReader("12"), d.size, 1)
	if _, err := d.cursor(0).object(); !errors.Is(err, io.EOF) {
		t.Fatalf("short source error=%v", err)
	}
}

type inputErrorAt struct {
	data []byte
	off  int64
	err  error
}

func (r inputErrorAt) ReadAt(p []byte, off int64) (int, error) {
	if off >= r.off {
		return 0, r.err
	}
	return bytes.NewReader(r.data).ReadAt(p, off)
}

func FuzzInputCursorObject(f *testing.F) {
	for _, seed := range []string{"[1 2 3]", "<< /Key (a\\(b) /Ref 42 0 R >>", "[+42 +2 R <ac e> true]", "12 (unterminated", "12 0 Rubbish"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		if len(raw) > 4096 {
			t.Skip()
		}
		limits := pdfmodel.Limits{MaxDepth: 16, MaxValues: 64, MaxTokenBytes: 512}
		want, n, wantErr := syntax.ParseObjectWithLimits([]byte(raw), 1, 0, limits)
		d := newCursorTestDocument(t, raw, 3, limits)
		c := d.cursor(0)
		got, err := c.object()
		if wantErr == nil && (err != nil || c.pos != int64(n) || !reflect.DeepEqual(got, want)) {
			t.Fatalf("%q: got %+v at %d error %v; want %+v at %d", raw, got, c.pos, err, want, n)
		}
		if wantErr != nil && err == nil {
			t.Fatalf("%q: malformed object accepted: %+v", raw, got)
		}
		if d.parsedValues > limits.MaxValues {
			t.Fatalf("value budget exceeded: %d", d.parsedValues)
		}
	})
}

func TestInputCursorLimitsAndFailedBudget(t *testing.T) {
	for _, tc := range []struct {
		raw    string
		limits pdfmodel.Limits
	}{
		{"(12345)", pdfmodel.Limits{MaxTokenBytes: 4}},
		{"[[0]]", pdfmodel.Limits{MaxDepth: 1}},
		{"[0 1 2]", pdfmodel.Limits{MaxValues: 2}},
		{"%123456\n0", pdfmodel.Limits{MaxTokenBytes: 4}},
	} {
		d := newCursorTestDocument(t, tc.raw, 1, tc.limits)
		if _, err := d.cursor(0).object(); !errors.Is(err, pdfmodel.ErrLimit) {
			t.Fatalf("%q: error = %v", tc.raw, err)
		}
	}
	d := newCursorTestDocument(t, "[true bogus]", 1, pdfmodel.Limits{MaxValues: 3})
	if _, err := d.cursor(0).object(); err == nil {
		t.Fatal("malformed object accepted")
	}
	if d.parsedValues != 3 {
		t.Fatalf("failed parse charged %d values; want 3", d.parsedValues)
	}
	if _, err := d.cursor(1).object(); !errors.Is(err, pdfmodel.ErrLimit) {
		t.Fatalf("retry error = %v", err)
	}
}

func TestInputCursorStopsBeforeStreamPayload(t *testing.T) {
	raw := "<< /Length 500000 >>\nstream\n"
	r := &inputReadRecorder{reader: bytes.NewReader(append([]byte(raw), make([]byte, 500000)...))}
	d := newCursorTestDocument(t, raw, 3, pdfmodel.Limits{})
	d.size += 500000
	d.input = newBufferedInput(r, d.size, 3)
	c := d.cursor(0)
	if _, err := c.object(); err != nil {
		t.Fatal(err)
	}
	if c.pos != 20 || r.calls > 9 || r.maxRead > 3 {
		t.Fatalf("position=%d reads=%d max=%d", c.pos, r.calls, r.maxRead)
	}
}
