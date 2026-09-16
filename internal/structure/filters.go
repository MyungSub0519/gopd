package structure

import (
	"bytes"
	"compress/zlib"
	"encoding/ascii85"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"

	"github.com/MyungSub0519/gopd/internal/model"
)

func (d *Document) DecodeStream(stream model.Stream) (model.Source, error) {
	if d.Encrypted {
		return model.Source{}, errors.New("encrypted stream decoding is not supported")
	}
	if _, err := stream.Dictionary.Get("F"); err == nil {
		return model.Source{}, errors.New("external stream data is not supported")
	} else if !errors.Is(err, model.ErrMissingKey) {
		return model.Source{}, err
	}
	if stream.Encoded == nil {
		return model.Source{}, errors.New("stream boundary is unresolved")
	}
	if id, ok := d.decoded[*stream.Encoded]; ok {
		return d.Sources[id], nil
	}
	remaining := d.Options.Limits.MaxDecodedBytes - d.decodedBytes
	if remaining <= 0 {
		return model.Source{}, errors.New("decoded stream byte budget exhausted")
	}
	data, err := d.Bytes(*stream.Encoded)
	if err != nil {
		return model.Source{}, err
	}
	var filters []model.Object
	if object, e := stream.Dictionary.Get("Filter"); e == nil {
		object, e = d.ResolveObject(object)
		if e != nil {
			return model.Source{}, e
		}
		switch value := object.Value.(type) {
		case model.Name:
			filters = []model.Object{object}
		case model.Array:
			filters = value.Items
		default:
			return model.Source{}, errors.New("invalid stream /Filter")
		}
	} else if !errors.Is(e, model.ErrMissingKey) {
		return model.Source{}, e
	}
	params := make([]*model.Object, len(filters))
	if object, e := stream.Dictionary.Get("DecodeParms"); e == nil {
		object, e = d.ResolveObject(object)
		if e != nil {
			return model.Source{}, e
		}
		switch value := object.Value.(type) {
		case model.Null:
		case model.Dictionary:
			if len(filters) != 1 {
				return model.Source{}, errors.New("DecodeParms dictionary needs one filter")
			}
			params[0] = &object
		case model.Array:
			if len(value.Items) != len(filters) {
				return model.Source{}, errors.New("Filter/DecodeParms array lengths differ")
			}
			for i := range value.Items {
				o, e := d.ResolveObject(value.Items[i])
				if e != nil {
					return model.Source{}, e
				}
				params[i] = &o
			}
		default:
			return model.Source{}, errors.New("invalid /DecodeParms")
		}
	} else if !errors.Is(e, model.ErrMissingKey) {
		return model.Source{}, e
	}
	steps := make([]model.Transform, 0, len(filters))
	for i, object := range filters {
		object, err = d.ResolveObject(object)
		if err != nil {
			return model.Source{}, err
		}
		name, ok := object.Value.(model.Name)
		if !ok {
			return model.Source{}, errors.New("filter name is not a PDF name")
		}
		data, err = decodeFilter(name, data, params[i], remaining)
		if err != nil {
			return model.Source{}, fmt.Errorf("/%s: %w", name, err)
		}
		steps = append(steps, model.Transform{Kind: model.TransformFilter, Name: name, Params: params[i]})
	}
	if int64(len(data)) > remaining {
		return model.Source{}, errors.New("decoded stream byte budget exceeded")
	}
	id := model.SourceID(len(d.Sources) + 1)
	if id == 0 {
		return model.Source{}, errors.New("source ID limit exceeded")
	}
	source := model.Source{ID: id, Reader: bytes.NewReader(data), Size: int64(len(data)), Origin: &model.Derivation{Input: *stream.Encoded, Steps: steps}}
	d.Sources[id] = source
	d.decoded[*stream.Encoded] = id
	d.decodedBytes += int64(len(data))
	return source, nil
}

