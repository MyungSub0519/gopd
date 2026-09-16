package syntax

import (
	"strconv"

	"github.com/MyungSub0519/gopd/internal/model"
)

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

type objectParser struct {
	scanner  syntaxScanner
	maxDepth int
	objects  int
}

func (p *objectParser) bytes(token model.Token) []byte {
	return p.scanner.data[token.Span.Start-p.scanner.offset : token.Span.End-p.scanner.offset]
}

func (p *objectParser) parse(depth int) (model.Object, error) {
	token, err := p.scanner.nextNonTrivia()
	if err != nil {
		return model.Object{}, err
	}
	return p.parseToken(token, depth)
}

func (p *objectParser) parseToken(token model.Token, depth int) (model.Object, error) {
	object := model.Object{Span: token.Span}
	p.objects++
	if p.objects > maxSyntaxObjects {
		return object, p.scanner.errorAt(int(token.Span.Start-p.scanner.offset), "object value count limit exceeded")
	}
	switch token.Kind {
	case model.TokenInteger:
		object.Value = model.Integer(p.bytes(token))
		// An integer may start a reference. Speculative lookahead must not consume
		// trailing trivia, another object, or report errors after this object.
		look := p.scanner
		generation, err := look.nextNonTrivia()
		if err != nil || generation.Kind != model.TokenInteger {
			return object, nil
		}
		r, err := look.nextNonTrivia()
		if err != nil || r.Kind != model.TokenKeyword || string(p.bytes(r)) != "R" {
			return object, nil
		}
		number, numberErr := parseReferenceComponent(p.bytes(token), 32)
		gen, genErr := parseReferenceComponent(p.bytes(generation), 16)
		if numberErr != nil || number == 0 || genErr != nil {
			return object, p.scanner.errorAt(int(token.Span.Start-p.scanner.offset), "invalid indirect reference number or generation")
		}
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

func parseReferenceComponent(raw []byte, bits int) (uint64, error) {
	if len(raw) > 0 && raw[0] == '+' {
		raw = raw[1:]
	}
	return strconv.ParseUint(string(raw), 10, bits)
}

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
