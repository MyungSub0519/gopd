package syntax

import (
	"strconv"

	"github.com/MyungSub0519/gopd/internal/model"
)

// ParseObjectWithLimits parses one direct object or indirect reference from
// the start of data.
//
// It returns the object, the number of bytes consumed, and any error. The
// consumed count includes leading whitespace and comments but stops at the end
// of the object, so trailing trivia is left for the caller; that is what lets
// a caller parse a sequence of objects by advancing through the same buffer.
//
// Spans are absolute within source: offset states where data begins there.
func ParseObjectWithLimits(data []byte, source model.SourceID, offset int64, limits model.Limits) (model.Object, int, error) {
	if limits.MaxDepth < 0 || limits.MaxTokenBytes < 0 {
		return model.Object{}, 0, &syntaxError{Position: model.Position{Source: source, Offset: offset}, Message: "syntax limits must not be negative"}
	}
	// Even explicitly requested limits must stay below a safe recursion ceiling.
	if limits.MaxDepth > 4096 {
		return model.Object{}, 0, &syntaxError{Position: model.Position{Source: source, Offset: offset}, Message: "syntax nesting depth limit must not exceed 4096"}
	}
	if limits.MaxDepth == 0 {
		limits.MaxDepth = defaultSyntaxDepth
	}
	if limits.MaxTokenBytes == 0 {
		limits.MaxTokenBytes = defaultSyntaxTokenBytes
	}
	s, err := newSyntaxScanner(data, source, offset, limits.MaxTokenBytes)
	if err != nil {
		return model.Object{}, 0, err
	}
	p := objectParser{scanner: s, maxDepth: limits.MaxDepth}
	object, err := p.parse(0)
	return object, p.scanner.pos, err
}

// objectParser builds objects from the scanner's tokens.
//
// It counts every value it produces, not just nesting depth, because a flat
// but enormous array is as effective a resource attack as a deeply nested one.
type objectParser struct {
	scanner  syntaxScanner
	maxDepth int
	objects  int
}

// bytes returns the raw input behind a token, converting the token's absolute
// span back into an index into the scanner's buffer.
func (p *objectParser) bytes(token model.Token) []byte {
	return p.scanner.data[token.Span.Start-p.scanner.offset : token.Span.End-p.scanner.offset]
}

// parse reads the next meaningful token and parses an object from it.
func (p *objectParser) parse(depth int) (model.Object, error) {
	token, err := p.scanner.nextNonTrivia()
	if err != nil {
		return model.Object{}, err
	}
	return p.parseToken(token, depth)
}

// parseToken parses the object that begins with token.
//
// It takes the first token as an argument rather than reading it, because the
// array and dictionary loops must inspect a token to see whether it closes the
// container before deciding to parse an item from it.
func (p *objectParser) parseToken(token model.Token, depth int) (model.Object, error) {
	object := model.Object{Span: token.Span}
	p.objects++
	if p.objects > maxSyntaxObjects {
		return object, p.scanner.errorAt(int(token.Span.Start-p.scanner.offset), "object value count limit exceeded")
	}
	switch token.Kind {
	case model.TokenInteger:
		object.Value = model.Integer(p.bytes(token))
		// An integer may be the start of an indirect reference, which is
		// written "number generation R". Nothing but the third token tells
		// the two apart, so look ahead on a copy of the scanner: if the
		// pattern does not appear, the copy is discarded and the integer
		// stands on its own, having consumed nothing extra.
		//
		// A failure during lookahead is not this object's failure either,
		// so errors are swallowed rather than returned.
		look := p.scanner
		generation, err := look.nextNonTrivia()
		if err != nil || generation.Kind != model.TokenInteger {
			return object, nil
		}
		r, err := look.nextNonTrivia()
		if err != nil || r.Kind != model.TokenKeyword || string(p.bytes(r)) != "R" {
			return object, nil
		}
		// The pattern matched, so from here a malformed number is a real
		// error rather than a reason to fall back to a plain integer.
		number, numberErr := parseReferenceComponent(p.bytes(token), 32)
		gen, genErr := parseReferenceComponent(p.bytes(generation), 16)
		if numberErr != nil || number == 0 || genErr != nil {
			return object, p.scanner.errorAt(int(token.Span.Start-p.scanner.offset), "invalid indirect reference number or generation")
		}
		// Commit the lookahead and widen the span to cover all three tokens.
		p.scanner = look
		object.Span.End = r.Span.End
		object.Value = model.Reference{ID: model.ObjectID{Number: uint32(number), Generation: uint16(gen)}}
	case model.TokenReal:
		object.Value = model.Real(p.bytes(token))
	case model.TokenName:
		object.Value = decodeName(p.bytes(token))
	case model.TokenLiteralString:
		object.Value = model.PDFString{Form: model.StringLiteral, Bytes: decodeLiteralString(p.bytes(token))}
	case model.TokenHexString:
		object.Value = model.PDFString{Form: model.StringHex, Bytes: decodeHexString(p.bytes(token))}
	case model.TokenKeyword:
		switch string(p.bytes(token)) {
		case "true":
			object.Value = model.Boolean(true)
		case "false":
			object.Value = model.Boolean(false)
		case "null":
			object.Value = model.Null{}
		default:
			return object, p.scanner.errorAt(int(token.Span.Start-p.scanner.offset), "keyword is not a PDF object value")
		}
	case model.TokenArrayOpen:
		if depth >= p.maxDepth {
			return object, p.scanner.errorAt(int(token.Span.Start-p.scanner.offset), "object nesting depth limit exceeded")
		}
		// Non-nil empty slice: an array that is present but empty should not
		// be indistinguishable from one that is absent.
		array := model.Array{Items: make([]model.Object, 0)}
		for {
			next, err := p.scanner.nextNonTrivia()
			if err != nil {
				return object, err
			}
			if next.Kind == model.TokenArrayClose {
				object.Value, object.Span.End = array, next.Span.End
				break
			}
			item, err := p.parseToken(next, depth+1)
			if err != nil {
				return object, err
			}
			array.Items = append(array.Items, item)
		}
	case model.TokenDictOpen:
		if depth >= p.maxDepth {
			return object, p.scanner.errorAt(int(token.Span.Start-p.scanner.offset), "object nesting depth limit exceeded")
		}
		dictionary := model.Dictionary{Entries: make([]model.DictionaryEntry, 0)}
		for {
			key, err := p.scanner.nextNonTrivia()
			if err != nil {
				return object, err
			}
			if key.Kind == model.TokenDictClose {
				object.Value, object.Span.End = dictionary, key.Span.End
				break
			}
			// Only a name may be a key. Anything else means the dictionary
			// is malformed, and guessing would silently misread the file.
			if key.Kind != model.TokenName {
				return object, p.scanner.errorAt(int(key.Span.Start-p.scanner.offset), "expected dictionary name key or >>")
			}
			value, err := p.parse(depth + 1)
			if err != nil {
				return object, err
			}
			dictionary.Entries = append(dictionary.Entries, model.DictionaryEntry{Key: decodeName(p.bytes(key)), KeySpan: key.Span, Value: value})
		}
	case model.TokenEOF:
		return object, p.scanner.errorAt(p.scanner.pos, "unexpected end of input; expected PDF object")
	default:
		return object, p.scanner.errorAt(int(token.Span.Start-p.scanner.offset), "unexpected delimiter; expected PDF object")
	}
	return object, nil
}

