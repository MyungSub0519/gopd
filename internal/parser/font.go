package parser

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/MyungSub0519/gopd/internal/common/pdfmodel"
	"github.com/MyungSub0519/gopd/internal/common/syntax"
)

// FontInfo contains display metadata, shared by original font resource identity.
// FontSize belongs to Text because the same font can be used at different sizes.
type FontInfo struct {
	BaseFont Name
	Subtype  Name
}

type Font struct {
	Object               Object
	ID                   ObjectID
	Subtype              Name
	BaseFont             Name
	Encoding             Object
	ToUnicode            *CMap
	Embedded             *Object
	DecodeSupported      bool
	WidthsKnown          bool
	PositioningSupported bool
	widths               map[uint32]float64
	defaultWidth         float64
	defaultWidthKnown    bool
	simpleEncoding       map[byte]string
	composite            bool
	vertical             bool
	widthScale           float64
}

// effectiveWidthScale returns widthScale, defaulting to 0.001 when unset so
// the zero Font and non-Type3 fonts keep the historical advance formula.
func (font *Font) effectiveWidthScale() float64 {
	if font.widthScale == 0 {
		return 0.001
	}
	return font.widthScale
}

func (b *semanticBuilder) font(resource Object) (int, error) {
	object, err := b.doc.ResolveObject(resource)
	if err != nil {
		return -1, err
	}
	key := object.Span
	if index, ok := b.fonts[key]; ok {
		return index, nil
	}
	dict, err := semDictionary(object)
	if err != nil {
		return -1, err
	}
	font := Font{Object: object, ID: semID(resource)}
	if b.wantPositions() {
		font.widths = make(map[uint32]float64)
	}
	font.Subtype, err = b.name(dict, "Subtype")
	if err != nil {
		return -1, err
	}
	font.BaseFont, err = b.name(dict, "BaseFont")
	if err != nil {
		return -1, err
	}
	font.Encoding, _, err = b.get(dict, "Encoding")
	if err != nil {
		return -1, err
	}
	font.composite = font.Subtype == "Type0"
	font.PositioningSupported = font.Subtype == "Type1" || font.Subtype == "MMType1" || font.Subtype == "TrueType"
	if font.Subtype == "Type3" && b.wantPositions() {
		// Glyph content is still not executed; positioning relies only on
		// the font dictionary metrics (Widths plus FontMatrix scale).
		if err := b.diag("unsupported-type3-font", "Type3 glyph content is retained without execution; positioning uses font dictionary metrics", object.Span); err != nil {
			return -1, err
		}
		if font.widthScale, err = b.type3WidthScale(dict); err != nil {
			return -1, err
		}
	}
	metricDict := dict
	if font.composite {
		// Unicode decoding uses the parent Encoding and ToUnicode; the
		// descendant is needed only for metrics.
		if b.wantPositions() {
			descendants, ok, e := b.get(dict, "DescendantFonts")
			if e != nil {
				return -1, e
			}
			if !ok {
				return -1, fmt.Errorf("Type0 font missing DescendantFonts at %+v", object.Span)
			}
			array, ok := descendants.Value.(Array)
			if !ok || len(array.Items) != 1 {
				return -1, fmt.Errorf("invalid DescendantFonts at %+v", descendants.Span)
			}
			child, e := b.doc.ResolveObject(array.Items[0])
			if e != nil {
				return -1, e
			}
			metricDict, e = semDictionary(child)
			if e != nil {
				return -1, e
			}
		}
		encoding, ok := font.Encoding.Value.(Name)
		font.vertical = ok && strings.HasSuffix(string(encoding), "-V")
		font.PositioningSupported = encoding == "Identity-H"
		if b.wantPositions() && (!ok || (encoding != "Identity-H" && encoding != "Identity-V")) {
			if err := b.diag("unsupported-font-encoding", "Composite font encoding is not Identity-H/Identity-V; code widths and positioning may be incomplete", font.Encoding.Span); err != nil {
				return -1, err
			}
		}
		if b.wantPositions() {
			font.defaultWidth = 1000
			font.defaultWidthKnown = encoding == "Identity-H"
			if value, ok, e := b.get(metricDict, "DW"); e != nil {
				return -1, e
			} else if ok {
				font.defaultWidth, e = Number(value)
				if e != nil {
					return -1, e
				}
			}
			if value, ok, e := b.get(metricDict, "W"); e != nil {
				return -1, e
			} else if ok {
				if e = b.cidWidths(&font, value); e != nil {
					return -1, e
				}
			}
		}
	} else if b.wantPositions() {
		if first, ok, e := b.get(dict, "FirstChar"); e != nil {
			return -1, e
		} else if ok {
			start, e := Int(first)
			if e != nil || start < 0 || start > 255 {
				return -1, fmt.Errorf("invalid font FirstChar at %+v", first.Span)
			}
			widths, exists, e := b.get(dict, "Widths")
			if e != nil {
				return -1, e
			}
			if exists {
				array, ok := widths.Value.(Array)
				if !ok || int64(len(array.Items))+start > 256 {
					return -1, fmt.Errorf("invalid simple font Widths at %+v", widths.Span)
				}
				if e = b.reserveWidths(len(array.Items)); e != nil {
					return -1, e
				}
				for i, item := range array.Items {
					value, e := b.doc.ResolveObject(item)
					if e != nil {
						return -1, e
					}
					width, e := Number(value)
					if e != nil {
						return -1, e
					}
					font.widths[uint32(start)+uint32(i)] = width
				}
			}
		}
	}
	if font.Subtype == "Type3" && b.wantPositions() {
		// Glyph content stays unexecuted, but the d0/d1 width operands at the
		// head of each charproc are the authoritative advance metrics, so they
		// are read without interpreting the glyph body.
		if err := b.type3CharProcWidths(&font); err != nil {
			return -1, err
		}
		// Dictionary widths are glyph-space; normalize them to the 1000-unit
		// convention so the advance formula stays uniform across subtypes.
		scale := font.effectiveWidthScale()
		if scale != 0.001 {
			for code, width := range font.widths {
				font.widths[code] = width * scale * 1000
			}
			if font.defaultWidthKnown {
				font.defaultWidth *= scale * 1000
			}
		}
		font.PositioningSupported = len(font.widths) > 0 || font.defaultWidthKnown
	}
	if b.wantPositions() {
		if descriptor, ok, e := b.get(metricDict, "FontDescriptor"); e != nil {
			return -1, e
		} else if ok {
			dd, e := semDictionary(descriptor)
			if e != nil {
				return -1, e
			}
			// Embedded bytes are retained only by the detailed API. Result
			// uses dictionary metrics and never executes the font program.
			if b.result == nil {
				for _, name := range []Name{"FontFile", "FontFile2", "FontFile3"} {
					if stream, exists, e := b.get(dd, name); e != nil {
						return -1, e
					} else if exists {
						font.Embedded = &stream
						break
					}
				}
			}
			if width, exists, e := b.get(dd, "MissingWidth"); e != nil {
				return -1, e
			} else if exists && !font.composite {
				font.defaultWidth, e = Number(width)
				if e != nil {
					return -1, e
				}
				font.defaultWidthKnown = true
			}
		}
	}
	if value, ok, e := b.get(dict, "ToUnicode"); e != nil {
		return -1, e
	} else if ok {
		stream, ok := value.Value.(Stream)
		if !ok {
			return -1, fmt.Errorf("ToUnicode is not a stream at %+v", value.Span)
		}
		if cached, exists := b.cmaps[value.Span]; exists {
			font.ToUnicode = cached
		} else {
			source, e := b.doc.DecodeStream(stream)
			if e != nil {
				if errors.Is(e, ErrLimit) {
					return -1, e
				}
				if err := b.diag("unsupported-tounicode", e.Error(), value.Span); err != nil {
					return -1, err
				}
			} else {
				data, e := b.doc.Bytes(Span{Source: source.ID, Start: 0, End: source.Size})
				if e != nil {
					return -1, e
				}
				font.ToUnicode, e = parseToUnicodeBounded(data, b.maxObjects-b.cmapEntries, b.maxUnicodeBytes-b.unicodeBytes)
				if e != nil {
					if errors.Is(e, ErrLimit) {
						return -1, fmt.Errorf("ToUnicode at %+v: %w", value.Span, e)
					}
					if err := b.diag("unsupported-tounicode", e.Error(), value.Span); err != nil {
						return -1, err
					}
				}
				if font.ToUnicode != nil {
					b.cmapEntries += len(font.ToUnicode.Mappings)
					// Shared maps are charged once, alongside emitted UTF-8 text.
					b.unicodeBytes += font.ToUnicode.mappingBytes()
					b.cmaps[value.Span] = font.ToUnicode
				}
			}
		}
	}
	if !font.composite {
		if e := b.simpleFontEncoding(&font); e != nil {
			return -1, e
		}
	}
	font.DecodeSupported = font.ToUnicode != nil || len(font.simpleEncoding) > 0
	font.WidthsKnown = len(font.widths) > 0 || font.defaultWidthKnown
	index := len(b.pdf.Fonts)
	b.pdf.Fonts = append(b.pdf.Fonts, font)
	b.fonts[key] = index
	return index, nil
}

