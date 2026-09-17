package gopd

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

type CodeSpace struct{ Low, High []byte }

type CMap struct {
	CodeSpaces []CodeSpace
	Mappings   map[string]string
}

const maxCMapEntries = 65536

var errCMapLimit = fmt.Errorf("CMap: %w", ErrLimit)

type decodedCode struct {
	bytes    []byte
	unicode  string
	complete bool
}

func parseToUnicode(data []byte) (*CMap, error) {
	return parseToUnicodeBounded(data, maxCMapEntries, 256<<20)
}

// The byte budget counts retained mapping keys and UTF-8 destination strings,
// excluding map overhead and codespaces. The caller shares it with emitted text.
func parseToUnicodeBounded(data []byte, entryLimit int, byteLimit int64) (*CMap, error) {
	if entryLimit < 0 || byteLimit < 0 {
		return nil, errCMapLimit
	}
	entryLimit = min(entryLimit, maxCMapEntries)
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
	add := func(low []byte, offset uint64, destination []byte) error {
		size, e := cmapUnicodeSize(destination)
		if e != nil {
			return e
		}
		cost := int64(len(low)) + int64(size)
		if len(cmap.Mappings) >= entryLimit || cost > byteLimit {
			return errCMapLimit
		}
		// Allocate expanded keys and Unicode strings only after both checks.
		code := string(incrementCode(low, offset))
		if _, exists := cmap.Mappings[code]; exists {
			return fmt.Errorf("duplicate ToUnicode mapping for %X", code)
		}
		value, e := cmapUnicode(destination)
		if e != nil {
			return e
		}
		cmap.Mappings[code] = value
		byteLimit -= cost
		return nil
	}
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
		if e != nil || count < 0 {
			return nil, fmt.Errorf("invalid CMap count before %s", op)
		}
		if count > maxCMapEntries {
			return nil, errCMapLimit
		}
		if op == "begincodespacerange" && count > 256-len(cmap.CodeSpaces) {
			return nil, fmt.Errorf("codespace count: %w", errCMapLimit)
		}
		if op != "begincodespacerange" && count > entryLimit-len(cmap.Mappings) {
			return nil, errCMapLimit
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
				if !codeInSpace(low, CodeSpace{Low: low, High: high}) {
					return nil, fmt.Errorf("invalid CMap code space")
				}
				cmap.CodeSpaces = append(cmap.CodeSpaces, CodeSpace{low, high})
			case "beginbfchar":
				if e = add(low, 0, high); e != nil {
					return nil, e
				}
			case "beginbfrange":
				if len(low) != len(high) || codeNumber(low) > codeNumber(high) {
					return nil, fmt.Errorf("invalid CMap range")
				}
				length := uint64(codeNumber(high)) - uint64(codeNumber(low)) + 1
				if length > uint64(entryLimit-len(cmap.Mappings)) {
					return nil, errCMapLimit
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
						if e = add(low, j, value); e != nil {
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
					size, e := cmapUnicodeSize(start)
					if e != nil {
						return nil, e
					}
					if int64(len(low))+int64(size) > byteLimit {
						return nil, errCMapLimit
					}
					last := incrementCode(start, length-1)
					if strings.Compare(string(last), string(start)) < 0 {
						return nil, fmt.Errorf("ToUnicode destination range overflows")
					}
					// One input-sized scratch buffer, reused without expanding output.
					value := make([]byte, len(start))
					for j := uint64(0); j < length; j++ {
						incrementCodeInto(value, start, j)
						if e = add(low, j, value); e != nil {
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
				if codeInSpace([]byte(code), space) {
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

func (c *CMap) mappingBytes() int64 {
	var size int64
	for code, value := range c.Mappings {
		size += int64(len(code)) + int64(len(value))
	}
	return size
}

func codeInSpace(code []byte, space CodeSpace) bool {
	if len(code) != len(space.Low) || len(code) != len(space.High) {
		return false
	}
	for i, b := range code {
		if b < space.Low[i] || b > space.High[i] {
			return false
		}
	}
	return true
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

// cmapUnicodeSize validates UTF-16BE and measures its UTF-8 representation
// without allocating an expanded destination.
func cmapUnicodeSize(b []byte) (int, error) {
	if len(b) == 0 || len(b)%2 != 0 {
		return 0, fmt.Errorf("ToUnicode destination must be UTF-16BE")
	}
	size := 0
	for i := 0; i < len(b); i += 2 {
		u := binary.BigEndian.Uint16(b[i:])
		if u >= 0xD800 && u <= 0xDBFF {
			if i+2 == len(b) {
				return 0, fmt.Errorf("invalid ToUnicode surrogate pair")
			}
			next := binary.BigEndian.Uint16(b[i+2:])
			if next < 0xDC00 || next > 0xDFFF {
				return 0, fmt.Errorf("invalid ToUnicode surrogate pair")
			}
			i += 2
			size += 4
		} else if u >= 0xDC00 && u <= 0xDFFF {
			return 0, fmt.Errorf("unpaired ToUnicode surrogate")
		} else {
			size += utf8.RuneLen(rune(u))
		}
	}
	return size, nil
}

func cmapUnicode(b []byte) (string, error) {
	if _, err := cmapUnicodeSize(b); err != nil {
		return "", err
	}
	units := make([]uint16, len(b)/2)
	for i := range units {
		units[i] = binary.BigEndian.Uint16(b[2*i:])
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
	out := make([]byte, len(b))
	incrementCodeInto(out, b, n)
	return out
}

func incrementCodeInto(out, b []byte, n uint64) {
	for i := len(out) - 1; i >= 0; i-- {
		n += uint64(b[i])
		out[i] = byte(n)
		n >>= 8
	}
}

func (c *CMap) decode(raw []byte) (string, []decodedCode, bool) {
	text, codes, complete, _ := c.decodeBounded(raw, 256<<20)
	return text, codes, complete
}

func (c *CMap) decodeBounded(raw []byte, limit int64) (string, []decodedCode, bool, error) {
	return c.decodeSelected(raw, limit, true)
}

func (c *CMap) decodeSelected(raw []byte, limit int64, collectCodes bool) (string, []decodedCode, bool, error) {
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
				if codeInSpace(raw[at:at+size], space) {
					n = size
					break
				}
			}
		}
		if n == 0 {
			n = 1
		}
		code := raw[at : at+n]
		u, ok := c.Mappings[string(code)]
		if !ok {
			u = "\uFFFD"
			complete = false
		}
		if int64(len(u)) > limit-int64(out.Len()) {
			return "", nil, false, fmt.Errorf("%w: expanded Unicode text byte limit exceeded", ErrLimit)
		}
		out.WriteString(u)
		if collectCodes {
			codes = append(codes, decodedCode{append([]byte(nil), code...), u, ok})
		}
		at += n
	}
	return out.String(), codes, complete, nil
}
