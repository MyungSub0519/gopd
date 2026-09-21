package document

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/MyungSub0519/gopd/internal/common/pdfmodel"
)

// ReadOptions bounds input, recursive parsing, xrefs and decoded stream data.
// Zero fields use defaults. Negative values are invalid.
type ReadOptions struct {
	MaxFileBytes int64
	Limits       pdfmodel.Limits
}

func normalizeOptions(options []ReadOptions) (ReadOptions, error) {
	if len(options) > 1 {
		return ReadOptions{}, errors.New("at most one ReadOptions value is accepted")
	}
	var o ReadOptions
	if len(options) == 1 {
		o = options[0]
	}
	if o.MaxFileBytes < 0 || o.Limits.MaxDepth < 0 || o.Limits.MaxTokenBytes < 0 ||
		o.Limits.MaxObjects < 0 || o.Limits.MaxXRefSections < 0 || o.Limits.MaxDecodedBytes < 0 ||
		o.Limits.MaxValues < 0 || o.Limits.MaxContentBytes < 0 ||
		o.Limits.MaxSemanticObjects < 0 || o.Limits.MaxRegionWork < 0 {
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
	if o.Limits.MaxValues == 0 {
		o.Limits.MaxValues = 1 << 20
	}
	if o.Limits.MaxContentBytes == 0 {
		o.Limits.MaxContentBytes = 256 << 20
	}
	if o.Limits.MaxSemanticObjects == 0 {
		o.Limits.MaxSemanticObjects = o.Limits.MaxObjects
	}
	if o.Limits.MaxRegionWork == 0 {
		o.Limits.MaxRegionWork = 16 << 20
	}
	return o, nil
}

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
	if r == nil || size <= 0 {
		return nil, fmt.Errorf("invalid PDF size %d or nil reader", size)
	}
	if size > o.MaxFileBytes || uint64(size) > uint64(^uint(0)>>1) {
		return nil, fmt.Errorf("PDF size %d exceeds input limit: %w", size, pdfmodel.ErrLimit)
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
	d := &Document{
		data:     data,
		Options:  o,
		Sources:  make(map[pdfmodel.SourceID]pdfmodel.Source),
		entries:  make(map[uint32]pdfmodel.XRefRecord),
		cache:    make(map[pdfmodel.ObjectID]*pdfmodel.IndirectObject),
		physical: make(map[int64]*pdfmodel.IndirectObject),
		loading:  make(map[pdfmodel.ObjectID]bool),
		decoded:  make(map[pdfmodel.Span]pdfmodel.SourceID),
	}
	d.Sources[1] = pdfmodel.Source{ID: 1, Reader: bytes.NewReader(data), Size: size}
	d.Structure.File = 1
	d.Structure.Regions = []pdfmodel.FileRegion{{Kind: pdfmodel.RegionUnknown, Span: pdfmodel.Span{Source: 1, End: size}}}
	if err = d.readHeaderAndXRefs(); err != nil {
		return d, err
	}
	return d, nil
}
