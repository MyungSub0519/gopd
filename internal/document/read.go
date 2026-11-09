package document

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/MyungSub0519/gopd/internal/pdfmodel"
)

// ReadOptions bounds input, recursive parsing, xrefs and decoded stream data.
// Zero fields use defaults. Negative values are invalid.
type ReadOptions struct {
	MaxFileBytes int64
	// BufferSize is the shared input buffer size in bytes; zero uses 64 KiB.
	// It does not bound decoded streams or extracted results.
	BufferSize int
	Limits     pdfmodel.Limits
}

func normalizeOptions(options []ReadOptions) (ReadOptions, error) {
	if len(options) > 1 {
		return ReadOptions{}, errors.New("at most one ReadOptions value is accepted")
	}
	var o ReadOptions
	if len(options) == 1 {
		o = options[0]
	}
	if o.MaxFileBytes < 0 || o.BufferSize < 0 || o.Limits.MaxDepth < 0 || o.Limits.MaxTokenBytes < 0 ||
		o.Limits.MaxObjects < 0 || o.Limits.MaxXRefSections < 0 || o.Limits.MaxDecodedBytes < 0 ||
		o.Limits.MaxValues < 0 || o.Limits.MaxContentBytes < 0 ||
		o.Limits.MaxSemanticObjects < 0 || o.Limits.MaxRegionWork < 0 {
		return o, errors.New("negative PDF read limit")
	}
	if o.MaxFileBytes == 0 {
		o.MaxFileBytes = 256 << 20
	}
	if o.BufferSize == 0 {
		o.BufferSize = 64 << 10
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

// Load opens a file and prepares buffered object access. Close any non-nil
// returned Document, including a partial Document returned with an error.
func Load(path string, options ...ReadOptions) (*Document, error) {
	o, err := normalizeOptions(options)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	stat, err := f.Stat()
	if err != nil {
		return nil, errors.Join(err, f.Close())
	}
	d, err := Read(f, stat.Size(), o)
	if d == nil {
		return nil, errors.Join(err, f.Close())
	}
	d.closer = f
	return d, err
}

// Read borrows r; keep it open and unchanged until Document.Close.
// It never closes a caller-owned ReaderAt.
func Read(r io.ReaderAt, size int64, options ...ReadOptions) (*Document, error) {
	o, err := normalizeOptions(options)
	if err != nil {
		return nil, err
	}
	if r == nil || size <= 0 {
		return nil, fmt.Errorf("invalid PDF size %d or nil reader", size)
	}
	if size > o.MaxFileBytes {
		return nil, fmt.Errorf("PDF size %d exceeds input limit: %w", size, pdfmodel.ErrLimit)
	}
	d := &Document{
		input:    newBufferedInput(r, size, o.BufferSize),
		size:     size,
		Options:  o,
		Sources:  make(map[pdfmodel.SourceID]pdfmodel.Source),
		entries:  make(map[uint32]pdfmodel.XRefRecord),
		cache:    make(map[pdfmodel.ObjectID]*pdfmodel.IndirectObject),
		physical: make(map[int64]*pdfmodel.IndirectObject),
		loading:  make(map[pdfmodel.ObjectID]bool),
		decoded:  make(map[pdfmodel.Span]pdfmodel.SourceID),
	}
	d.Sources[1] = pdfmodel.Source{ID: 1, Reader: d.input, Size: size}
	d.Structure.File = 1
	d.Structure.Regions = []pdfmodel.FileRegion{{Kind: pdfmodel.RegionUnknown, Span: pdfmodel.Span{Source: 1, End: size}}}
	if err = d.readHeaderAndXRefs(); err != nil {
		return d, err
	}
	return d, nil
}
