package content

import (
	"fmt"

	"github.com/MyungSub0519/gopd/internal/model"
	"github.com/MyungSub0519/gopd/internal/syntax"
)

// inlineImageLookahead is how many tokens after a candidate EI must scan
// cleanly before the candidate is believed. See findInlineImageEnd.
const inlineImageLookahead = 8

// inlineKey returns a dictionary entry under either its abbreviated or its full
// name.
//
// Inline images have their own abbreviated key names — /W for /Width, /BPC for
// /BitsPerComponent and so on — because the dictionary sits in the content
// stream and is written once per image. Both spellings are permitted, and
// producers mix them, so every lookup accepts either.
func inlineKey(dict model.Dictionary, short, full model.Name) (model.Object, bool) {
	for _, name := range [2]model.Name{short, full} {
		if object, err := dict.Get(name); err == nil {
			return object, true
		}
	}
	return model.Object{}, false
}

// inlineInt reads an integer entry under either spelling.
func inlineInt(dict model.Dictionary, short, full model.Name) (int64, bool) {
	object, ok := inlineKey(dict, short, full)
	if !ok {
		return 0, false
	}
	n, err := model.Int(object)
	if err != nil {
		return 0, false
	}
	return n, true
}

// inlineComponents returns the number of colour components per sample.
//
// Only the device spaces and Indexed can be settled from the dictionary alone.
// A named space refers to the page's resources and an unrecognised array could
// be anything, so those report false and the caller falls back to searching for
// EI rather than computing a length it cannot trust.
func inlineComponents(object model.Object) (int, bool) {
	switch value := object.Value.(type) {
	case model.Name:
		switch value {
		case "G", "DeviceGray", "CalGray":
			return 1, true
		case "RGB", "DeviceRGB", "CalRGB":
			return 3, true
		case "CMYK", "DeviceCMYK":
			return 4, true
		case "I", "Indexed":
			return 1, true
		}
	case model.Array:
		// An indexed space stores one index per sample whatever its base.
		if len(value.Items) > 0 {
			if name, ok := value.Items[0].Value.(model.Name); ok && (name == "I" || name == "Indexed") {
				return 1, true
			}
		}
	}
	return 0, false
}

// inlineIsMask reports whether the dictionary declares an image mask, which is
// one bit per sample regardless of what /BPC says.
func inlineIsMask(dict model.Dictionary) bool {
	mask, ok := inlineKey(dict, "IM", "ImageMask")
	if !ok {
		return false
	}
	value, isBool := mask.Value.(model.Boolean)
	return isBool && bool(value)
}

// inlineImageSize returns the exact payload length of an unfiltered image.
//
// Each row is padded up to a byte boundary, so the size is
// ceil(width * components * bitsPerComponent / 8) * height. Forgetting that
// padding is the classic way to misread a narrow one-bit image.
//
// It reports false whenever an input is missing, out of range, or in a colour
// space whose component count cannot be settled from the dictionary alone.
func inlineImageSize(dict model.Dictionary) (int, bool) {
	width, okWidth := inlineInt(dict, "W", "Width")
	height, okHeight := inlineInt(dict, "H", "Height")
	if !okWidth || !okHeight || width <= 0 || height <= 0 || width > 1<<20 || height > 1<<20 {
		return 0, false
	}
	bits, components := int64(1), int64(1)
	if !inlineIsMask(dict) {
		bits = 8
		if n, ok := inlineInt(dict, "BPC", "BitsPerComponent"); ok {
			bits = n
		}
		if bits != 1 && bits != 2 && bits != 4 && bits != 8 && bits != 16 {
			return 0, false
		}
		if space, ok := inlineKey(dict, "CS", "ColorSpace"); ok {
			n, known := inlineComponents(space)
			if !known {
				return 0, false
			}
			components = int64(n)
		}
	}
	size := (width*components*bits + 7) / 8 * height
	if size <= 0 || size > 1<<30 {
		return 0, false
	}
	return int(size), true
}

// inlineHasFilter reports whether the image is encoded, in which case its
// payload length cannot be computed from its dimensions.
func inlineHasFilter(dict model.Dictionary) bool {
	object, ok := inlineKey(dict, "F", "Filter")
	if !ok {
		return false
	}
	switch value := object.Value.(type) {
	case model.Null:
		return false
	case model.Array:
		return len(value.Items) > 0
	}
	return true
}

