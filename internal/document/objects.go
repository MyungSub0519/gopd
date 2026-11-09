package document

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/MyungSub0519/gopd/internal/pdfmodel"
)

func (d *Document) Load(id pdfmodel.ObjectID) (*pdfmodel.IndirectObject, error) {
	if d.closed {
		return nil, os.ErrClosed
	}
	if id.Number == 0 {
		return nil, errors.New("object zero is reserved")
	}
	if obj, ok := d.cache[id]; ok {
		return obj, nil
	}
	if d.loading[id] {
		return nil, fmt.Errorf("cyclic object load %v", id)
	}
	if len(d.loading) >= d.Options.Limits.MaxDepth {
		return nil, fmt.Errorf("object load depth exceeded for %v: %w", id, pdfmodel.ErrLimit)
	}
	record, ok := d.entries[id.Number]
	if !ok {
		return nil, fmt.Errorf("object %v is absent from xref", id)
	}
	d.loading[id] = true
	defer delete(d.loading, id)
	var obj *pdfmodel.IndirectObject
	var err error
	switch entry := record.Entry.(type) {
	case pdfmodel.InUseEntry:
		if entry.Generation != id.Generation {
			return nil, fmt.Errorf("generation mismatch for object %v (xref generation %d)", id, entry.Generation)
		}
		if entry.Offset < 0 || entry.Offset >= d.size {
			return nil, fmt.Errorf("object %v offset outside file", id)
		}
		obj, err = d.parseIndirect(entry.Offset)
	case pdfmodel.CompressedEntry:
		if id.Generation != 0 {
			return nil, errors.New("compressed object generation must be zero")
		}
		obj, err = d.loadCompressed(id, entry)
	case pdfmodel.FreeEntry:
		return nil, fmt.Errorf("object %v is free", id)
	default:
		return nil, fmt.Errorf("object %v has unsupported xref entry", id)
	}
	if err != nil {
		return nil, fmt.Errorf("load object %v: %w", id, err)
	}
	if obj.ID != id {
		return nil, fmt.Errorf("xref requested %v but file defines %v", id, obj.ID)
	}
	d.cache[id] = obj
	return obj, nil
}