// type3WidthScale returns the FontMatrix x-scale used to normalize Type3
// glyph-space widths to the 1000-unit convention. A missing or zero-scale
// matrix falls back to 0.001.
func (b *semanticBuilder) type3WidthScale(dict Dictionary) (float64, error) {
	value, ok, err := b.get(dict, "FontMatrix")
	if err != nil || !ok {
		return 0.001, err
	}
	array, isArray := value.Value.(Array)
	if !isArray || len(array.Items) != 6 {
		return 0, fmt.Errorf("invalid Type3 FontMatrix at %+v", value.Span)
	}
	first, err := b.doc.ResolveObject(array.Items[0])
	if err != nil {
		return 0, err
	}
	scale, err := Number(first)
	if err != nil {
		return 0, fmt.Errorf("invalid Type3 FontMatrix at %+v: %w", value.Span, err)
	}
	if scale == 0 {
		return 0.001, nil
	}
	return scale, nil
}

// type3CharProcWidths overrides dictionary widths with the authoritative
// d0/d1 glyph widths declared inside each charproc referenced by the font
// Encoding Differences.
func (b *semanticBuilder) type3CharProcWidths(font *Font) error {
	dict, err := semDictionary(font.Object)
	if err != nil {
		return err
	}
	encDict, ok := font.Encoding.Value.(Dictionary)
	if !ok {
		return nil
	}
	diff, exists, err := b.get(encDict, "Differences")
	if err != nil || !exists {
		return err
	}
	array, isArray := diff.Value.(Array)
	if !isArray {
		return fmt.Errorf("invalid Encoding Differences")
	}
	code := -1
	for _, item := range array.Items {
		resolved, err := b.doc.ResolveObject(item)
		if err != nil {
			return err
		}
		if _, ok := resolved.Value.(Integer); ok {
			n, err := Int(resolved)
			if err != nil || n < 0 || n > 255 {
				return fmt.Errorf("invalid encoding difference code")
			}
			code = int(n)
			continue
		}
		glyph, ok := resolved.Value.(Name)
		if !ok || code < 0 || code > 255 {
			return fmt.Errorf("invalid Encoding Differences sequence")
		}
		width, found, err := b.charProcWidth(dict, string(glyph))
		if err != nil {
			return err
		}
		if found {
			font.widths[uint32(code)] = width
		}
		code++
	}
	return nil
}

