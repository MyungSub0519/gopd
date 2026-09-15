package pdf

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"
)

const maxCMapEntries = 65536

type CodeSpace struct{ Low, High []byte }
type CMap struct {
	CodeSpaces []CodeSpace
	Mappings   map[string]string
}
type decodedCode struct {
	bytes    []byte
	unicode  string
	complete bool
}

func parseToUnicode(data []byte) (*CMap, error) {
	tokens, err := Lex(data, 1, 0)
	if err != nil {
		return nil, err
	}
	var raw []string
	for _, t := range tokens {
		if t.Kind != TokenWhitespace && t.Kind != TokenComment && t.Kind != TokenEOF {
			raw = append(raw, string(data[t.Span.Start:t.Span.End]))
		}
	}
	cmap := &CMap{Mappings: make(map[string]string)}
	for i := 0; i < len(raw); i++ {
		op := raw[i]
		if op == "usecmap" {
			return nil, fmt.Errorf("ToUnicode usecmap inheritance is unsupported")
		}
		if op != "begincodespacerange" && op != "beginbfchar" && op != "beginbfrange" {
			continue
		}
		if i == 0 {
			return nil, fmt.Errorf("missing count before %s", op)
		}
		count, e := strconv.Atoi(raw[i-1])
		if e != nil || count < 0 || count > maxCMapEntries {
			return nil, fmt.Errorf("invalid CMap count before %s", op)
		}
		if op == "begincodespacerange" && count > 256-len(cmap.CodeSpaces) {
			return nil, fmt.Errorf("CMap codespace count limit exceeded")
		}
		i++
		nextHex := func() ([]byte, error) {
			if i >= len(raw) {
				return nil, fmt.Errorf("truncated %s", op)
			}
			b, e := cmapHex(raw[i])
			i++
			return b, e
		}
		for n := 0; n < count; n++ {
			low, e := nextHex()
			if e != nil {
				return nil, e
			}
			if len(low) < 1 || len(low) > 4 {
				return nil, fmt.Errorf("CMap source code width must be 1..4")
			}
			high, e := nextHex()
			if e != nil {
				return nil, e
			}
			switch op {
			case "begincodespacerange":
				if len(low) != len(high) || codeNumber(low) > codeNumber(high) {
					return nil, fmt.Errorf("invalid CMap code space")
				}
				cmap.CodeSpaces = append(cmap.CodeSpaces, CodeSpace{low, high})
			case "beginbfchar":
				value, e := cmapUnicode(high)
				if e != nil {
					return nil, e
				}
				if e = cmap.add(low, value); e != nil {
					return nil, e
				}
			case "beginbfrange":
				if len(low) != len(high) || codeNumber(low) > codeNumber(high) {
					return nil, fmt.Errorf("invalid CMap range")
				}
				length := uint64(codeNumber(high)) - uint64(codeNumber(low)) + 1
				if length > maxCMapEntries || uint64(len(cmap.Mappings))+length > maxCMapEntries {
					return nil, fmt.Errorf("CMap range exceeds resource limit")
				}
				if i >= len(raw) {
					return nil, fmt.Errorf("truncated CMap range")
				}
				if raw[i] == "[" {
					i++
					for j := uint64(0); j < length; j++ {
						value, e := nextHex()
						if e != nil {
							return nil, e
						}
						u, e := cmapUnicode(value)
						if e != nil {
							return nil, e
						}
						if e = cmap.add(incrementCode(low, j), u); e != nil {
							return nil, e
						}
					}
					if i >= len(raw) || raw[i] != "]" {
						return nil, fmt.Errorf("invalid CMap destination array")
					}
					i++
				} else {
					start, e := nextHex()
					if e != nil {
						return nil, e
					}
					last := incrementCode(start, length-1)
					if strings.Compare(string(last), string(start)) < 0 {
						return nil, fmt.Errorf("ToUnicode destination range overflows")
					}
					for j := uint64(0); j < length; j++ {
						value := incrementCode(start, j)
						u, e := cmapUnicode(value)
						if e != nil {
							return nil, e
						}
						if e = cmap.add(incrementCode(low, j), u); e != nil {
							return nil, e
						}
					}
				}
			}
		}
		end := "end" + strings.TrimPrefix(op, "begin")
		if i >= len(raw) || raw[i] != end {
			return nil, fmt.Errorf("missing %s", end)
		}
	}
	if len(cmap.Mappings) == 0 {
		return nil, fmt.Errorf("ToUnicode has no supported mappings")
	}
	if len(cmap.CodeSpaces) > 0 {
		for code := range cmap.Mappings {
			valid := false
			for _, space := range cmap.CodeSpaces {
				if len(code) == len(space.Low) && codeNumber([]byte(code)) >= codeNumber(space.Low) && codeNumber([]byte(code)) <= codeNumber(space.High) {
					valid = true
					break
				}
			}
			if !valid {
				return nil, fmt.Errorf("ToUnicode code %X is outside its codespace", code)
			}
		}
	}
	return cmap, nil
}

