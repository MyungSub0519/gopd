package gopd

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
)

// ParseFile snapshots a file and prepares low-level object access. It closes
// the file before returning. At most one ReadOptions value may be supplied;
// a structural error can return a partial Document together with the error.
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