// charProcWidth decodes one Type3 charproc and returns the wx operand of its
// leading d0/d1 operator, in glyph space units.
func (b *semanticBuilder) charProcWidth(fontDict Dictionary, glyph string) (float64, bool, error) {
	procsRef, ok, err := b.get(fontDict, "CharProcs")
	if err != nil || !ok {
		return 0, false, err
	}
	procsResolved, err := b.doc.ResolveObject(procsRef)
	if err != nil {
		return 0, false, err
	}
	procs, err := semDictionary(procsResolved)
	if err != nil {
		return 0, false, err
	}
	procRef, ok, err := b.get(procs, Name(glyph))
	if err != nil || !ok {
		return 0, false, err
	}
	proc, err := b.doc.ResolveObject(procRef)
	if err != nil {
		return 0, false, err
	}
	stream, ok := proc.Value.(Stream)
	if !ok {
		return 0, false, fmt.Errorf("Type3 charproc %s is not a stream", glyph)
	}
	source, err := b.doc.DecodeStream(stream)
	if err != nil {
		if errors.Is(err, ErrLimit) {
			return 0, false, err
		}
		// An unreadable glyph body keeps the dictionary width.
		return 0, false, nil
	}
	data, err := b.doc.Bytes(pdfmodel.Span{Source: source.ID, Start: 0, End: source.Size})
	if err != nil {
		return 0, false, err
	}
	scanner, err := syntax.NewContentScanner(data, source.ID, 0, b.doc.Options.Limits)
	if err != nil {
		return 0, false, err
	}
	var operands []pdfmodel.Object
	for {
		object, operator, values, err := scanner.Next(b.maxValues - b.semanticValues)
		b.semanticValues += values
		if errors.Is(err, io.EOF) {
			return 0, false, nil
		}
		if err != nil {
			return 0, false, err
		}
		if !operator {
			operands = append(operands, object)
			if len(operands) > 256 {
				return 0, false, fmt.Errorf("%w: Type3 charproc operand limit", ErrLimit)
			}
			continue
		}
		name := string(object.Value.(Name))
		if name == "d0" || name == "d1" {
			if len(operands) == 0 {
				return 0, false, fmt.Errorf("Type3 charproc %s: %s without width", glyph, name)
			}
			width, err := Number(operands[0])
			if err != nil {
				return 0, false, err
			}
			return width, true, nil
		}
		operands = operands[:0]
	}
}