func decodeFilter(name model.Name, data []byte, params *model.Object, limit int64) ([]byte, error) {
	var result []byte
	var err error
	switch name {
	case "FlateDecode", "Fl":
		r, e := zlib.NewReader(bytes.NewReader(data))
		if e != nil {
			return nil, e
		}
		result, err = readDecodeLimit(r, limit)
		closeErr := r.Close()
		if err == nil {
			err = closeErr
		}
		if err == nil && int64(len(result)) <= limit {
			result, err = applyPredictor(result, params, limit)
		}
	case "ASCIIHexDecode", "AHx":
		digits := make([]byte, 0, min(len(data), 4096))
		terminated := false
		for _, c := range data {
			if c == '>' {
				terminated = true
				break
			}
			if docSpace(c) {
				continue
			}
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
				return nil, errors.New("invalid ASCIIHex digit")
			}
			if int64(len(digits))/2 >= limit {
				return nil, errors.New("decoded byte limit exceeded")
			}
			digits = append(digits, c)
		}
		if !terminated {
			return nil, errors.New("missing ASCIIHex end marker")
		}
		if len(digits)%2 != 0 {
			digits = append(digits, '0')
		}
		result = make([]byte, len(digits)/2)
		_, err = hex.Decode(result, digits)
	case "ASCII85Decode", "A85":
		end := bytes.Index(data, []byte("~>"))
		if end < 0 {
			return nil, errors.New("missing ASCII85 end marker")
		}
		// Strip PDF whitespace, including NUL, before the standard decoder.
		var clean []byte
		for _, c := range data[:end] {
			if !docSpace(c) {
				clean = append(clean, c)
			}
		}
		result, err = readDecodeLimit(ascii85.NewDecoder(bytes.NewReader(clean)), limit)
	case "RunLengthDecode", "RL":
		terminated := false
		for pos := 0; pos < len(data); {
			n := int(data[pos])
			pos++
			if n == 128 {
				terminated = true
				break
			}
			count := n + 1
			if n > 128 {
				count = 257 - n
			}
			if int64(len(result))+int64(count) > limit {
				return nil, errors.New("decoded byte limit exceeded")
			}
			if n < 128 {
				if count > len(data)-pos {
					return nil, io.ErrUnexpectedEOF
				}
				result = append(result, data[pos:pos+count]...)
				pos += count
			} else {
				if pos >= len(data) {
					return nil, io.ErrUnexpectedEOF
				}
				for i := 0; i < count; i++ {
					result = append(result, data[pos])
				}
				pos++
			}
		}
		if !terminated {
			return nil, errors.New("missing RunLength end marker")
		}
	default:
		return nil, fmt.Errorf("unsupported PDF filter /%s", name)
	}
	if err != nil {
		return nil, err
	}
	if int64(len(result)) > limit {
		return nil, errors.New("decoded byte limit exceeded")
	}
	return result, nil
}

func readDecodeLimit(r io.Reader, limit int64) ([]byte, error) {
	// Read one extra byte to detect exhaustion without overflowing MaxInt64.
	n := limit
	if n < math.MaxInt64 {
		n++
	}
	return io.ReadAll(io.LimitReader(r, n))
}

func predictorInt(dict model.Dictionary, key model.Name, fallback int64) (int64, error) {
	o, err := dict.Get(key)
	if errors.Is(err, model.ErrMissingKey) {
		return fallback, nil
	}
	if err != nil {
		return 0, err
	}
	return model.Int(o)
}

