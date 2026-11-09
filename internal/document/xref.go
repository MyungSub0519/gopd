package document

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strconv"

	"github.com/MyungSub0519/gopd/internal/pdfmodel"
)

func (d *Document) readHeaderAndXRefs() error {
	head, err := d.Bytes(pdfmodel.Span{Source: 1, End: min(d.size, 1031)})
	if err != nil {
		return err
	}
	header := bytes.Index(head[:min(len(head), 1024)], []byte("%PDF-"))
	if header < 0 || header+8 > len(head) {
		return errors.New("missing PDF header")
	}
	version := string(head[header+5 : header+8])
	if version[1] != '.' || version[0] < '1' || version[0] > '2' || version[2] < '0' || version[2] > '9' {
		return fmt.Errorf("invalid PDF header version %q", version)
	}
	d.Structure.Header = &pdfmodel.Header{Span: pdfmodel.Span{Source: 1, Start: int64(header), End: int64(header + 8)}, Version: pdfmodel.Version{Major: version[0] - '0', Minor: version[2] - '0'}}
	if err := d.markRegion(pdfmodel.RegionHeader, d.Structure.Header.Span); err != nil {
		return err
	}
	if header != 0 {
		d.Structure.Diagnostics = append(d.Structure.Diagnostics, pdfmodel.Diagnostic{Severity: pdfmodel.SeverityWarning, Code: "leading-data", Message: "bytes precede PDF header", Span: pdfmodel.Span{Source: 1, End: int64(header)}})
	}
	start, err := d.findLastStartXRef()
	if err != nil {
		return err
	}
	if start < 0 {
		return errors.New("missing startxref")
	}
	c := d.cursor(start + 9)
	offset, raw, err := c.uint(63)
	if err != nil {
		return err
	}
	rawEnd := c.pos
	// %%EOF is a marker here, not a comment.
	for {
		b, e := c.peek()
		if e != nil {
			return e
		}
		if !docSpace(b) {
			break
		}
		c.pos++
	}
	ok, err := c.prefix("%%EOF")
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("missing EOF marker after startxref at byte %d", start)
	}
	tail := pdfmodel.FileTail{StartXRef: pdfmodel.Span{Source: 1, Start: start, End: start + 9}, Offset: int64(offset), OffsetRaw: pdfmodel.Span{Source: 1, Start: raw, End: rawEnd}, EOFMarker: pdfmodel.Span{Source: 1, Start: c.pos, End: c.pos + 5}}
	d.Structure.Tails = append(d.Structure.Tails, tail)
	if err := d.markRegion(pdfmodel.RegionFileTail, pdfmodel.Span{Source: 1, Start: start, End: c.pos + 5}); err != nil {
		return err
	}
	seen := make(map[int64]bool)
	if err = d.readXRefChain(int64(offset), seen); err != nil {
		return err
	}
	dict, ok := d.trailer.Value.(pdfmodel.Dictionary)
	if !ok {
		return errors.New("missing trailer")
	}
	if encrypt, e := dict.Get("Encrypt"); e == nil {
		_, null := encrypt.Value.(pdfmodel.Null)
		d.Encrypted = !null
	} else if !errors.Is(e, pdfmodel.ErrMissingKey) {
		return e
	}
	return nil
}

func (d *Document) readXRefChain(offset int64, seen map[int64]bool) error {
	if seen[offset] {
		return fmt.Errorf("xref cycle/repeated section at byte %d", offset)
	}
	if len(seen) >= d.Options.Limits.MaxXRefSections {
		return fmt.Errorf("xref section limit exceeded: %w", pdfmodel.ErrLimit)
	}
	seen[offset] = true
	section, err := d.readXRefSection(offset)
	if err != nil {
		return fmt.Errorf("xref at byte %d: %w", offset, err)
	}
	section.ID = pdfmodel.SectionID(len(d.Structure.XRefs) + 1)
	d.Structure.XRefs = append(d.Structure.XRefs, section)
	if d.trailer.Value == nil {
		d.trailer = section.Trailer
	}
	// A hybrid stream overrides its companion table, but never newer revisions.
	if section.XRefStm != nil {
		hybridOff := *section.XRefStm
		if seen[hybridOff] {
			return errors.New("hybrid xref cycle/repeated section")
		}
		if len(seen) >= d.Options.Limits.MaxXRefSections {
			return fmt.Errorf("hybrid xref section limit exceeded: %w", pdfmodel.ErrLimit)
		}
		seen[hybridOff] = true
		hybrid, e := d.readXRefSection(hybridOff)
		if e != nil {
			return e
		}
		if hybrid.Form != pdfmodel.XRefStream {
			return errors.New("XRefStm does not refer to xref stream")
		}
		hybrid.ID = pdfmodel.SectionID(len(d.Structure.XRefs) + 1)
		d.Structure.XRefs = append(d.Structure.XRefs, hybrid)
		if e = d.mergeEntries(hybrid); e != nil {
			return e
		}
	}
	if err = d.mergeEntries(section); err != nil {
		return err
	}
	if section.Prev != nil {
		return d.readXRefChain(*section.Prev, seen)
	}
	return nil
}

