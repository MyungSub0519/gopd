package structure

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/MyungSub0519/gopd/internal/model"
)

// ParseFile reads a file into a Document and closes it before returning.
//
// The whole file is snapshotted, so the caller owes the result no cleanup and
// the file may be replaced or deleted immediately afterwards.
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

// Parse reads size bytes from r into a Document.
//
// The caller keeps ownership of r, which is never closed here and is not
// retained after this call returns.
//
// A structural failure returns a partial Document together with the error, so
// that what was learned before the failure — the header, the regions, the
// sections already read — remains available for inspection. Callers must check
// the error before treating the result as sound.
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
	d := &Document{data: data, Options: o, Sources: make(map[model.SourceID]model.Source), entries: make(map[uint32]model.XRefRecord), cache: make(map[model.ObjectID]*model.IndirectObject), physical: make(map[int64]*model.IndirectObject), loading: make(map[model.ObjectID]bool), decoded: make(map[model.Span]model.SourceID)}
	d.Sources[1] = model.Source{ID: 1, Reader: bytes.NewReader(data), Size: size}
	// Source 1 is the file itself, and the whole of it starts out
	// unaccounted for; markRegion carves it up as the reader recognises
	// pieces. See Document.markRegion.
	d.Structure.File = 1
	d.Structure.Regions = []model.FileRegion{{Kind: model.RegionUnknown, Span: model.Span{Source: 1, End: size}}}
	if err = d.readHeaderAndXRefs(); err != nil {
		return d, err
	}
	return d, nil
}