// eiAt reports whether a well-formed EI keyword begins at i, returning the
// index just past it.
//
// Both sides are checked. EI must be preceded by whitespace, since it is a
// keyword and not a suffix of something else, and it must be followed by
// whitespace, a delimiter or the end of the stream. Those two tests are what
// reject most of the EI byte pairs that occur by chance inside compressed
// image data.
func eiAt(data []byte, i int) (int, bool) {
	if i < 0 || i+2 > len(data) || data[i] != 'E' || data[i+1] != 'I' {
		return 0, false
	}
	if i > 0 && !syntax.IsWhitespace(data[i-1]) {
		return 0, false
	}
	end := i + 2
	if end < len(data) && !syntax.IsWhitespace(data[end]) && !syntax.IsDelimiter(data[end]) {
		return 0, false
	}
	return end, true
}

// scansCleanly reports whether the bytes from pos tokenise as PDF syntax for
// the next few tokens.
//
// It is the third test applied to a candidate EI. Image data that happens to
// contain a plausible-looking EI usually continues with bytes that are not
// syntax at all — an unbalanced parenthesis, a bad name escape — so trying to
// scan past the candidate rejects it. Only a bounded number of tokens is read,
// because the point is to sample what follows, not to validate the remainder.
func scansCleanly(data []byte, pos int, source model.SourceID, limits model.Limits) bool {
	scanner, err := syntax.NewScanner(data[pos:], source, int64(pos), limits)
	if err != nil {
		return false
	}
	for n := 0; n < inlineImageLookahead; n++ {
		token, err := scanner.NextNonTrivia()
		if err != nil {
			return false
		}
		if token.Kind == model.TokenEOF {
			return true
		}
	}
	return true
}

// expectEI consumes optional whitespace at pos and requires an EI keyword,
// which is how a payload whose length was already known is confirmed.
func expectEI(data []byte, pos int) (int, bool) {
	for pos < len(data) && syntax.IsWhitespace(data[pos]) {
		pos++
	}
	return eiAt(data, pos)
}

// findInlineImageEnd locates the end of an inline image payload that begins at
// start, returning the payload end, the index past EI, and how the boundary
// was established.
//
// Three strategies are tried in order of how much they can be trusted.
// A /L entry states the length outright, which PDF 2.0 added for exactly this
// reason but which most files do not carry. Failing that, an unfiltered
// image's length follows from its dimensions. Only when neither applies is the
// payload searched for a terminating EI, which is a heuristic: see eiAt and
// scansCleanly for the tests a candidate must pass.
func findInlineImageEnd(data []byte, start int, dict model.Dictionary, source model.SourceID, limits model.Limits) (int, int, model.StreamBoundary, error) {
	if length, ok := inlineInt(dict, "L", "Length"); ok {
		if length < 0 || start+int(length) > len(data) {
			return 0, 0, 0, fmt.Errorf("inline image /L lies outside the content stream")
		}
		end := start + int(length)
		after, ok := expectEI(data, end)
		if !ok {
			return 0, 0, 0, fmt.Errorf("inline image /L does not end at EI")
		}
		return end, after, model.StreamFromLength, nil
	}
	if !inlineHasFilter(dict) {
		if size, ok := inlineImageSize(dict); ok {
			if start+size > len(data) {
				return 0, 0, 0, fmt.Errorf("inline image data is truncated")
			}
			end := start + size
			after, ok := expectEI(data, end)
			if !ok {
				return 0, 0, 0, fmt.Errorf("inline image dimensions do not end at EI")
			}
			return end, after, model.StreamRecovered, nil
		}
	}
	for i := start; i+1 < len(data); i++ {
		if data[i] != 'E' {
			continue
		}
		after, ok := eiAt(data, i)
		if !ok || !scansCleanly(data, after, source, limits) {
			continue
		}
		// The whitespace introducing EI is a separator rather than image
		// data. Where the data itself ends in a whitespace byte the two are
		// genuinely ambiguous, and dropping one is the conventional reading;
		// an image that cares carries /L, which never reaches this path.
		end := i
		if end > start && syntax.IsWhitespace(data[end-1]) {
			end--
		}
		return end, after, model.StreamRecovered, nil
	}
	return 0, 0, 0, fmt.Errorf("inline image has no EI terminator")
}