func (c *CMap) add(code []byte, value string) error {
	if _, exists := c.Mappings[string(code)]; exists {
		return fmt.Errorf("duplicate ToUnicode mapping for %X", code)
	}
	if len(c.Mappings) >= maxCMapEntries {
		return fmt.Errorf("CMap exceeds resource limit")
	}
	c.Mappings[string(code)] = value
	return nil
}
func cmapHex(s string) ([]byte, error) {
	if len(s) < 2 || s[0] != '<' || s[len(s)-1] != '>' {
		return nil, fmt.Errorf("expected CMap hex string, got %q", s)
	}
	clean := strings.Join(strings.Fields(s[1:len(s)-1]), "")
	if len(clean) == 0 || len(clean)%2 != 0 || len(clean) > 2048 {
		return nil, fmt.Errorf("invalid CMap hex string length")
	}
	return hex.DecodeString(clean)
}
func cmapUnicode(b []byte) (string, error) {
	if len(b) == 0 || len(b)%2 != 0 {
		return "", fmt.Errorf("ToUnicode destination must be UTF-16BE")
	}
	units := make([]uint16, len(b)/2)
	for i := range units {
		units[i] = binary.BigEndian.Uint16(b[2*i:])
	}
	for i := 0; i < len(units); i++ {
		u := units[i]
		if u >= 0xD800 && u <= 0xDBFF {
			if i+1 == len(units) || units[i+1] < 0xDC00 || units[i+1] > 0xDFFF {
				return "", fmt.Errorf("invalid ToUnicode surrogate pair")
			}
			i++
		} else if u >= 0xDC00 && u <= 0xDFFF {
			return "", fmt.Errorf("unpaired ToUnicode surrogate")
		}
	}
	return string(utf16.Decode(units)), nil
}
func codeNumber(code []byte) uint32 {
	var n uint32
	for _, b := range code {
		n = n<<8 | uint32(b)
	}
	return n
}
func incrementCode(b []byte, n uint64) []byte {
	out := append([]byte(nil), b...)
	for i := len(out) - 1; i >= 0; i-- {
		n += uint64(out[i])
		out[i] = byte(n)
		n >>= 8
	}
	return out
}
func (c *CMap) decode(raw []byte) (string, []decodedCode, bool) {
	text, codes, complete, _ := c.decodeBounded(raw, 256<<20)
	return text, codes, complete
}
func (c *CMap) decodeBounded(raw []byte, limit int64) (string, []decodedCode, bool, error) {
	spaces := c.CodeSpaces
	if len(spaces) == 0 {
		lengths := make(map[int]bool)
		for code := range c.Mappings {
			lengths[len(code)] = true
		}
		for n := range lengths {
			spaces = append(spaces, CodeSpace{make([]byte, n), []byte(strings.Repeat("\xff", n))})
		}
		sort.Slice(spaces, func(i, j int) bool { return len(spaces[i].Low) < len(spaces[j].Low) })
	}
	var out strings.Builder
	var codes []decodedCode
	complete := true
	for at := 0; at < len(raw); {
		n := 0
		for _, space := range spaces {
			size := len(space.Low)
			if at+size <= len(raw) {
				value := codeNumber(raw[at : at+size])
				if value >= codeNumber(space.Low) && value <= codeNumber(space.High) {
					n = size
					break
				}
			}
		}
		if n == 0 {
			n = 1
		}
		code := append([]byte(nil), raw[at:at+n]...)
		u, ok := c.Mappings[string(code)]
		if !ok {
			u = "\uFFFD"
			complete = false
		}
		if int64(len(u)) > limit-int64(out.Len()) {
			return "", nil, false, fmt.Errorf("expanded Unicode text byte limit exceeded")
		}
		out.WriteString(u)
		codes = append(codes, decodedCode{code, u, ok})
		at += n
	}
	return out.String(), codes, complete, nil
}
