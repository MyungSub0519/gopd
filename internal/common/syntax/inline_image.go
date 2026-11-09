package syntax

import (
	"github.com/MyungSub0519/gopd/internal/common/pdfmodel"
)

const (
	// maxInlineImageEntries bounds the BI dictionary, which the format keeps
	// small; real producers write fewer than a dozen keys.
	maxInlineImageEntries = 64
	// inlineImageLookaheadTokens and inlineImageLookaheadBytes bound the
	// syntax check after a candidate EI, so that a payload full of decoys
	// costs linear rather than quadratic work.
	inlineImageLookaheadTokens = 8
	inlineImageLookaheadBytes  = 512
	// maxInlineImageCandidates bounds how many plausible EI keywords a
	// search may reject before giving up on the image.
	maxInlineImageCandidates = 64
)

// InlineImage reads the dictionary, payload and EI that follow a BI operator
// just returned by Next, and leaves the scanner after EI.
//
// The result describes the image as a Stream: StartKeyword is the ID keyword,
// DictionarySpan covers the bytes between BI and ID, and Encoded is the raw
// payload, which is located but not decoded. Values are charged against
// remainingValues as in Next. Errors terminate the scanner.
//
// The payload end is taken from /L when present, otherwise computed from the
// dimensions of an unfiltered image, otherwise found by searching for an EI
// keyword followed by well-formed syntax. Only /L reports StreamFromLength; a
// computed or searched boundary is StreamRecovered.
func (s *ContentScanner) InlineImage(remainingValues int) (pdfmodel.Stream, int, error) {
	if s.err != nil {
		return pdfmodel.Stream{}, 0, s.err
	}
	p := &s.parser
	if remainingValues < 0 {
		s.err = p.scanner.errorAt(p.scanner.pos, "remaining value budget must not be negative")
		return pdfmodel.Stream{}, 0, s.err
	}
	p.objects = 0
	p.maxValues = min(s.maxValues, remainingValues)
	stream, err := s.inlineImage()
	s.err = err
	return stream, p.objects, err
}

func (s *ContentScanner) inlineImage() (pdfmodel.Stream, error) {
	p := &s.parser
	sc := &p.scanner
	dictStart := sc.pos
	dict := pdfmodel.Dictionary{Entries: make([]pdfmodel.DictionaryEntry, 0, 8)}
	var id pdfmodel.Token
	for {
		key, err := sc.nextNonTrivia()
		if err != nil {
			return pdfmodel.Stream{}, err
		}
		if key.Kind == pdfmodel.TokenEOF {
			return pdfmodel.Stream{}, sc.errorAt(sc.pos, "inline image ends before ID")
		}
		if key.Kind == pdfmodel.TokenKeyword && string(p.bytes(key)) == "ID" {
			id = key
			break
		}
		if key.Kind != pdfmodel.TokenName {
			return pdfmodel.Stream{}, sc.errorAt(int(key.Span.Start-sc.offset), "expected inline image name key or ID")
		}
		if len(dict.Entries) >= maxInlineImageEntries {
			return pdfmodel.Stream{}, sc.limitAt(int(key.Span.Start-sc.offset), "inline image dictionary entry limit exceeded")
		}
		value, err := p.parse(1)
		if err != nil {
			return pdfmodel.Stream{}, err
		}
		dict.Entries = append(dict.Entries, pdfmodel.DictionaryEntry{Key: decodeName(p.bytes(key)), KeySpan: key.Span, Value: value})
	}

	// A single whitespace byte separates ID from the payload, so the next
	// byte is already image data. CRLF is tolerated because producers emit it.
	data := sc.data
	start := int(id.Span.End - sc.offset)
	if start >= len(data) || !isPDFWhitespace(data[start]) {
		return pdfmodel.Stream{}, sc.errorAt(start, "inline image ID is not followed by whitespace")
	}
	if data[start] == '\r' && start+1 < len(data) && data[start+1] == '\n' {
		start++
	}
	start++

	end, after, boundary, err := s.inlineImageEnd(dict, start)
	if err != nil {
		return pdfmodel.Stream{}, err
	}
	sc.pos = after
	span := func(from, to int) pdfmodel.Span {
		return pdfmodel.Span{Source: sc.source, Start: sc.offset + int64(from), End: sc.offset + int64(to)}
	}
	encoded, ei := span(start, end), span(after-2, after)
	return pdfmodel.Stream{
		Dictionary:     dict,
		DictionarySpan: span(dictStart, int(id.Span.Start-sc.offset)),
		StartKeyword:   id.Span,
		DataStart:      pdfmodel.Position{Source: sc.source, Offset: encoded.Start},
		Encoded:        &encoded,
		EndKeyword:     &ei,
		Boundary:       boundary,
	}, nil
}

