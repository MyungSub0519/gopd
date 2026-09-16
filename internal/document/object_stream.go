package document

import (
	"errors"
	"fmt"

	"github.com/MyungSub0519/gopd/internal/pdfmodel"
)

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
