package content

import (
	"github.com/MyungSub0519/gopd/internal/model"
)

// FontInfo is the small, shareable part of a font: enough to identify it when
// reporting text.
//
// Size is not here, because the same font resource is routinely used at many
// sizes; it belongs to each Text instead.
type FontInfo struct {
	BaseFont model.Name
	Subtype  model.Name
}

// Font is a font resource together with what the interpreter worked out about
// decoding and measuring text drawn with it.
//
// The three capability flags say what can be trusted rather than what was
// attempted. DecodeSupported means character codes can be turned into text at
// all; WidthsKnown means glyph advances are available, without which positions
// after the first glyph are guesses; PositioningSupported means the writing
// mode and encoding are ones this library can follow. A font can decode
// correctly and still not be positionable, which is why they are separate.
type Font struct {
	Object               model.Object
	ID                   model.ObjectID
	Subtype              model.Name
	BaseFont             model.Name
	Encoding             model.Object
	ToUnicode            *CMap
	Embedded             *model.Object
	DecodeSupported      bool
	WidthsKnown          bool
	PositioningSupported bool
	widths               map[uint32]float64
	defaultWidth         float64
	defaultWidthKnown    bool
	simpleEncoding       map[byte]string
	composite            bool
	vertical             bool
}

// CodeSpace is one codespace range of a CMap: byte sequences from Low to High,
// compared position by position, all of the same length.
//
// Codespaces are what make variable-width encodings decodable. A CMap may
// declare one-byte and two-byte ranges together, and the first byte decides
// which applies, so a decoder must consult them before it knows how many bytes
// the next character code occupies.
type CodeSpace struct{ Low, High []byte }

// CMap maps character codes to Unicode text.
//
// Mappings is keyed by the raw code bytes as a string, and a value may be
// several runes long, since one code can stand for a ligature or a composed
// sequence. This type covers /ToUnicode CMaps only; the predefined CMaps that
// select glyphs in a composite font are not implemented.
type CMap struct {
	CodeSpaces []CodeSpace
	Mappings   map[string]string
}