// inlineImage handles a BI operator, consuming the image dictionary, its
// payload and the closing EI.
//
// It runs here rather than in execute because it has to drive the scanner: the
// payload is not PDF syntax, and only the dictionary that precedes it says
// where it ends. The bytes are located but never decoded, matching how image
// XObjects are treated — this library does not implement image codecs.
func (c *contentInterpreter) inlineImage(scanner *syntax.Scanner, bi model.Token) error {
	if len(c.operands) > 0 {
		return fmt.Errorf("BI takes no operands")
	}
	data, base := scanner.Data(), scanner.Offset()
	entries := make([]model.DictionaryEntry, 0, 8)
	var id model.Token
	for {
		token, err := scanner.NextNonTrivia()
		if err != nil {
			return err
		}
		if token.Kind == model.TokenEOF {
			return fmt.Errorf("inline image ends before ID")
		}
		if token.Kind == model.TokenKeyword {
			if word := string(data[token.Span.Start-base : token.Span.End-base]); word != "ID" {
				return fmt.Errorf("unexpected keyword %q in inline image dictionary", word)
			}
			id = token
			break
		}
		if token.Kind != model.TokenName {
			return fmt.Errorf("inline image dictionary key is not a name at %+v", token.Span)
		}
		key, err := scanner.ParseObjectAt(int(token.Span.Start - base))
		if err != nil {
			return err
		}
		name, ok := key.Value.(model.Name)
		if !ok {
			return fmt.Errorf("inline image dictionary key is not a name at %+v", key.Span)
		}
		value, err := scanner.ParseObjectAt(scanner.Pos())
		if err != nil {
			return err
		}
		if len(entries) >= 64 {
			return fmt.Errorf("inline image dictionary entry limit")
		}
		entries = append(entries, model.DictionaryEntry{Key: name, KeySpan: key.Span, Value: value})
	}
	dict := model.Dictionary{Entries: entries}

	// Exactly one whitespace byte separates ID from the payload, so the second
	// byte is already image data and must not be skipped. CRLF is tolerated
	// because producers emit it, but nothing longer is.
	start := int(id.Span.End - base)
	if start >= len(data) || !syntax.IsWhitespace(data[start]) {
		return fmt.Errorf("inline image ID is not followed by whitespace")
	}
	if data[start] == '\r' && start+1 < len(data) && data[start+1] == '\n' {
		start++
	}
	start++

	end, after, boundary, err := findInlineImageEnd(data, start, dict, id.Span.Source, c.b.doc.Options.Limits)
	if err != nil {
		return err
	}
	if err := scanner.Seek(after); err != nil {
		return err
	}

	payload := model.Span{Source: id.Span.Source, Start: base + int64(start), End: base + int64(end)}
	ei := model.Span{Source: id.Span.Source, Start: base + int64(after) - 2, End: base + int64(after)}
	dictSpan := model.Span{Source: bi.Span.Source, Start: bi.Span.End, End: id.Span.Start}
	op := Operation{
		Operator: "BI",
		Operands: []model.Object{{Span: dictSpan, Value: dict}},
		Span:     bi.Span,
		FormPath: append([]FormCall(nil), c.formPath...),
	}
	index := len(c.b.pdf.Pages[c.page].Operations)
	c.b.pdf.Pages[c.page].Operations = append(c.b.pdf.Pages[c.page].Operations, op)

	resource, exists := c.b.images[payload]
	if !exists {
		if len(c.b.pdf.ImageResources) >= c.b.maxObjects {
			return fmt.Errorf("image resource limit at %+v", payload)
		}
		image := ImageResource{
			Object: model.Object{Span: dictSpan, Value: dict},
			Stream: model.Stream{
				Dictionary:     dict,
				DictionarySpan: dictSpan,
				StartKeyword:   id.Span,
				DataStart:      model.Position{Source: payload.Source, Offset: payload.Start},
				Encoded:        &payload,
				EndKeyword:     &ei,
				Boundary:       boundary,
			},
		}
		if width, ok := inlineInt(dict, "W", "Width"); ok {
			image.Width = int(width)
		}
		if height, ok := inlineInt(dict, "H", "Height"); ok {
			image.Height = int(height)
		}
		if bits, ok := inlineInt(dict, "BPC", "BitsPerComponent"); ok {
			image.BitsPerComponent = int(bits)
		}
		if space, ok := inlineKey(dict, "CS", "ColorSpace"); ok {
			image.ColorSpace = space
		}
		if mask, ok := inlineKey(dict, "IM", "ImageMask"); ok {
			if _, isBool := mask.Value.(model.Boolean); !isBool {
				return fmt.Errorf("inline image /IM is not a boolean at %+v", mask.Span)
			}
			image.ImageMask = inlineIsMask(dict)
		}
		resource = len(c.b.pdf.ImageResources)
		c.b.pdf.ImageResources = append(c.b.pdf.ImageResources, image)
		c.b.images[payload] = resource
	}

	// Like an image XObject, an inline image is drawn into the unit square, so
	// the CTM alone gives its placement.
	source := c.source(op, index)
	source.Spans = append(source.Spans, payload, ei)
	c.item(ElementImage, len(c.b.pdf.Images))
	c.b.pdf.Images = append(c.b.pdf.Images, DetailedImage{
		Source:   source,
		Resource: resource,
		Matrix:   c.state.graphics.CTM,
		State:    c.state.graphics,
	})
	return nil
}
