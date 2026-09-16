package structure

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strconv"

	"github.com/MyungSub0519/gopd/internal/model"
)

// Document owns an immutable input snapshot and lazily decoded sources.
// Treat returned objects/slices as read-only. Lazy methods are not concurrent-safe.
type Document struct {
	Structure    model.Structure
	Sources      map[model.SourceID]model.Source
	Options      ReadOptions
	Encrypted    bool
	data         []byte
	entries      map[uint32]model.XRefRecord
	cache        map[model.ObjectID]*model.IndirectObject
	physical     map[int64]*model.IndirectObject
	loading      map[model.ObjectID]bool
	decoded      map[model.Span]model.SourceID
	decodedBytes int64
	xrefRecords  int
	xrefRanges   int
	trailer      model.Object
}

// Bytes returns a copy of the requested source range. Offsets address either
// the original file or a decoded source according to span.Source.
func (d *Document) Bytes(span model.Span) ([]byte, error) {
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
func (d *Document) Catalog() (model.Object, error) {
	dict, ok := d.trailer.Value.(model.Dictionary)
	if !ok {
		return model.Object{}, errors.New("missing trailer dictionary")
	}
	root, err := dict.Get("Root")
	if err != nil {
		return model.Object{}, err
	}
	return d.ResolveObject(root)
}

// Resolve loads one indirect reference and returns its object body.
func (d *Document) Resolve(ref model.Reference) (model.Object, error) {
	obj, err := d.Load(ref.ID)
	if err != nil {
		return model.Object{}, err
	}
	return obj.Body, nil
}

// ResolveObject follows indirect reference chains, rejecting cycles and depth
// limit violations. A direct object is returned unchanged.
func (d *Document) ResolveObject(object model.Object) (model.Object, error) {
	seen := make(map[model.ObjectID]bool)
	for depth := 0; depth < d.Options.Limits.MaxDepth; depth++ {
		ref, ok := object.Value.(model.Reference)
		if !ok {
			return object, nil
		}
		if seen[ref.ID] {
			return model.Object{}, fmt.Errorf("cyclic indirect reference %v", ref.ID)
		}
		seen[ref.ID] = true
		var err error
		object, err = d.Resolve(ref)
		if err != nil {
			return model.Object{}, err
		}
	}
	return model.Object{}, errors.New("indirect reference depth limit exceeded")
}

func (d *Document) dictInt(dict model.Dictionary, key model.Name) (int64, error) {
	o, err := dict.Get(key)
	if err != nil {
		return 0, err
	}
	o, err = d.ResolveObject(o)
	if err != nil {
		return 0, err
	}
	return model.Int(o)
}

func (d *Document) markRegion(kind model.RegionKind, span model.Span) {
	if span.Source != 1 || span.Start >= span.End {
		return
	}
	var regions []model.FileRegion
	for _, r := range d.Structure.Regions {
		if r.Span.End <= span.Start || r.Span.Start >= span.End {
			regions = append(regions, r)
			continue
		}
		lo, hi := max(r.Span.Start, span.Start), min(r.Span.End, span.End)
		if r.Span.Start < lo {
			regions = append(regions, model.FileRegion{Kind: r.Kind, Span: model.Span{Source: 1, Start: r.Span.Start, End: lo}})
		}
		regions = append(regions, model.FileRegion{Kind: kind, Span: model.Span{Source: 1, Start: lo, End: hi}})
		if hi < r.Span.End {
			regions = append(regions, model.FileRegion{Kind: r.Kind, Span: model.Span{Source: 1, Start: hi, End: r.Span.End}})
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

// RawObject returns the original syntax of an object, including its delimiters.
func (d *Document) RawObject(object model.Object) ([]byte, error) { return d.Bytes(object.Span) }
