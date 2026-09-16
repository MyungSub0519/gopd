package content

import (
	"github.com/MyungSub0519/gopd/internal/model"
)

// FontInfo contains display metadata, shared by original font resource identity.
// FontSize belongs to Text because the same font can be used at different sizes.
type FontInfo struct {
	BaseFont model.Name
	Subtype  model.Name
}

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

type CodeSpace struct{ Low, High []byte }

type CMap struct {
	CodeSpaces []CodeSpace
	Mappings   map[string]string
}
