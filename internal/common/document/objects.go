package document

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/MyungSub0519/gopd/internal/common/pdfmodel"
)

func (d *Document) Load(id pdfmodel.ObjectID) (*pdfmodel.IndirectObject, error) {
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
		if entry.Offset < 0 || entry.Offset >= int64(len(d.data)) {
			return nil, fmt.Errorf("object %v offset outside file", id)
		}
		obj, err = d.parseIndirect(int(entry.Offset))
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

func (d *Document) parseIndirect(start int) (*pdfmodel.IndirectObject, error) {
	if old, ok := d.physical[int64(start)]; ok {
		return old, nil
	}
	if start < 0 || start >= len(d.data) {
		return nil, errors.New("indirect object offset outside file")
	}
	pos := start
	number, begin, err := docUint(d.data, &pos, 32)
	if err != nil {
		return nil, err
	}
	generation, _, err := docUint(d.data, &pos, 16)
	if err != nil {
		return nil, err
	}
	word, _, err := docWord(d.data, &pos)
	if err != nil || word != "obj" || number == 0 {
		return nil, fmt.Errorf("invalid indirect object header at byte %d", start)
	}
	headerEnd := pos
	body, n, err := d.parseObject(d.data[pos:], 1, int64(pos))
	if err != nil {
		return nil, err
	}
	pos += n
	word, keywordStart, err := docWord(d.data, &pos)
	if err != nil {
		return nil, err
	}
	if word == "stream" {
		dict, ok := body.Value.(pdfmodel.Dictionary)
		if !ok {
			return nil, errors.New("stream must follow a dictionary")
		}
		keywordEnd := pos
		if pos >= len(d.data) {
			return nil, errors.New("truncated stream start")
		}
		switch d.data[pos] {
		case '\n':
			pos++
		case '\r':
			if pos+1 >= len(d.data) || d.data[pos+1] != '\n' {
				return nil, fmt.Errorf("stream keyword at %d requires LF or CRLF", keywordStart)
			}
			pos += 2
		default:
			return nil, fmt.Errorf("stream keyword at %d lacks end-of-line", keywordStart)
		}
		dataStart := pos
		length, err := d.dictInt(dict, "Length")
		if err != nil {
			return nil, fmt.Errorf("stream /Length: %w", err)
		}
		if length < 0 || length > int64(len(d.data)-dataStart) {
			return nil, errors.New("stream Length outside file")
		}
		dataEnd := dataStart + int(length)
		pos = dataEnd
		// Only the optional separating EOL may lie outside /Length. Treating
		// arbitrary bytes as comments here can silently truncate binary data.
		if pos < len(d.data) && d.data[pos] == '\r' {
			pos++
			if pos < len(d.data) && d.data[pos] == '\n' {
				pos++
			}
		} else if pos < len(d.data) && d.data[pos] == '\n' {
			pos++
		}
		endStart := pos
		if !bytes.HasPrefix(d.data[pos:], []byte("endstream")) || (pos+9 < len(d.data) && !docDelimiter(d.data[pos+9])) {
			return nil, fmt.Errorf("stream Length does not end at endstream (byte %d)", dataEnd)
		}
		pos += 9
		encoded := pdfmodel.Span{Source: 1, Start: int64(dataStart), End: int64(dataEnd)}
		endKeyword := pdfmodel.Span{Source: 1, Start: int64(endStart), End: int64(pos)}
		body.Value = pdfmodel.Stream{Dictionary: dict, DictionarySpan: body.Span, StartKeyword: pdfmodel.Span{Source: 1, Start: int64(keywordStart), End: int64(keywordEnd)}, DataStart: pdfmodel.Position{Source: 1, Offset: int64(dataStart)}, Encoded: &encoded, EndKeyword: &endKeyword, Boundary: pdfmodel.StreamFromLength}
		body.Span.End = int64(pos)
		word, keywordStart, err = docWord(d.data, &pos)
		if err != nil {
			return nil, err
		}
	}
	if word != "endobj" {
		return nil, fmt.Errorf("expected endobj at byte %d, got %q", keywordStart, word)
	}
	endobj := pdfmodel.Span{Source: 1, Start: int64(keywordStart), End: int64(pos)}
	whole := pdfmodel.Span{Source: 1, Start: int64(begin), End: int64(pos)}
	obj := &pdfmodel.IndirectObject{ID: pdfmodel.ObjectID{Number: uint32(number), Generation: uint16(generation)}, Body: body, Origin: pdfmodel.FileObjectOrigin{Whole: whole, Header: pdfmodel.Span{Source: 1, Start: int64(begin), End: int64(headerEnd)}, EndObj: &endobj}}
	if len(d.Structure.Objects) >= d.Options.Limits.MaxObjects {
		return nil, fmt.Errorf("parsed object limit exceeded: %w", pdfmodel.ErrLimit)
	}
	if err := d.markRegion(pdfmodel.RegionIndirectObject, whole); err != nil {
		return nil, err
	}
	d.physical[int64(start)] = obj
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
