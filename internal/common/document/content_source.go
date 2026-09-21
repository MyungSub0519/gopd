package document

import (
	"errors"
	"fmt"
	"io"
	"sort"

	"github.com/MyungSub0519/gopd/internal/common/pdfmodel"
)

// JoinContentSources registers a logical concatenation without copying stream
// bytes. Inputs retain the exact ranges and order needed to recover provenance.
// A single complete source is returned unchanged. The interpreter must separately
// charge cumulative visits and bytes before calling this method.
func JoinContentSources(d *Document, inputs []pdfmodel.Span) (pdfmodel.Source, error) {
	if len(inputs) == 0 {
		return pdfmodel.Source{}, fmt.Errorf("no content sources to concatenate")
	}
	if len(inputs) > d.Options.Limits.MaxSemanticObjects {
		return pdfmodel.Source{}, fmt.Errorf("%w: content source count limit exceeded", pdfmodel.ErrLimit)
	}
	var size int64
	for _, input := range inputs {
		source, ok := d.Sources[input.Source]
		if !ok || input.Start < 0 || input.End < input.Start || input.End > source.Size {
			return pdfmodel.Source{}, fmt.Errorf("invalid content source span %+v", input)
		}
		length := input.End - input.Start
		if length > d.Options.Limits.MaxContentBytes-size {
			return pdfmodel.Source{}, fmt.Errorf("%w: content source byte limit exceeded", pdfmodel.ErrLimit)
		}
		size += length
	}
	if len(inputs) == 1 {
		source := d.Sources[inputs[0].Source]
		if inputs[0].Start == 0 && inputs[0].End == source.Size {
			return source, nil
		}
	}
	id := pdfmodel.SourceID(len(d.Sources) + 1)
	if id == 0 {
		return pdfmodel.Source{}, fmt.Errorf("%w: source ID limit exceeded", pdfmodel.ErrLimit)
	}
	reader := &contentReader{segments: make([]contentSegment, 0, len(inputs))}
	var start int64
	for _, input := range inputs {
		end := start + input.End - input.Start
		reader.segments = append(reader.segments, contentSegment{
			reader: d.Sources[input.Source].Reader,
			offset: input.Start,
			start:  start,
			end:    end,
		})
		start = end
	}
	source := pdfmodel.Source{
		ID: id, Reader: reader, Size: size,
		Origin: &pdfmodel.Derivation{Inputs: append([]pdfmodel.Span(nil), inputs...)},
	}
	d.Sources[id] = source
	return source, nil
}

type contentSegment struct {
	reader io.ReaderAt
	offset int64
	start  int64
	end    int64
}

type contentReader struct {
	segments []contentSegment
}

func (r *contentReader) ReadAt(p []byte, offset int64) (int, error) {
	if offset < 0 {
		return 0, fmt.Errorf("negative content offset")
	}
	if len(p) == 0 {
		return 0, nil
	}
	i := sort.Search(len(r.segments), func(i int) bool { return r.segments[i].end > offset })
	n := 0
	for i < len(r.segments) && n < len(p) {
		segment := r.segments[i]
		length := int(min(int64(len(p)-n), segment.end-offset))
		read, err := segment.reader.ReadAt(p[n:n+length], segment.offset+offset-segment.start)
		n += read
		if read != length {
			if err == nil {
				err = io.ErrUnexpectedEOF
			}
			return n, err
		}
		if err != nil && !errors.Is(err, io.EOF) {
			return n, err
		}
		offset += int64(read)
		i++
	}
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}
