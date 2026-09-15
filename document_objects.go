package pdf

import (
	"bytes"
	"errors"
	"fmt"
)

func (d *Document) Load(id ObjectID) (*IndirectObject, error) {
	if id.Number == 0 {
		return nil, errors.New("object zero is reserved")
	}
	if obj, ok := d.cache[id]; ok {
		return obj, nil
	}
	if d.loading[id] || len(d.loading) >= d.Options.Limits.MaxDepth {
		return nil, fmt.Errorf("cyclic or too-deep object load %v", id)
	}
	record, ok := d.entries[id.Number]
	if !ok {
		return nil, fmt.Errorf("object %v is absent from xref", id)
	}
	d.loading[id] = true
	defer delete(d.loading, id)
	var obj *IndirectObject
	var err error
	switch entry := record.Entry.(type) {
	case InUseEntry:
		if entry.Generation != id.Generation {
			return nil, fmt.Errorf("generation mismatch for object %v (xref generation %d)", id, entry.Generation)
		}
		if entry.Offset < 0 || entry.Offset >= int64(len(d.data)) {
			return nil, fmt.Errorf("object %v offset outside file", id)
		}
		obj, err = d.parseIndirect(int(entry.Offset))
	case CompressedEntry:
		if id.Generation != 0 {
			return nil, errors.New("compressed object generation must be zero")
		}
		obj, err = d.loadCompressed(id, entry)
	case FreeEntry:
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

func (d *Document) parseIndirect(start int) (*IndirectObject, error) {
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
	body, n, err := parseObjectWithLimits(d.data[pos:], 1, int64(pos), d.Options.Limits)
	if err != nil {
		return nil, err
	}
	pos += n
	word, keywordStart, err := docWord(d.data, &pos)
	if err != nil {
		return nil, err
	}
	if word == "stream" {
		dict, ok := body.Value.(Dictionary)
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
		encoded := Span{Source: 1, Start: int64(dataStart), End: int64(dataEnd)}
		endKeyword := Span{Source: 1, Start: int64(endStart), End: int64(pos)}
		body.Value = Stream{Dictionary: dict, DictionarySpan: body.Span, StartKeyword: Span{Source: 1, Start: int64(keywordStart), End: int64(keywordEnd)}, DataStart: Position{Source: 1, Offset: int64(dataStart)}, Encoded: &encoded, EndKeyword: &endKeyword, Boundary: StreamFromLength}
		body.Span.End = int64(pos)
		word, keywordStart, err = docWord(d.data, &pos)
		if err != nil {
			return nil, err
		}
	}
	if word != "endobj" {
		return nil, fmt.Errorf("expected endobj at byte %d, got %q", keywordStart, word)
	}
	endobj := Span{Source: 1, Start: int64(keywordStart), End: int64(pos)}
	whole := Span{Source: 1, Start: int64(begin), End: int64(pos)}
	obj := &IndirectObject{ID: ObjectID{Number: uint32(number), Generation: uint16(generation)}, Body: body, Origin: FileObjectOrigin{Whole: whole, Header: Span{Source: 1, Start: int64(begin), End: int64(headerEnd)}, EndObj: &endobj}}
	if len(d.Structure.Objects) >= d.Options.Limits.MaxObjects {
		return nil, errors.New("parsed object limit exceeded")
	}
	d.physical[int64(start)] = obj
	d.Structure.Objects = append(d.Structure.Objects, *obj)
	d.markRegion(RegionIndirectObject, whole)
	return obj, nil
}

func (d *Document) loadCompressed(id ObjectID, entry CompressedEntry) (*IndirectObject, error) {
	container, err := d.Load(ObjectID{Number: entry.StreamNumber})
	if err != nil {
		return nil, err
	}
	stream, ok := container.Body.Value.(Stream)
	if !ok {
		return nil, errors.New("object stream container is not a stream")
	}
	kind, err := stream.Dictionary.Get("Type")
	if err != nil || kind.Value != Name("ObjStm") {
		return nil, errors.New("compressed object requires /Type /ObjStm")
	}
	n, err := d.dictInt(stream.Dictionary, "N")
	if err != nil || n <= 0 || n > int64(d.Options.Limits.MaxObjects) || int64(entry.Index) >= n {
		return nil, errors.New("invalid object stream /N or index")
	}
	first, err := d.dictInt(stream.Dictionary, "First")
	if err != nil || first < 0 {
		return nil, errors.New("invalid object stream /First")
	}
	source, err := d.DecodeStream(stream)
	if err != nil {
		return nil, err
	}
	data, err := d.Bytes(Span{Source: source.ID, End: source.Size})
	if err != nil {
		return nil, err
	}
	if first > int64(len(data)) || n > first/3 {
		return nil, errors.New("object stream header outside decoded data")
	}
	pos := 0
	type pair struct {
		number uint32
		offset int64
		span   Span
	}
	pairs := make([]pair, 0, int(n))
	seen := make(map[uint32]bool)
	for i := int64(0); i < n; i++ {
		num, begin, e := docUint(data[:int(first)], &pos, 32)
		if e != nil {
			return nil, e
		}
		off, _, e := docUint(data[:int(first)], &pos, 63)
		if e != nil {
			return nil, e
		}
		if num == 0 || seen[uint32(num)] || off > uint64(int64(len(data))-first) {
			return nil, errors.New("invalid object stream header pair")
		}
		seen[uint32(num)] = true
		if i > 0 && int64(off) <= pairs[len(pairs)-1].offset {
			return nil, errors.New("object stream offsets are not increasing")
		}
		pairs = append(pairs, pair{number: uint32(num), offset: int64(off), span: Span{Source: source.ID, Start: int64(begin), End: int64(pos)}})
	}
	if skipDocSpace(data[:int(first)], pos) != int(first) {
		return nil, errors.New("extra object stream header data")
	}
	p := pairs[entry.Index]
	if p.number != id.Number {
		return nil, errors.New("object stream index points to wrong object number")
	}
	start, end := first+p.offset, int64(len(data))
	if int(entry.Index)+1 < len(pairs) {
		end = first + pairs[int(entry.Index)+1].offset
	}
	body, consumed, err := parseObjectWithLimits(data[start:end], source.ID, start, d.Options.Limits)
	if err != nil {
		return nil, err
	}
	if skipDocSpace(data[start:end], consumed) != int(end-start) {
		return nil, errors.New("extra data in compressed object")
	}
	origin, ok := container.Origin.(FileObjectOrigin)
	if !ok {
		return nil, errors.New("object stream must be a file object")
	}
	obj := &IndirectObject{ID: id, Body: body, Origin: ObjectStreamOrigin{Container: container.ID, ContainerSpan: origin.Whole, Index: entry.Index, HeaderPair: p.span}}
	if len(d.Structure.Objects) >= d.Options.Limits.MaxObjects {
		return nil, errors.New("parsed object limit exceeded")
	}
	d.Structure.Objects = append(d.Structure.Objects, *obj)
	return obj, nil
}

// IsStream reports whether a resolved syntax object contains stream data.
func IsStream(object Object) bool { _, ok := object.Value.(Stream); return ok }

// RawObject returns the original syntax of an object, including its delimiters.
func (d *Document) RawObject(object Object) ([]byte, error) { return d.Bytes(object.Span) }
