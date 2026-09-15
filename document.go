package gopd

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
)

// ReadOptions bounds input, recursive parsing, xrefs and decoded stream data.
// Zero fields use defaults. Negative values are invalid.
type ReadOptions struct {
	MaxFileBytes int64
	Limits       Limits
}

// Document owns an immutable input snapshot and lazily decoded sources.
// Treat returned objects/slices as read-only. Lazy methods are not concurrent-safe.
type Document struct {
	Structure    Structure
	Sources      map[SourceID]Source
	Options      ReadOptions
	Encrypted    bool
	data         []byte
	entries      map[uint32]XRefRecord
	cache        map[ObjectID]*IndirectObject
	physical     map[int64]*IndirectObject
	loading      map[ObjectID]bool
	decoded      map[Span]SourceID
	decodedBytes int64
	xrefRecords  int
	xrefRanges   int
	trailer      Object
}

func normalizeOptions(options []ReadOptions) (ReadOptions, error) {
	if len(options) > 1 {
		return ReadOptions{}, errors.New("at most one ReadOptions value is accepted")
	}
	var o ReadOptions
	if len(options) == 1 {
		o = options[0]
	}
	if o.MaxFileBytes < 0 || o.Limits.MaxDepth < 0 || o.Limits.MaxTokenBytes < 0 || o.Limits.MaxObjects < 0 || o.Limits.MaxXRefSections < 0 || o.Limits.MaxDecodedBytes < 0 {
		return o, errors.New("negative PDF read limit")
	}
	if o.MaxFileBytes == 0 {
		o.MaxFileBytes = 256 << 20
	}
	if o.Limits.MaxDepth == 0 {
		o.Limits.MaxDepth = 256
	}
	if o.Limits.MaxTokenBytes == 0 {
		o.Limits.MaxTokenBytes = 16 << 20
	}
	if o.Limits.MaxObjects == 0 {
		o.Limits.MaxObjects = 1_000_000
	}
	if o.Limits.MaxXRefSections == 0 {
		o.Limits.MaxXRefSections = 256
	}
	if o.Limits.MaxDecodedBytes == 0 {
		o.Limits.MaxDecodedBytes = 256 << 20
	}
	return o, nil
}

// Parse snapshots r; it never closes a caller-owned ReaderAt.
func Parse(r io.ReaderAt, size int64, options ...ReadOptions) (*Document, error) {
	o, err := normalizeOptions(options)
	if err != nil {
		return nil, err
	}
	if r == nil || size <= 0 || size > o.MaxFileBytes || uint64(size) > uint64(^uint(0)>>1) {
		return nil, fmt.Errorf("invalid or over-limit PDF size %d", size)
	}
	data := make([]byte, int(size))
	n, err := r.ReadAt(data, 0)
	if n != len(data) {
		if err == nil {
			err = io.ErrUnexpectedEOF
		}
		return nil, fmt.Errorf("read PDF snapshot: %w", err)
	}
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	d := &Document{data: data, Options: o, Sources: make(map[SourceID]Source), entries: make(map[uint32]XRefRecord), cache: make(map[ObjectID]*IndirectObject), physical: make(map[int64]*IndirectObject), loading: make(map[ObjectID]bool), decoded: make(map[Span]SourceID)}
	d.Sources[1] = Source{ID: 1, Reader: bytes.NewReader(data), Size: size}
	d.Structure.File = 1
	d.Structure.Regions = []FileRegion{{Kind: RegionUnknown, Span: Span{Source: 1, End: size}}}
	if err = d.readHeaderAndXRefs(); err != nil {
		return d, err
	}
	return d, nil
}

func ParseFile(path string, options ...ReadOptions) (*Document, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		return nil, err
	}
	return Parse(f, stat.Size(), options...)
}

