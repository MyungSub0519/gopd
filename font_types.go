package gopd

// FontInfo contains display metadata, shared by original font resource identity.
// FontSize belongs to Text because the same font can be used at different sizes.
type FontInfo struct {
	BaseFont Name
	Subtype  Name
}

type Font struct {
	Object               Object
	ID                   ObjectID
	Subtype              Name
	BaseFont             Name
	Encoding             Object
	ToUnicode            *CMap
	Embedded             *Object
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