func (d *Document) parseIndirect(start int64) (*pdfmodel.IndirectObject, error) {
	if old, ok := d.physical[start]; ok {
		return old, nil
	}
	if start < 0 || start >= d.size {
		return nil, errors.New("indirect object offset outside file")
	}
	c := d.cursor(start)
	number, begin, err := c.uint(32)
	if err != nil {
		return nil, err
	}
	generation, _, err := c.uint(16)
	if err != nil {
		return nil, err
	}
	word, _, err := c.word()
	if err != nil {
		return nil, err
	}
	if word != "obj" || number == 0 {
		return nil, fmt.Errorf("invalid indirect object header at byte %d", start)
	}
	headerEnd := c.pos
	body, err := c.object()
	if err != nil {
		return nil, err
	}
	word, keywordStart, err := c.word()
	if err != nil {
		return nil, err
	}
	if word == "stream" {
		dict, ok := body.Value.(pdfmodel.Dictionary)
		if !ok {
			return nil, errors.New("stream must follow a dictionary")
		}
		keywordEnd := c.pos
		newline, err := c.take()
		if err != nil {
			return nil, err
		}
		switch newline {
		case '\n':
		case '\r':
			next, e := c.take()
			if e != nil {
				return nil, e
			}
			if next != '\n' {
				return nil, fmt.Errorf("stream keyword at %d requires LF or CRLF", keywordStart)
			}
		default:
			return nil, fmt.Errorf("stream keyword at %d lacks end-of-line", keywordStart)
		}
		dataStart := c.pos
		length, err := d.dictInt(dict, "Length")
		if err != nil {
			return nil, fmt.Errorf("stream /Length: %w", err)
		}
		if length < 0 || length > d.size-dataStart {
			return nil, errors.New("stream Length outside file")
		}
		dataEnd := dataStart + length
		c.pos = dataEnd
		// Skip encoded bytes using /Length. Only the optional separating EOL may
		// lie outside it; arbitrary whitespace/comments could truncate binary data.
		next, err := c.peek()
		if err != nil {
			return nil, err
		}
		switch next {
		case '\r':
			c.pos++
			next, err = c.peek()
			if err != nil {
				return nil, err
			}
			if next == '\n' {
				c.pos++
			}
		case '\n':
			c.pos++
		}
		endStart := c.pos
		match, err := c.prefix("endstream")
		if err != nil {
			return nil, err
		}
		if !match {
			return nil, fmt.Errorf("stream Length does not end at endstream (byte %d)", dataEnd)
		}
		c.pos += 9
		next, err = c.peek()
		if err != nil && err != io.EOF {
			return nil, err
		}
		if err == nil && !docDelimiter(next) {
			return nil, fmt.Errorf("invalid endstream delimiter at byte %d", c.pos)
		}
		encoded := pdfmodel.Span{Source: 1, Start: dataStart, End: dataEnd}
		endKeyword := pdfmodel.Span{Source: 1, Start: endStart, End: c.pos}
		body.Value = pdfmodel.Stream{Dictionary: dict, DictionarySpan: body.Span, StartKeyword: pdfmodel.Span{Source: 1, Start: keywordStart, End: keywordEnd}, DataStart: pdfmodel.Position{Source: 1, Offset: dataStart}, Encoded: &encoded, EndKeyword: &endKeyword, Boundary: pdfmodel.StreamFromLength}
		body.Span.End = c.pos
		word, keywordStart, err = c.word()
		if err != nil {
			return nil, err
		}
	}
	if word != "endobj" {
		return nil, fmt.Errorf("expected endobj at byte %d, got %q", keywordStart, word)
	}
	endobj := pdfmodel.Span{Source: 1, Start: keywordStart, End: c.pos}
	whole := pdfmodel.Span{Source: 1, Start: begin, End: c.pos}
	obj := &pdfmodel.IndirectObject{ID: pdfmodel.ObjectID{Number: uint32(number), Generation: uint16(generation)}, Body: body, Origin: pdfmodel.FileObjectOrigin{Whole: whole, Header: pdfmodel.Span{Source: 1, Start: begin, End: headerEnd}, EndObj: &endobj}}
	if len(d.Structure.Objects) >= d.Options.Limits.MaxObjects {
		return nil, fmt.Errorf("parsed object limit exceeded: %w", pdfmodel.ErrLimit)
	}
	if err := d.markRegion(pdfmodel.RegionIndirectObject, whole); err != nil {
		return nil, err
	}
	d.physical[start] = obj
	d.Structure.Objects = append(d.Structure.Objects, *obj)
	return obj, nil
}