func (d *Document) Bytes(span Span) ([]byte, error) {
	source, ok := d.Sources[span.Source]
	if !ok || span.Start < 0 || span.End < span.Start || span.End > source.Size || uint64(span.End-span.Start) > uint64(^uint(0)>>1) {
		return nil, fmt.Errorf("invalid source span %+v", span)
	}
	data := make([]byte, int(span.End-span.Start))
	if len(data) == 0 {
		return data, nil
	}
	n, err := source.Reader.ReadAt(data, span.Start)
	if n != len(data) {
		if err == nil {
			err = io.ErrUnexpectedEOF
		}
		return nil, err
	}
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	return data, nil
}

func (d *Document) Catalog() (Object, error) {
	dict, ok := d.trailer.Value.(Dictionary)
	if !ok {
		return Object{}, errors.New("missing trailer dictionary")
	}
	root, err := dict.Get("Root")
	if err != nil {
		return Object{}, err
	}
	return d.ResolveObject(root)
}

func (d *Document) Resolve(ref Reference) (Object, error) {
	obj, err := d.Load(ref.ID)
	if err != nil {
		return Object{}, err
	}
	return obj.Body, nil
}

func (d *Document) ResolveObject(object Object) (Object, error) {
	seen := make(map[ObjectID]bool)
	for depth := 0; depth < d.Options.Limits.MaxDepth; depth++ {
		ref, ok := object.Value.(Reference)
		if !ok {
			return object, nil
		}
		if seen[ref.ID] {
			return Object{}, fmt.Errorf("cyclic indirect reference %v", ref.ID)
		}
		seen[ref.ID] = true
		var err error
		object, err = d.Resolve(ref)
		if err != nil {
			return Object{}, err
		}
	}
	return Object{}, errors.New("indirect reference depth limit exceeded")
}

func (d *Document) dictInt(dict Dictionary, key Name) (int64, error) {
	o, err := dict.Get(key)
	if err != nil {
		return 0, err
	}
	o, err = d.ResolveObject(o)
	if err != nil {
		return 0, err
	}
	return Int(o)
}

func (d *Document) markRegion(kind RegionKind, span Span) {
	if span.Source != 1 || span.Start >= span.End {
		return
	}
	var regions []FileRegion
	for _, r := range d.Structure.Regions {
		if r.Span.End <= span.Start || r.Span.Start >= span.End {
			regions = append(regions, r)
			continue
		}
		lo, hi := max(r.Span.Start, span.Start), min(r.Span.End, span.End)
		if r.Span.Start < lo {
			regions = append(regions, FileRegion{Kind: r.Kind, Span: Span{Source: 1, Start: r.Span.Start, End: lo}})
		}
		regions = append(regions, FileRegion{Kind: kind, Span: Span{Source: 1, Start: lo, End: hi}})
		if hi < r.Span.End {
			regions = append(regions, FileRegion{Kind: r.Kind, Span: Span{Source: 1, Start: hi, End: r.Span.End}})
		}
	}
	d.Structure.Regions = regions
}

func docSpace(c byte) bool { return c == 0 || c == 9 || c == 10 || c == 12 || c == 13 || c == 32 }
func skipDocSpace(data []byte, pos int) int {
	for pos < len(data) {
		if docSpace(data[pos]) {
			pos++
			continue
		}
		if data[pos] == '%' {
			for pos < len(data) && data[pos] != '\r' && data[pos] != '\n' {
				pos++
			}
			continue
		}
		break
	}
	return pos
}
func docDelimiter(c byte) bool { return docSpace(c) || bytes.IndexByte([]byte("()<>[]{}/%"), c) >= 0 }
func docWord(data []byte, pos *int) (string, int, error) {
	*pos = skipDocSpace(data, *pos)
	start := *pos
	for *pos < len(data) && !docDelimiter(data[*pos]) {
		*pos++
	}
	if start == *pos {
		return "", start, fmt.Errorf("expected token at byte %d", start)
	}
	return string(data[start:*pos]), start, nil
}
func docUint(data []byte, pos *int, bits int) (uint64, int, error) {
	s, start, err := docWord(data, pos)
	if err != nil {
		return 0, start, err
	}
	n, err := strconv.ParseUint(s, 10, bits)
	if err != nil {
		return 0, start, fmt.Errorf("invalid unsigned integer at byte %d: %w", start, err)
	}
	return n, start, nil
}
