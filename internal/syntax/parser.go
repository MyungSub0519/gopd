package syntax

import (
	"strconv"

	"github.com/MyungSub0519/gopd/internal/pdfmodel"
)

// ParseObject parses one direct PDF object or indirect reference. consumed is
// relative to data and includes leading whitespace and comments, but excludes
// trailing trivia. Spans are absolute within source and exclude leading trivia.
// Bare keywords other than true, false and null are not object values.
// Default limits are 256 container levels, 16 MiB per token and 1,048,576 values.
func ParseObject(data []byte, source pdfmodel.SourceID, offset int64) (object pdfmodel.Object, consumed int, err error) {
	return ParseObjectWithLimits(data, source, offset, pdfmodel.Limits{})
}

// ParseObjectWithLimits parses one object with the supplied syntax limits.
// Zero limit fields use the same defaults as ParseObject.
func ParseObjectWithLimits(data []byte, source pdfmodel.SourceID, offset int64, limits pdfmodel.Limits) (pdfmodel.Object, int, error) {
	remaining := limits.MaxValues
	if remaining == 0 {
		remaining = maxSyntaxObjects
	}
	object, consumed, _, err := ParseObjectWithValueBudget(data, source, offset, limits, remaining)
	return object, consumed, err
}

// ParseObjectWithValueBudget counts containers and values (not dictionary keys)
// against remaining. Unlike zero limit fields, remaining == 0 means exhausted.
// The returned count includes work completed before a parsing error.
func ParseObjectWithValueBudget(data []byte, source pdfmodel.SourceID, offset int64, limits pdfmodel.Limits, remaining int) (pdfmodel.Object, int, int, error) {
	if limits.MaxDepth < 0 || limits.MaxTokenBytes < 0 || limits.MaxValues < 0 || remaining < 0 {
		return pdfmodel.Object{}, 0, 0, &syntaxError{Position: pdfmodel.Position{Source: source, Offset: offset}, Message: "syntax limits must not be negative"}
	}
	// Even explicitly requested limits must stay below a safe recursion ceiling.
	if limits.MaxDepth > 4096 {
		return pdfmodel.Object{}, 0, 0, &syntaxError{Position: pdfmodel.Position{Source: source, Offset: offset}, Message: "syntax nesting depth limit must not exceed 4096"}
	}
	if limits.MaxDepth == 0 {
		limits.MaxDepth = defaultSyntaxDepth
	}
	if limits.MaxTokenBytes == 0 {
		limits.MaxTokenBytes = defaultSyntaxTokenBytes
	}
	if limits.MaxValues == 0 {
		limits.MaxValues = maxSyntaxObjects
	}
	s, err := newSyntaxScanner(data, source, offset, limits.MaxTokenBytes)
	if err != nil {
		return pdfmodel.Object{}, 0, 0, err
	}
	p := objectParser{scanner: s, maxDepth: limits.MaxDepth, maxValues: min(limits.MaxValues, remaining)}
	object, err := p.parse(0)
	return object, p.scanner.pos, p.objects, err
}

type objectParser struct {
	scanner   syntaxScanner
	maxDepth  int
	objects   int
	maxValues int
}

func (p *objectParser) bytes(token pdfmodel.Token) []byte {
	return p.scanner.data[token.Span.Start-p.scanner.offset : token.Span.End-p.scanner.offset]
}

func (p *objectParser) parse(depth int) (pdfmodel.Object, error) {
	token, err := p.scanner.nextNonTrivia()
	if err != nil {
		return pdfmodel.Object{}, err
	}
	return p.parseToken(token, depth)
}