func applyPredictor(data []byte, params *model.Object, limit int64) ([]byte, error) {
	if params == nil {
		return data, nil
	}
	if _, ok := params.Value.(model.Null); ok {
		return data, nil
	}
	dict, ok := params.Value.(model.Dictionary)
	if !ok {
		return nil, errors.New("predictor parameters must be dictionary or null")
	}
	predictor, err := predictorInt(dict, "Predictor", 1)
	if err != nil {
		return nil, err
	}
	if predictor == 1 {
		return data, nil
	}
	colors, err := predictorInt(dict, "Colors", 1)
	if err != nil {
		return nil, err
	}
	columns, err := predictorInt(dict, "Columns", 1)
	if err != nil {
		return nil, err
	}
	bits, err := predictorInt(dict, "BitsPerComponent", 8)
	if err != nil {
		return nil, err
	}
	if colors <= 0 || columns <= 0 || (bits != 1 && bits != 2 && bits != 4 && bits != 8 && bits != 16) || colors > math.MaxInt64/columns || colors*columns > (math.MaxInt64-7)/bits {
		return nil, errors.New("invalid/over-limit predictor geometry")
	}
	samples := colors * columns
	rowSize := (samples*bits + 7) / 8
	if rowSize > limit || uint64(rowSize) >= uint64(^uint(0)>>1) {
		return nil, errors.New("predictor row exceeds byte limit")
	}
	rowBytes := int(rowSize)
	bpp := int((colors*bits + 7) / 8)
	if predictor == 2 {
		if rowBytes == 0 || len(data)%rowBytes != 0 {
			return nil, errors.New("truncated TIFF predictor row")
		}
		out := bytes.Clone(data)
		mask := uint32(1<<bits) - 1
		for row := 0; row < len(out); row += rowBytes {
			for sample := colors; sample < samples; sample++ {
				at := int(sample * bits)
				previous := int((sample - colors) * bits)
				value := (sampleBits(out[row:row+rowBytes], at, int(bits)) + sampleBits(out[row:row+rowBytes], previous, int(bits))) & mask
				putSampleBits(out[row:row+rowBytes], at, int(bits), value)
			}
		}
		return out, nil
	}
	if predictor < 10 || predictor > 15 {
		return nil, fmt.Errorf("unsupported predictor %d", predictor)
	}
	if rowBytes == 0 || len(data)%(rowBytes+1) != 0 {
		return nil, errors.New("truncated PNG predictor row")
	}
	rows := len(data) / (rowBytes + 1)
	out := make([]byte, rows*rowBytes)
	for row := 0; row < rows; row++ {
		filter := data[row*(rowBytes+1)]
		if filter > 4 {
			return nil, errors.New("invalid PNG predictor filter")
		}
		for i := 0; i < rowBytes; i++ {
			var left, up, upperLeft byte
			if i >= bpp {
				left = out[row*rowBytes+i-bpp]
			}
			if row > 0 {
				up = out[(row-1)*rowBytes+i]
				if i >= bpp {
					upperLeft = out[(row-1)*rowBytes+i-bpp]
				}
			}
			v := data[row*(rowBytes+1)+1+i]
			switch filter {
			case 1:
				v += left
			case 2:
				v += up
			case 3:
				v += byte((int(left) + int(up)) / 2)
			case 4:
				v += paeth(left, up, upperLeft)
			}
			out[row*rowBytes+i] = v
		}
	}
	return out, nil
}
func sampleBits(data []byte, start, bits int) uint32 {
	var n uint32
	for i := 0; i < bits; i++ {
		at := start + i
		n = n<<1 | uint32((data[at/8]>>uint(7-at%8))&1)
	}
	return n
}
func putSampleBits(data []byte, start, bits int, n uint32) {
	for i := 0; i < bits; i++ {
		at := start + i
		mask := byte(1 << uint(7-at%8))
		if n&(1<<uint(bits-1-i)) != 0 {
			data[at/8] |= mask
		} else {
			data[at/8] &= ^mask
		}
	}
}
func paeth(a, b, c byte) byte {
	p := int(a) + int(b) - int(c)
	pa, pb, pc := absInt(p-int(a)), absInt(p-int(b)), absInt(p-int(c))
	if pa <= pb && pa <= pc {
		return a
	}
	if pb <= pc {
		return b
	}
	return c
}
func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