// inlineImageEnd returns the payload end, the index just past EI, and how the
// boundary was established, for a payload beginning at start.
func (s *ContentScanner) inlineImageEnd(dict pdfmodel.Dictionary, start int) (int, int, pdfmodel.StreamBoundary, error) {
	sc := &s.parser.scanner
	data := sc.data
	known := func(length int64, what string, boundary pdfmodel.StreamBoundary) (int, int, pdfmodel.StreamBoundary, error) {
		if length < 0 || length > int64(len(data)-start) {
			return 0, 0, 0, sc.errorAt(start, "inline image "+what+" exceeds the content stream")
		}
		end := int(length) + start
		after := end
		for after < len(data) && isPDFWhitespace(data[after]) {
			after++
		}
		if !eiAt(data, after) {
			return 0, 0, 0, sc.errorAt(end, "inline image "+what+" does not end at EI")
		}
		return end, after + 2, boundary, nil
	}
	if length, ok := inlineInt(dict, "L", "Length"); ok {
		return known(length, "/L", pdfmodel.StreamFromLength)
	}
	if !inlineFiltered(dict) {
		if size, ok := inlineImageSize(dict); ok {
			return known(size, "size", pdfmodel.StreamRecovered)
		}
	}

	// The payload is opaque, so a candidate EI must also be followed by a
	// few tokens of clean syntax; random image bytes rarely manage that.
	candidates := 0
	for i := start; i+1 < len(data); i++ {
		if data[i] != 'E' || !eiAt(data, i) || (i > start && !isPDFWhitespace(data[i-1])) {
			continue
		}
		if scansCleanly(data[i+2:]) {
			// The whitespace introducing EI separates it from the data. An
			// image whose last byte is whitespace needs /L to say so.
			end := i
			if end > start {
				end--
			}
			return end, i + 2, pdfmodel.StreamRecovered, nil
		}
		if candidates++; candidates >= maxInlineImageCandidates {
			return 0, 0, 0, sc.limitAt(i, "inline image EI candidate limit exceeded")
		}
	}
	return 0, 0, 0, sc.errorAt(start, "inline image has no EI terminator")
}

// eiAt reports whether an EI keyword begins at i and is followed by
// whitespace, a delimiter or the end of the data.
func eiAt(data []byte, i int) bool {
	if i+2 > len(data) || data[i] != 'E' || data[i+1] != 'I' {
		return false
	}
	return i+2 == len(data) || isPDFWhitespace(data[i+2]) || isPDFDelimiter(data[i+2])
}

// scansCleanly reports whether rest begins with a few well-formed tokens. Only
// a bounded window is examined; a token cut off by the window edge is
// inconclusive and accepted.
func scansCleanly(rest []byte) bool {
	window := rest[:min(len(rest), inlineImageLookaheadBytes)]
	look, err := newSyntaxScanner(window, 0, 0, defaultSyntaxTokenBytes)
	if err != nil {
		return false
	}
	for range inlineImageLookaheadTokens {
		token, err := look.nextNonTrivia()
		if err != nil {
			return len(window) < len(rest) && look.pos == len(window)
		}
		if token.Kind == pdfmodel.TokenEOF {
			return true
		}
	}
	return true
}

// InlineImageEntry returns an inline image dictionary entry under its
// abbreviated or full key; producers use either spelling.
func InlineImageEntry(dict pdfmodel.Dictionary, short, full pdfmodel.Name) (pdfmodel.Object, bool) {
	for _, key := range [2]pdfmodel.Name{short, full} {
		if object, err := dict.Get(key); err == nil {
			return object, true
		}
	}
	return pdfmodel.Object{}, false
}

func inlineInt(dict pdfmodel.Dictionary, short, full pdfmodel.Name) (int64, bool) {
	object, ok := InlineImageEntry(dict, short, full)
	if !ok {
		return 0, false
	}
	n, err := pdfmodel.Int(object)
	return n, err == nil
}

func inlineFiltered(dict pdfmodel.Dictionary) bool {
	object, ok := InlineImageEntry(dict, "F", "Filter")
	if !ok {
		return false
	}
	switch value := object.Value.(type) {
	case pdfmodel.Null:
		return false
	case pdfmodel.Array:
		return len(value.Items) > 0
	}
	return true
}

// inlineImageSize returns the payload length of an unfiltered image, whose
// rows are padded to whole bytes. It reports false when the dictionary alone
// cannot settle the size, such as for a named color space.
func inlineImageSize(dict pdfmodel.Dictionary) (int64, bool) {
	width, okWidth := inlineInt(dict, "W", "Width")
	height, okHeight := inlineInt(dict, "H", "Height")
	if !okWidth || !okHeight || width <= 0 || height <= 0 || width > 1<<20 || height > 1<<20 {
		return 0, false
	}
	bits, components := int64(1), int64(1)
	mask, _ := InlineImageEntry(dict, "IM", "ImageMask")
	if isMask, _ := mask.Value.(pdfmodel.Boolean); !isMask {
		bits = 8
		if n, ok := inlineInt(dict, "BPC", "BitsPerComponent"); ok {
			bits = n
		}
		if bits != 1 && bits != 2 && bits != 4 && bits != 8 && bits != 16 {
			return 0, false
		}
		if space, ok := InlineImageEntry(dict, "CS", "ColorSpace"); ok {
			if components = inlineComponents(space); components == 0 {
				return 0, false
			}
		}
	}
	return (width*components*bits + 7) / 8 * height, true
}

// inlineComponents returns the components per sample of a device or indexed
// color space, or zero when the dictionary alone cannot tell.
func inlineComponents(space pdfmodel.Object) int64 {
	name, _ := space.Value.(pdfmodel.Name)
	if array, ok := space.Value.(pdfmodel.Array); ok && len(array.Items) > 0 {
		// An indexed space stores one index per sample whatever its base.
		if family, _ := array.Items[0].Value.(pdfmodel.Name); family == "I" || family == "Indexed" {
			return 1
		}
	}
	switch name {
	case "G", "DeviceGray", "CalGray", "I", "Indexed":
		return 1
	case "RGB", "DeviceRGB", "CalRGB":
		return 3
	case "CMYK", "DeviceCMYK":
		return 4
	}
	return 0
}