func (p *objectParser) parseToken(token pdfmodel.Token, depth int) (pdfmodel.Object, error) {
	object := pdfmodel.Object{Span: token.Span}
	if p.objects >= p.maxValues {
		return object, p.scanner.limitAt(int(token.Span.Start-p.scanner.offset), "object value count limit exceeded")
	}
	p.objects++
	switch token.Kind {
	case pdfmodel.TokenInteger:
		object.Value = pdfmodel.Integer(p.bytes(token))
		// An integer may start a reference. Speculative lookahead must not consume
		// trailing trivia, another object, or report errors after this object.
		look := p.scanner
		generation, err := look.nextNonTrivia()
		if err != nil || generation.Kind != pdfmodel.TokenInteger {
			return object, nil
		}
		r, err := look.nextNonTrivia()
		if err != nil || r.Kind != pdfmodel.TokenKeyword || string(p.bytes(r)) != "R" {
			return object, nil
		}
		number, numberErr := parseReferenceComponent(p.bytes(token), 32)
		gen, genErr := parseReferenceComponent(p.bytes(generation), 16)
		if numberErr != nil || number == 0 || genErr != nil {
			return object, p.scanner.errorAt(int(token.Span.Start-p.scanner.offset), "invalid indirect reference number or generation")
		}
		p.scanner = look
		object.Span.End = r.Span.End
		object.Value = pdfmodel.Reference{ID: pdfmodel.ObjectID{Number: uint32(number), Generation: uint16(gen)}}
	case pdfmodel.TokenReal:
		object.Value = pdfmodel.Real(p.bytes(token))
	case pdfmodel.TokenName:
		object.Value = decodeName(p.bytes(token))
	case pdfmodel.TokenLiteralString:
		object.Value = pdfmodel.PDFString{Form: pdfmodel.StringLiteral, Bytes: decodeLiteralString(p.bytes(token))}
	case pdfmodel.TokenHexString:
		object.Value = pdfmodel.PDFString{Form: pdfmodel.StringHex, Bytes: decodeHexString(p.bytes(token))}
	case pdfmodel.TokenKeyword:
		switch string(p.bytes(token)) {
		case "true":
			object.Value = pdfmodel.Boolean(true)
		case "false":
			object.Value = pdfmodel.Boolean(false)
		case "null":
			object.Value = pdfmodel.Null{}
		default:
			return object, p.scanner.errorAt(int(token.Span.Start-p.scanner.offset), "keyword is not a PDF object value")
		}
	case pdfmodel.TokenArrayOpen:
		if depth >= p.maxDepth {
			return object, p.scanner.limitAt(int(token.Span.Start-p.scanner.offset), "object nesting depth limit exceeded")
		}
		array := pdfmodel.Array{Items: make([]pdfmodel.Object, 0)}
		for {
			next, err := p.scanner.nextNonTrivia()
			if err != nil {
				return object, err
			}
			if next.Kind == pdfmodel.TokenArrayClose {
				object.Value, object.Span.End = array, next.Span.End
				break
			}
			item, err := p.parseToken(next, depth+1)
			if err != nil {
				return object, err
			}
			array.Items = append(array.Items, item)
		}
	case pdfmodel.TokenDictOpen:
		if depth >= p.maxDepth {
			return object, p.scanner.limitAt(int(token.Span.Start-p.scanner.offset), "object nesting depth limit exceeded")
		}
		dictionary := pdfmodel.Dictionary{Entries: make([]pdfmodel.DictionaryEntry, 0)}
		for {
			key, err := p.scanner.nextNonTrivia()
			if err != nil {
				return object, err
			}
			if key.Kind == pdfmodel.TokenDictClose {
				object.Value, object.Span.End = dictionary, key.Span.End
				break
			}
			if key.Kind != pdfmodel.TokenName {
				return object, p.scanner.errorAt(int(key.Span.Start-p.scanner.offset), "expected dictionary name key or >>")
			}
			value, err := p.parse(depth + 1)
			if err != nil {
				return object, err
			}
			dictionary.Entries = append(dictionary.Entries, pdfmodel.DictionaryEntry{Key: decodeName(p.bytes(key)), KeySpan: key.Span, Value: value})
		}
	case pdfmodel.TokenEOF:
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

func decodeName(raw []byte) pdfmodel.Name {
	decoded := make([]byte, 0, len(raw)-1)
	for i := 1; i < len(raw); i++ {
		if raw[i] == '#' {
			decoded = append(decoded, byte(hexNibble(raw[i+1])<<4|hexNibble(raw[i+2])))
			i += 2
		} else {
			decoded = append(decoded, raw[i])
		}
	}
	return pdfmodel.Name(decoded)
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