// parseReferenceComponent parses an object or generation number, which must be
// a non-negative integer that fits in bits.
//
// A leading '+' is stripped because the token scanner accepts it as part of a
// number while strconv.ParseUint does not.
func parseReferenceComponent(raw []byte, bits int) (uint64, error) {
	if len(raw) > 0 && raw[0] == '+' {
		raw = raw[1:]
	}
	return strconv.ParseUint(string(raw), 10, bits)
}

// decodeName converts a name token to its value by dropping the leading
// solidus and resolving #xx escapes.
//
// The scanner has already validated every escape, so the hex digits here are
// known to be well formed and need no re-checking.
func decodeName(raw []byte) model.Name {
	decoded := make([]byte, 0, len(raw)-1)
	for i := 1; i < len(raw); i++ {
		if raw[i] == '#' {
			decoded = append(decoded, byte(hexNibble(raw[i+1])<<4|hexNibble(raw[i+2])))
			i += 2
		} else {
			decoded = append(decoded, raw[i])
		}
	}
	return model.Name(decoded)
}

// decodeLiteralString converts a (...) token to its bytes, resolving escapes.
//
// Two rules are easy to get wrong. Any end-of-line inside the string becomes a
// single \n, so a CRLF written literally collapses to one byte. And a
// backslash before an end-of-line is a line continuation that produces nothing
// at all, which is how a producer wraps a long string across lines.
//
// An unrecognised escape yields the escaped byte itself, as the specification
// requires, rather than being an error.
func decodeLiteralString(raw []byte) []byte {
	decoded := make([]byte, 0, len(raw)-2)
	for i := 1; i < len(raw)-1; i++ {
		c := raw[i]
		if c == '\r' {
			decoded = append(decoded, '\n')
			if i+1 < len(raw)-1 && raw[i+1] == '\n' {
				i++
			}
			continue
		}
		if c != '\\' {
			decoded = append(decoded, c)
			continue
		}
		i++
		c = raw[i]
		switch c {
		case 'n':
			decoded = append(decoded, '\n')
		case 'r':
			decoded = append(decoded, '\r')
		case 't':
			decoded = append(decoded, '\t')
		case 'b':
			decoded = append(decoded, '\b')
		case 'f':
			decoded = append(decoded, '\f')
		case '\r':
			if i+1 < len(raw)-1 && raw[i+1] == '\n' {
				i++
			}
		case '\n': // A backslash followed by an EOL is a line continuation.
		default:
			// Octal escapes are one to three digits, and stop early at the
			// first non-octal byte, so \5 and \053 are both valid.
			if c >= '0' && c <= '7' {
				n := int(c - '0')
				for digits := 1; digits < 3 && i+1 < len(raw)-1 && raw[i+1] >= '0' && raw[i+1] <= '7'; digits++ {
					i++
					n = n*8 + int(raw[i]-'0')
				}
				decoded = append(decoded, byte(n))
			} else {
				decoded = append(decoded, c)
			}
		}
	}
	return decoded
}

// decodeHexString converts a <...> token to its bytes.
//
// Whitespace between digits is ignored, and an odd number of digits has an
// implied trailing zero, so <A1B> means the two bytes A1 B0.
func decodeHexString(raw []byte) []byte {
	decoded := make([]byte, 0, (len(raw)-1)/2)
	high := -1
	for _, c := range raw[1 : len(raw)-1] {
		if isPDFWhitespace(c) {
			continue
		}
		if high < 0 {
			high = hexNibble(c)
		} else {
			decoded = append(decoded, byte(high<<4|hexNibble(c)))
			high = -1
		}
	}
	if high >= 0 {
		decoded = append(decoded, byte(high<<4))
	}
	return decoded
}