func (d *Document) loadCompressed(id pdfmodel.ObjectID, entry pdfmodel.CompressedEntry) (*pdfmodel.IndirectObject, error) {
	container, err := d.Load(pdfmodel.ObjectID{Number: entry.StreamNumber})
	if err != nil {
		return nil, err
	}
	stream, ok := container.Body.Value.(pdfmodel.Stream)
	if !ok {
		return nil, errors.New("object stream container is not a stream")
	}
	kind, err := stream.Dictionary.Get("Type")
	if err != nil || kind.Value != pdfmodel.Name("ObjStm") {
		return nil, errors.New("compressed object requires /Type /ObjStm")
	}
	index, err := d.objectStreamIndex(container.ID, stream)
	if err != nil {
		return nil, err
	}
	if uint64(entry.Index) >= uint64(len(index.pairs)) {
		return nil, errors.New("invalid object stream index")
	}
	p := index.pairs[entry.Index]
	if p.number != id.Number {
		return nil, errors.New("object stream index points to wrong object number")
	}
	start, end := index.first+p.offset, index.size
	if int(entry.Index)+1 < len(index.pairs) {
		end = index.first + index.pairs[int(entry.Index)+1].offset
	}
	data, err := d.Bytes(pdfmodel.Span{Source: index.source, Start: start, End: end})
	if err != nil {
		return nil, err
	}
	body, consumed, err := d.parseObject(data, index.source, start)
	if err != nil {
		return nil, err
	}
	if skipDocSpace(data, consumed) != len(data) {
		return nil, errors.New("extra data in compressed object")
	}
	origin, ok := container.Origin.(pdfmodel.FileObjectOrigin)
	if !ok {
		return nil, errors.New("object stream must be a file object")
	}
	obj := &pdfmodel.IndirectObject{ID: id, Body: body, Origin: pdfmodel.ObjectStreamOrigin{Container: container.ID, ContainerSpan: origin.Whole, Index: entry.Index, HeaderPair: p.span}}
	if len(d.Structure.Objects) >= d.Options.Limits.MaxObjects {
		return nil, fmt.Errorf("parsed object limit exceeded: %w", pdfmodel.ErrLimit)
	}
	d.Structure.Objects = append(d.Structure.Objects, *obj)
	return obj, nil
}

type objectStreamPair struct {
	number uint32
	offset int64
	span   pdfmodel.Span
}

type objectStreamIndex struct {
	source pdfmodel.SourceID
	size   int64
	first  int64
	pairs  []objectStreamPair
}

func (d *Document) objectStreamIndex(id pdfmodel.ObjectID, stream pdfmodel.Stream) (*objectStreamIndex, error) {
	if index, ok := d.objectStreams[id]; ok {
		return index, nil
	}
	n, err := d.dictInt(stream.Dictionary, "N")
	if err != nil {
		return nil, err
	}
	if n > int64(d.Options.Limits.MaxObjects) {
		return nil, fmt.Errorf("object stream /N exceeds object limit: %w", pdfmodel.ErrLimit)
	}
	if n <= 0 {
		return nil, errors.New("invalid object stream /N")
	}
	first, err := d.dictInt(stream.Dictionary, "First")
	if err != nil {
		return nil, err
	}
	if first < 0 {
		return nil, errors.New("invalid object stream /First")
	}
	source, err := d.DecodeStream(stream)
	if err != nil {
		return nil, err
	}
	if first > source.Size || n > first/3 {
		return nil, errors.New("object stream header outside decoded data")
	}
	data, err := d.Bytes(pdfmodel.Span{Source: source.ID, End: first})
	if err != nil {
		return nil, err
	}
	index := &objectStreamIndex{source: source.ID, size: source.Size, first: first, pairs: make([]objectStreamPair, 0, int(n))}
	seen := make(map[uint32]bool)
	pos := 0
	for i := int64(0); i < n; i++ {
		number, begin, err := docUint(data, &pos, 32)
		if err != nil {
			return nil, err
		}
		offset, _, err := docUint(data, &pos, 63)
		if err != nil {
			return nil, err
		}
		if number == 0 || seen[uint32(number)] || offset > uint64(source.Size-first) {
			return nil, errors.New("invalid object stream header pair")
		}
		seen[uint32(number)] = true
		if i > 0 && int64(offset) <= index.pairs[len(index.pairs)-1].offset {
			return nil, errors.New("object stream offsets are not increasing")
		}
		index.pairs = append(index.pairs, objectStreamPair{number: uint32(number), offset: int64(offset), span: pdfmodel.Span{Source: source.ID, Start: int64(begin), End: int64(pos)}})
	}
	if skipDocSpace(data, pos) != len(data) {
		return nil, errors.New("extra object stream header data")
	}
	if d.objectStreams == nil {
		d.objectStreams = make(map[pdfmodel.ObjectID]*objectStreamIndex)
	}
	d.objectStreams[id] = index
	return index, nil
}
