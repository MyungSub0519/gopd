package document

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"

	"github.com/MyungSub0519/gopd/internal/pdfmodel"
)

func (d *Document) readHeaderAndXRefs() error {
	header := bytes.Index(d.data[:min(len(d.data), 1024)], []byte("%PDF-"))
	if header < 0 || header+8 > len(d.data) {
		return errors.New("missing PDF header")
	}
	version := string(d.data[header+5 : header+8])
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
	start := bytes.LastIndex(d.data, []byte("startxref"))
	if start < 0 {
		return errors.New("missing startxref")
	}
	pos := start + len("startxref")
	offset, raw, err := docUint(d.data, &pos, 63)
	if err != nil {
		return err
	}
	rawEnd := pos
	// Unlike ordinary comments, %%EOF is a file-tail marker in this context.
	for pos < len(d.data) && docSpace(d.data[pos]) {
		pos++
	}
	if !bytes.HasPrefix(d.data[pos:], []byte("%%EOF")) {
		return fmt.Errorf("missing EOF marker after startxref at byte %d", start)
	}
	tail := pdfmodel.FileTail{StartXRef: pdfmodel.Span{Source: 1, Start: int64(start), End: int64(start + 9)}, Offset: int64(offset), OffsetRaw: pdfmodel.Span{Source: 1, Start: int64(raw), End: int64(rawEnd)}, EOFMarker: pdfmodel.Span{Source: 1, Start: int64(pos), End: int64(pos + 5)}}
	d.Structure.Tails = append(d.Structure.Tails, tail)
	if err := d.markRegion(pdfmodel.RegionFileTail, pdfmodel.Span{Source: 1, Start: int64(start), End: int64(pos + 5)}); err != nil {
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
	if offset < 0 || offset >= int64(len(d.data)) {
		return pdfmodel.XRefSection{}, errors.New("xref offset outside file")
	}
	pos := int(offset)
	if bytes.HasPrefix(d.data[pos:], []byte("xref")) && pos+4 < len(d.data) && docDelimiter(d.data[pos+4]) {
		return d.readXRefTable(pos)
	}
	return d.readXRefStream(pos)
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
		if err != nil || n < 0 || n >= int64(len(d.data)) {
			return fmt.Errorf("invalid /%s xref offset", link.key)
		}
		*link.out = &n
	}
	return nil
}

func (d *Document) readXRefTable(start int) (pdfmodel.XRefSection, error) {
	section := pdfmodel.XRefSection{Form: pdfmodel.XRefTable, Offset: int64(start)}
	pos := start + 4
	total := 0
	for {
		word, begin, err := docWord(d.data, &pos)
		if err != nil {
			return section, err
		}
		if word == "trailer" {
			obj, n, err := d.parseObject(d.data[pos:], 1, int64(pos))
			if err != nil {
				return section, err
			}
			pos += n
			section.Trailer = obj
			section.Span = pdfmodel.Span{Source: 1, Start: int64(start), End: int64(pos)}
			if err := d.markRegion(pdfmodel.RegionXRef, pdfmodel.Span{Source: 1, Start: int64(start), End: int64(begin)}); err != nil {
				return section, err
			}
			if err := d.markRegion(pdfmodel.RegionTrailer, pdfmodel.Span{Source: 1, Start: int64(begin), End: int64(pos)}); err != nil {
				return section, err
			}
			return section, d.xrefLinks(&section)
		}
		first, err := strconv.ParseUint(word, 10, 32)
		if err != nil {
			return section, fmt.Errorf("invalid xref subsection at byte %d", begin)
		}
		count, _, err := docUint(d.data, &pos, 32)
		if err != nil {
			return section, err
		}
		if count > uint64(d.Options.Limits.MaxObjects-total) {
			return section, fmt.Errorf("xref subsection exceeds object limit: %w", pdfmodel.ErrLimit)
		}
		if first+count > 1<<32 || count > uint64(len(d.data)-pos)/5 {
			return section, errors.New("xref subsection exceeds input or object number range")
		}
		if err := d.reserveXRefRange(count); err != nil {
			return section, err
		}
		header := pdfmodel.Span{Source: 1, Start: int64(begin), End: int64(pos)}
		r := pdfmodel.XRefRange{First: uint32(first), Count: uint32(count), Header: &header}
		for i := uint64(0); i < count; i++ {
			off, recordStart, e := docUint(d.data, &pos, 63)
			if e != nil {
				return section, e
			}
			gen, _, e := docUint(d.data, &pos, 16)
			if e != nil {
				return section, e
			}
			flag, _, e := docWord(d.data, &pos)
			if e != nil {
				return section, e
			}
			record := pdfmodel.XRefRecord{Number: uint32(first + i), Span: pdfmodel.Span{Source: 1, Start: int64(recordStart), End: int64(pos)}}
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

func (d *Document) readXRefStream(start int) (pdfmodel.XRefSection, error) {
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
				if fields[1] > uint64(len(d.data)) || fields[2] > 65535 {
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