func (b *semanticBuilder) cidWidths(font *Font, object Object) error {
	array, ok := object.Value.(Array)
	if !ok {
		return fmt.Errorf("invalid CID Widths at %+v", object.Span)
	}
	for i := 0; i < len(array.Items); {
		first, e := b.doc.ResolveObject(array.Items[i])
		if e != nil {
			return e
		}
		start, e := Int(first)
		if e != nil || start < 0 || start > 65535 {
			return fmt.Errorf("invalid CID width range")
		}
		i++
		if i >= len(array.Items) {
			return fmt.Errorf("truncated CID width range")
		}
		next, e := b.doc.ResolveObject(array.Items[i])
		if e != nil {
			return e
		}
		if widths, ok := next.Value.(Array); ok {
			if start+int64(len(widths.Items)) > 65536 {
				return fmt.Errorf("invalid CID width range")
			}
			if e = b.reserveWidths(len(widths.Items)); e != nil {
				return e
			}
			for j, item := range widths.Items {
				value, e := b.doc.ResolveObject(item)
				if e != nil {
					return e
				}
				width, e := Number(value)
				if e != nil {
					return e
				}
				font.widths[uint32(start)+uint32(j)] = width
			}
			i++
		} else {
			end, e := Int(next)
			if e != nil || end < start || end > 65535 {
				return fmt.Errorf("invalid CID width range")
			}
			i++
			if i >= len(array.Items) {
				return fmt.Errorf("missing CID width")
			}
			value, e := b.doc.ResolveObject(array.Items[i])
			if e != nil {
				return e
			}
			width, e := Number(value)
			if e != nil {
				return e
			}
			i++
			if e = b.reserveWidths(int(end - start + 1)); e != nil {
				return e
			}
			for code := start; code <= end; code++ {
				font.widths[uint32(code)] = width
			}
		}
	}
	return nil
}

func (b *semanticBuilder) reserveWidths(count int) error {
	if count > b.maxObjects-b.widthEntries {
		return fmt.Errorf("%w: font width expansion limit exceeded", ErrLimit)
	}
	b.widthEntries += count
	return nil
}