func (d *Document) mergeEntries(section pdfmodel.XRefSection) error {
	within := make(map[uint32]bool)
	for _, r := range section.Ranges {
		for _, record := range r.Records {
			if within[record.Number] {
				return fmt.Errorf("duplicate xref object %d in section at %d", record.Number, section.Offset)
			}
			within[record.Number] = true
			if _, exists := d.entries[record.Number]; !exists {
				d.entries[record.Number] = record
			}
		}
	}
	if len(d.entries) > d.Options.Limits.MaxObjects {
		return fmt.Errorf("xref object limit exceeded: %w", pdfmodel.ErrLimit)
	}
	return nil
}

func (d *Document) reserveXRefRange(count uint64) error {
	limit := d.Options.Limits.MaxObjects
	if d.xrefRanges >= limit || count > uint64(limit-d.xrefRecords) {
		return fmt.Errorf("cumulative xref record/subsection limit exceeded: %w", pdfmodel.ErrLimit)
	}
	d.xrefRanges++
	d.xrefRecords += int(count)
	return nil
}

func (d *Document) readXRefSection(offset int64) (pdfmodel.XRefSection, error) {
	if offset < 0 || offset >= d.size {
		return pdfmodel.XRefSection{}, errors.New("xref offset outside file")
	}
	c := d.cursor(offset)
	match, err := c.prefix("xref")
	if err != nil {
		return pdfmodel.XRefSection{}, err
	}
	if match {
		c.pos += 4
		next, e := c.peek()
		if e != nil && e != io.EOF {
			return pdfmodel.XRefSection{}, e
		}
		if e == nil && docDelimiter(next) {
			return d.readXRefTable(offset)
		}
	}
	return d.readXRefStream(offset)
}

func (d *Document) xrefLinks(section *pdfmodel.XRefSection) error {
	dict, ok := section.Trailer.Value.(pdfmodel.Dictionary)
	if !ok {
		return errors.New("xref trailer is not a dictionary")
	}
	for _, link := range []struct {
		key pdfmodel.Name
		out **int64
	}{{"Prev", &section.Prev}, {"XRefStm", &section.XRefStm}} {
		obj, err := optionalDirect(dict, link.key)
		if errors.Is(err, pdfmodel.ErrMissingKey) {
			continue
		}
		if err != nil {
			return err
		}
		n, err := pdfmodel.Int(obj)
		if err != nil || n < 0 || n >= d.size {
			return fmt.Errorf("invalid /%s xref offset", link.key)
		}
		*link.out = &n
	}
	return nil
}

