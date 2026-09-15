package gopd

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

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
	font := Font{Object: object, ID: semID(resource), widths: make(map[uint32]float64)}
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
	if font.Subtype == "Type3" {
		b.diag("unsupported-type3-font", "Type3 glyph content and font-matrix positioning are retained without execution", object.Span)
	}
	metricDict := dict
	if font.composite {
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
		encoding, ok := font.Encoding.Value.(Name)
		font.vertical = ok && strings.HasSuffix(string(encoding), "-V")
		font.PositioningSupported = encoding == "Identity-H"
		if !ok || (encoding != "Identity-H" && encoding != "Identity-V") {
			b.diag("unsupported-font-encoding", "Composite font encoding is not Identity-H/Identity-V; code widths and positioning may be incomplete", font.Encoding.Span)
		}
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
	} else {
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
	if descriptor, ok, e := b.get(metricDict, "FontDescriptor"); e != nil {
		return -1, e
	} else if ok {
		dd, e := semDictionary(descriptor)
		if e != nil {
			return -1, e
		}
		for _, name := range []Name{"FontFile", "FontFile2", "FontFile3"} {
			if stream, exists, e := b.get(dd, name); e != nil {
				return -1, e
			} else if exists {
				font.Embedded = &stream
				break
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
				b.diag("unsupported-tounicode", e.Error(), value.Span)
			} else {
				data, e := b.doc.Bytes(Span{source.ID, 0, source.Size})
				if e != nil {
					return -1, e
				}
				font.ToUnicode, e = parseToUnicode(data)
				if e != nil {
					b.diag("unsupported-tounicode", e.Error(), value.Span)
				}
				if font.ToUnicode != nil {
					if len(font.ToUnicode.Mappings) > b.maxObjects-b.cmapEntries {
						return -1, fmt.Errorf("ToUnicode expanded mapping limit at %+v", value.Span)
					}
					b.cmapEntries += len(font.ToUnicode.Mappings)
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

func (b *semanticBuilder) cidWidths(font *Font, object Object) error {
	array, ok := object.Value.(Array)
	if !ok {
		return fmt.Errorf("invalid CID Widths at %+v", object.Span)
	}
	for i := 0; i < len(array.Items); {
		start, e := Int(array.Items[i])
		if e != nil || start < 0 || start > 65535 {
			return fmt.Errorf("invalid CID width range")
		}
		i++
		if i >= len(array.Items) {
			return fmt.Errorf("truncated CID width range")
		}
		if widths, ok := array.Items[i].Value.(Array); ok {
			if start+int64(len(widths.Items)) > 65536 {
				return fmt.Errorf("CID width range exceeds limit")
			}
			if e = b.reserveWidths(len(widths.Items)); e != nil {
				return e
			}
			for j, item := range widths.Items {
				width, e := Number(item)
				if e != nil {
					return e
				}
				font.widths[uint32(start)+uint32(j)] = width
			}
			i++
		} else {
			end, e := Int(array.Items[i])
			if e != nil || end < start || end > 65535 {
				return fmt.Errorf("invalid CID width range")
			}
			i++
			if i >= len(array.Items) {
				return fmt.Errorf("missing CID width")
			}
			width, e := Number(array.Items[i])
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
		return fmt.Errorf("font width expansion limit exceeded")
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
		font.simpleEncoding = make(map[byte]string)
		for code := 32; code <= 126; code++ {
			font.simpleEncoding[byte(code)] = string(rune(code))
		}
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
	// Standard 14 Latin fonts have a defined StandardEncoding when omitted.
	if name == "" && font.Encoding.Value == nil && font.Subtype == "Type1" {
		base := string(font.BaseFont)
		if strings.HasPrefix(base, "Helvetica") || strings.HasPrefix(base, "Times-") || strings.HasPrefix(base, "Courier") {
			font.simpleEncoding = make(map[byte]string)
			for code := 32; code <= 126; code++ {
				font.simpleEncoding[byte(code)] = string(rune(code))
			}
		}
	}
	if name == "StandardEncoding" || (name == "" && font.Encoding.Value == nil && font.simpleEncoding != nil) {
		font.simpleEncoding[39] = "’"
		font.simpleEncoding[96] = "‘"
	}
	code := -1
	for _, item := range differences.Items {
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
	if f.ToUnicode != nil {
		return f.ToUnicode.decodeBounded(raw, limit)
	}
	var result strings.Builder
	var codes []decodedCode
	complete := true
	for at := 0; at < len(raw); {
		size := 1
		if f.composite && at+2 <= len(raw) {
			size = 2
		}
		code := append([]byte(nil), raw[at:at+size]...)
		value, ok := f.simpleEncoding[code[0]]
		if f.composite {
			ok = false
		}
		if !ok {
			value = "\uFFFD"
			complete = false
		}
		if int64(len(value)) > limit-int64(result.Len()) {
			return "", nil, false, fmt.Errorf("expanded Unicode text byte limit exceeded")
		}
		result.WriteString(value)
		codes = append(codes, decodedCode{code, value, ok})
		at += size
	}
	return result.String(), codes, complete, nil
}