func (b *semanticBuilder) simpleFontEncoding(font *Font) error {
	encoding := font.Encoding
	var differences Array
	if dict, ok := encoding.Value.(Dictionary); ok {
		var e error
		encoding, _, e = b.get(dict, "BaseEncoding")
		if e != nil {
			return e
		}
		if diff, exists, e := b.get(dict, "Differences"); e != nil {
			return e
		} else if exists {
			var ok bool
			differences, ok = diff.Value.(Array)
			if !ok {
				return fmt.Errorf("invalid Encoding Differences")
			}
		}
	}
	name, _ := encoding.Value.(Name)
	if name == "WinAnsiEncoding" || name == "StandardEncoding" {
		font.simpleEncoding = asciiEncoding()
	}
	if name == "WinAnsiEncoding" {
		for code := 160; code <= 255; code++ {
			font.simpleEncoding[byte(code)] = string(rune(code))
		}
		specials := map[byte]rune{128: '€', 130: '‚', 131: 'ƒ', 132: '„', 133: '…', 134: '†', 135: '‡', 136: 'ˆ', 137: '‰', 138: 'Š', 139: '‹', 140: 'Œ', 142: 'Ž', 145: '‘', 146: '’', 147: '“', 148: '”', 149: '•', 150: '–', 151: '—', 152: '˜', 153: '™', 154: 'š', 155: '›', 156: 'œ', 158: 'ž', 159: 'Ÿ'}
		for code, value := range specials {
			font.simpleEncoding[code] = string(value)
		}
		font.simpleEncoding[160] = " "
		font.simpleEncoding[173] = "-"
		for _, code := range []byte{127, 129, 141, 143, 144, 157} {
			font.simpleEncoding[code] = "•"
		}
	}
	// The 12 Latin members of the standard 14 use StandardEncoding when omitted.
	if name == "" && encoding.Value == nil && font.Subtype == "Type1" {
		switch font.BaseFont {
		case "Helvetica", "Helvetica-Bold", "Helvetica-Oblique", "Helvetica-BoldOblique",
			"Times-Roman", "Times-Bold", "Times-Italic", "Times-BoldItalic",
			"Courier", "Courier-Bold", "Courier-Oblique", "Courier-BoldOblique":
			font.simpleEncoding = asciiEncoding()
		}
	}
	if name == "StandardEncoding" || (name == "" && encoding.Value == nil && font.simpleEncoding != nil) {
		font.simpleEncoding[39] = "’"
		font.simpleEncoding[96] = "‘"
	}
	code := -1
	for _, item := range differences.Items {
		item, err := b.doc.ResolveObject(item)
		if err != nil {
			return err
		}
		if _, ok := item.Value.(Integer); ok {
			n, e := Int(item)
			if e != nil || n < 0 || n > 255 {
				return fmt.Errorf("invalid encoding difference code")
			}
			code = int(n)
			continue
		}
		glyph, ok := item.Value.(Name)
		if !ok || code < 0 || code > 255 {
			return fmt.Errorf("invalid Encoding Differences sequence")
		}
		if font.simpleEncoding == nil {
			font.simpleEncoding = make(map[byte]string)
		}
		value, ok := glyphUnicode(string(glyph))
		if ok {
			font.simpleEncoding[byte(code)] = value
		} else {
			delete(font.simpleEncoding, byte(code))
		}
		code++
	}
	return nil
}

func asciiEncoding() map[byte]string {
	encoding := make(map[byte]string)
	for code := 32; code <= 126; code++ {
		encoding[byte(code)] = string(rune(code))
	}
	return encoding
}

func glyphUnicode(name string) (string, bool) {
	if len(name) == 1 && name[0] >= 33 && name[0] <= 126 {
		return name, true
	}
	known := map[string]string{"space": " ", "hyphen": "-", "period": ".", "comma": ",", "parenleft": "(", "parenright": ")", "colon": ":", "semicolon": ";", "slash": "/", "zero": "0", "one": "1", "two": "2", "three": "3", "four": "4", "five": "5", "six": "6", "seven": "7", "eight": "8", "nine": "9"}
	if value, ok := known[name]; ok {
		return value, true
	}
	if strings.HasPrefix(name, "uni") && len(name) == 7 {
		n, e := strconv.ParseUint(name[3:], 16, 16)
		if e == nil && utf8.ValidRune(rune(n)) {
			return string(rune(n)), true
		}
	}
	if strings.HasPrefix(name, "u") && len(name) >= 5 && len(name) <= 7 {
		n, e := strconv.ParseUint(name[1:], 16, 32)
		if e == nil && utf8.ValidRune(rune(n)) {
			return string(rune(n)), true
		}
	}
	return "", false
}

func (f *Font) decodeBounded(raw []byte, limit int64) (string, []decodedCode, bool, error) {
	return f.decodeSelected(raw, limit, true)
}

func (f *Font) decodeSelected(raw []byte, limit int64, collectCodes bool) (string, []decodedCode, bool, error) {
	if f.ToUnicode != nil {
		return f.ToUnicode.decodeSelected(raw, limit, collectCodes)
	}
	var result strings.Builder
	var codes []decodedCode
	complete := true
	for at := 0; at < len(raw); {
		size := 1
		if f.composite && at+2 <= len(raw) {
			size = 2
		}
		code := raw[at : at+size]
		value, ok := f.simpleEncoding[code[0]]
		if f.composite {
			ok = false
		}
		if !ok {
			value = "\uFFFD"
			complete = false
		}
		if int64(len(value)) > limit-int64(result.Len()) {
			return "", nil, false, fmt.Errorf("%w: expanded Unicode text byte limit exceeded", ErrLimit)
		}
		result.WriteString(value)
		if collectCodes {
			codes = append(codes, decodedCode{append([]byte(nil), code...), value, ok})
		}
		at += size
	}
	return result.String(), codes, complete, nil
}