func (d *Document) readXRefTable(start int64) (pdfmodel.XRefSection, error) {
	section := pdfmodel.XRefSection{Form: pdfmodel.XRefTable, Offset: int64(start)}
	c := d.cursor(start + 4)
	total := 0
	for {
		word, begin, err := c.word()
		if err != nil {
			return section, err
		}
		if word == "trailer" {
			obj, err := c.object()
			if err != nil {
				return section, err
			}
			section.Trailer = obj
			section.Span = pdfmodel.Span{Source: 1, Start: int64(start), End: c.pos}
			if err := d.markRegion(pdfmodel.RegionXRef, pdfmodel.Span{Source: 1, Start: int64(start), End: int64(begin)}); err != nil {
				return section, err
			}
			if err := d.markRegion(pdfmodel.RegionTrailer, pdfmodel.Span{Source: 1, Start: int64(begin), End: c.pos}); err != nil {
				return section, err
			}
			return section, d.xrefLinks(&section)
		}
		first, err := strconv.ParseUint(word, 10, 32)
		if err != nil {
			return section, fmt.Errorf("invalid xref subsection at byte %d", begin)
		}
		count, _, err := c.uint(32)
		if err != nil {
			return section, err
		}
		if count > uint64(d.Options.Limits.MaxObjects-total) {
			return section, fmt.Errorf("xref subsection exceeds object limit: %w", pdfmodel.ErrLimit)
		}
		if first+count > 1<<32 || count > uint64(d.size-c.pos)/5 {
			return section, errors.New("xref subsection exceeds input or object number range")
		}
		if err := d.reserveXRefRange(count); err != nil {
			return section, err
		}
		header := pdfmodel.Span{Source: 1, Start: int64(begin), End: c.pos}
		r := pdfmodel.XRefRange{First: uint32(first), Count: uint32(count), Header: &header}
		for i := uint64(0); i < count; i++ {
			off, recordStart, e := c.uint(63)
			if e != nil {
				return section, e
			}
			gen, _, e := c.uint(16)
			if e != nil {
				return section, e
			}
			flag, _, e := c.word()
			if e != nil {
				return section, e
			}
			record := pdfmodel.XRefRecord{Number: uint32(first + i), Span: pdfmodel.Span{Source: 1, Start: int64(recordStart), End: c.pos}}
			switch flag {
			case "n":
				record.Entry = pdfmodel.InUseEntry{Offset: int64(off), Generation: uint16(gen)}
			case "f":
				if off > 1<<32-1 {
					return section, errors.New("free object number overflows uint32")
				}
				record.Entry = pdfmodel.FreeEntry{NextFree: uint32(off), Generation: uint16(gen)}
			default:
				return section, fmt.Errorf("invalid xref flag %q", flag)
			}
			r.Records = append(r.Records, record)
		}
		total += int(count)
		section.Ranges = append(section.Ranges, r)
	}
}

func directInt(dict pdfmodel.Dictionary, key pdfmodel.Name) (int64, error) {
	o, err := dict.Get(key)
	if err != nil {
		return 0, err
	}
	return pdfmodel.Int(o)
}

