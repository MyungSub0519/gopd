package document

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"

	"github.com/MyungSub0519/gopd/internal/pdfmodel"
)

// Document owns an immutable input snapshot and lazily decoded sources.
// Treat returned objects/slices as read-only. Lazy methods are not concurrent-safe.
type Document struct {
	Structure     pdfmodel.Structure
	Sources       map[pdfmodel.SourceID]pdfmodel.Source
	Options       ReadOptions
	Encrypted     bool
	data          []byte
	entries       map[uint32]pdfmodel.XRefRecord
	cache         map[pdfmodel.ObjectID]*pdfmodel.IndirectObject
	physical      map[int64]*pdfmodel.IndirectObject
	loading       map[pdfmodel.ObjectID]bool
	decoded       map[pdfmodel.Span]pdfmodel.SourceID
	decodedBytes  int64
	parsedValues  int
	regionWork    int64
	objectStreams map[pdfmodel.ObjectID]*objectStreamIndex
	xrefRecords   int
	xrefRanges    int
	trailer       pdfmodel.Object
}

// Bytes returns a copy of the requested source range. Offsets address either
// the original file or a decoded source according to span.Source.
func (d *Document) Bytes(span pdfmodel.Span) ([]byte, error) {
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

// Catalog resolves the document catalog referenced by the effective trailer.
func (d *Document) Catalog() (pdfmodel.Object, error) {
	dict, ok := d.trailer.Value.(pdfmodel.Dictionary)
	if !ok {
		return pdfmodel.Object{}, errors.New("missing trailer dictionary")
	}
	root, err := dict.Get("Root")
	if err != nil {
		return pdfmodel.Object{}, err
	}
	return d.ResolveObject(root)
}

// Resolve loads one indirect reference and returns its object body.
func (d *Document) Resolve(ref pdfmodel.Reference) (pdfmodel.Object, error) {
	obj, err := d.Load(ref.ID)
	if err != nil {
		return pdfmodel.Object{}, err
	}
	return obj.Body, nil
}

// ResolveObject follows indirect reference chains, rejecting cycles and depth
// limit violations. A direct object is returned unchanged.
func (d *Document) ResolveObject(object pdfmodel.Object) (pdfmodel.Object, error) {
	seen := make(map[pdfmodel.ObjectID]bool)
	for depth := 0; depth < d.Options.Limits.MaxDepth; depth++ {
		ref, ok := object.Value.(pdfmodel.Reference)
		if !ok {
			return object, nil
		}
		if seen[ref.ID] {
			return pdfmodel.Object{}, fmt.Errorf("cyclic indirect reference %v", ref.ID)
		}
		seen[ref.ID] = true
		var err error
		object, err = d.Resolve(ref)
		if err != nil {
			return pdfmodel.Object{}, err
		}
	}
	return pdfmodel.Object{}, fmt.Errorf("indirect reference depth limit exceeded: %w", pdfmodel.ErrLimit)
}

func (d *Document) dictInt(dict pdfmodel.Dictionary, key pdfmodel.Name) (int64, error) {
	o, err := dict.Get(key)
	if err != nil {
		return 0, err
	}
	o, err = d.ResolveObject(o)
	if err != nil {
		return 0, err
	}
	return pdfmodel.Int(o)
}

func (d *Document) markRegion(kind pdfmodel.RegionKind, span pdfmodel.Span) error {
	if span.Source != 1 || span.Start >= span.End {
		return nil
	}
	regions := d.Structure.Regions
	// Two binary searches plus the replaced interval and moved suffix bound
	// work even when callers load physical objects in reverse order.
	var comparisons int64
	first := sort.Search(len(regions), func(i int) bool {
		comparisons++
		return regions[i].Span.End > span.Start
	})
	last := sort.Search(len(regions), func(i int) bool {
		comparisons++
		return regions[i].Span.Start >= span.End
	})
	if first >= last {
		return nil
	}
	left, right := regions[first], regions[last-1]
	span.Start = max(span.Start, left.Span.Start)
	span.End = min(span.End, right.Span.End)
	var replacement [3]pdfmodel.FileRegion
	count := 0
	if left.Span.Start < span.Start {
		left.Span.End = span.Start
		replacement[count] = left
		count++
	}
	replacement[count] = pdfmodel.FileRegion{Kind: kind, Span: span}
	count++
	if right.Span.End > span.End {
		right.Span.Start = span.End
		replacement[count] = right
		count++
	}
	change := count - (last - first)
	work := comparisons + int64(last-first)
	if change != 0 {
		work += int64(len(regions) - last)
	}
	// Growing the backing array also copies its existing contents.
	if len(regions)+change > cap(regions) {
		work += int64(len(regions))
	}
	if work > d.Options.Limits.MaxRegionWork-d.regionWork {
		return fmt.Errorf("file region work limit exceeded: %w", pdfmodel.ErrLimit)
	}
	d.regionWork += work
	if change > 0 {
		regions = append(regions, make([]pdfmodel.FileRegion, change)...)
	}
	if change != 0 {
		copy(regions[first+count:], regions[last:len(d.Structure.Regions)])
	}
	copy(regions[first:first+count], replacement[:count])
	if change < 0 {
		regions = regions[:len(regions)+change]
	}
	d.Structure.Regions = regions
	return nil
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

// RawObject returns the original syntax of an object, including its delimiters.
func (d *Document) RawObject(object pdfmodel.Object) ([]byte, error) { return d.Bytes(object.Span) }