func (d *Document) readXRefStream(start int64) (pdfmodel.XRefSection, error) {
	section := pdfmodel.XRefSection{Form: pdfmodel.XRefStream, Offset: int64(start)}
	obj, err := d.parseIndirect(start)
	if err != nil {
		return section, err
	}
	stream, ok := obj.Body.Value.(pdfmodel.Stream)
	if !ok {
		return section, errors.New("xref target is not a stream")
	}
	kind, err := stream.Dictionary.Get("Type")
	if err != nil || kind.Value != pdfmodel.Name("XRef") {
		return section, errors.New("xref stream lacks /Type /XRef")
	}
	section.StreamID = &obj.ID
	section.Span = obj.Origin.(pdfmodel.FileObjectOrigin).Whole
	section.Trailer = pdfmodel.Object{Span: stream.DictionarySpan, Value: stream.Dictionary}
	source, err := d.DecodeStream(stream)
	if err != nil {
		return section, err
	}
	data, err := d.Bytes(pdfmodel.Span{Source: source.ID, End: source.Size})
	if err != nil {
		return section, err
	}
	wobj, err := stream.Dictionary.Get("W")
	if err != nil {
		return section, err
	}
	warr, ok := wobj.Value.(pdfmodel.Array)
	if !ok || len(warr.Items) != 3 {
		return section, errors.New("xref /W must contain three integers")
	}
	var widths [3]int
	rowWidth := 0
	for i, v := range warr.Items {
		n, e := pdfmodel.Int(v)
		if e != nil || n < 0 || n > 8 {
			return section, errors.New("unsupported or invalid xref field width")
		}
		widths[i] = int(n)
		rowWidth += int(n)
	}
	if rowWidth == 0 {
		return section, errors.New("zero-width xref record")
	}
	size, err := directInt(stream.Dictionary, "Size")
	if err != nil || size < 0 || size > 1<<32-1 {
		return section, errors.New("invalid xref /Size")
	}
	indices := []int64{0, size}
	if iobj, e := optionalDirect(stream.Dictionary, "Index"); e == nil {
		arr, ok := iobj.Value.(pdfmodel.Array)
		if !ok || len(arr.Items)%2 != 0 {
			return section, errors.New("invalid xref /Index")
		}
		indices = nil
		for _, v := range arr.Items {
			n, e := pdfmodel.Int(v)
			if e != nil || n < 0 {
				return section, errors.New("invalid xref /Index value")
			}
			indices = append(indices, n)
		}
	} else if !errors.Is(e, pdfmodel.ErrMissingKey) {
		return section, e
	}
	pos, total := 0, 0
	for i := 0; i < len(indices); i += 2 {
		first, count := indices[i], indices[i+1]
		if count > int64(d.Options.Limits.MaxObjects-total) {
			return section, fmt.Errorf("xref stream range exceeds object limit: %w", pdfmodel.ErrLimit)
		}
		if first > 1<<32-1 || count > int64((len(data)-pos)/rowWidth) || count > size || first > size-count {
			return section, errors.New("xref stream range outside size or input")
		}
		if err := d.reserveXRefRange(uint64(count)); err != nil {
			return section, err
		}
		r := pdfmodel.XRefRange{First: uint32(first), Count: uint32(count)}
		for j := int64(0); j < count; j++ {
			begin := pos
			fields := [3]uint64{1, 0, 0}
			for k, width := range widths {
				if width == 0 {
					continue
				}
				var value uint64
				for n := 0; n < width; n++ {
					value = value<<8 | uint64(data[pos])
					pos++
				}
				fields[k] = value
			}
			record := pdfmodel.XRefRecord{Number: uint32(first + j), Span: pdfmodel.Span{Source: source.ID, Start: int64(begin), End: int64(pos)}}
			switch fields[0] {
			case 0:
				if fields[1] > 1<<32-1 || fields[2] > 65535 {
					return section, errors.New("xref free entry overflow")
				}
				record.Entry = pdfmodel.FreeEntry{NextFree: uint32(fields[1]), Generation: uint16(fields[2])}
			case 1:
				if fields[1] > uint64(d.size) || fields[2] > 65535 {
					return section, errors.New("xref in-use entry out of range")
				}
				record.Entry = pdfmodel.InUseEntry{Offset: int64(fields[1]), Generation: uint16(fields[2])}
			case 2:
				if fields[1] > 1<<32-1 || fields[2] > 1<<32-1 {
					return section, errors.New("compressed xref entry overflow")
				}
				record.Entry = pdfmodel.CompressedEntry{StreamNumber: uint32(fields[1]), Index: uint32(fields[2])}
			default:
				record.Entry = pdfmodel.UnknownXRefEntry{Type: fields[0]}
				d.Structure.Diagnostics = append(d.Structure.Diagnostics, pdfmodel.Diagnostic{Severity: pdfmodel.SeverityWarning, Code: "unknown-xref-entry", Message: fmt.Sprintf("unsupported xref entry type %d", fields[0]), Span: record.Span})
			}
			r.Records = append(r.Records, record)
		}
		total += int(count)
		section.Ranges = append(section.Ranges, r)
	}
	if pos != len(data) {
		return section, errors.New("extra bytes in xref stream")
	}
	return section, d.xrefLinks(&section)
}

// findLastStartXRef scans backward in bounded windows, including overlap for a
// marker split at a window boundary. No file-sized allocation is needed.
func (d *Document) findLastStartXRef() (int64, error) {
	const marker = "startxref"
	window := min(int64(d.Options.BufferSize), d.size, 64<<10)
	buf := make([]byte, int(window)+len(marker)-1)
	end := d.size
	for end > 0 {
		start := max(int64(0), end-window)
		readEnd := end + min(d.size-end, int64(len(marker)-1))
		data := buf[:int(readEnd-start)]
		n, err := d.input.ReadAt(data, start)
		if n != len(data) {
			if err == nil {
				err = io.ErrUnexpectedEOF
			}
			return 0, err
		}
		if err != nil && err != io.EOF {
			return 0, err
		}
		if i := bytes.LastIndex(data, []byte(marker)); i >= 0 {
			return start + int64(i), nil
		}
		end = start
	}
	return -1, nil
}
