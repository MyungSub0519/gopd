package gopd

// DetailedPDF is a semantic snapshot. Slices and the underlying Document are read-only
// by convention. Elements follow content execution order, not reading order.
type DetailedPDF struct {
	Pages           []DetailedPage
	Texts           []DetailedText
	Graphics        []DetailedGraphic
	Images          []DetailedImage
	ImageResources  []ImageResource
	Fonts           []Font
	Annotations     []Annotation
	Structure       *Structure
	Document        *Document
	Diagnostics     []Diagnostic
	diagnosticPages []int // emission context, including reused Form streams
}

type ElementKind uint8

const (
	ElementText ElementKind = iota + 1
	ElementGraphic
	ElementImage
)

type ElementRef struct {
	Kind  ElementKind
	Index int
}

type DetailedPage struct {
	Index       int
	Object      Object
	MediaBox    Rect
	CropBox     Rect
	Rotate      int
	UserUnit    float64
	Resources   Dictionary
	Contents    []Object
	Items       []ElementRef
	Operations  []Operation
	Annotations []int
	// Complete is false when a content effect could not be interpreted.
	// True does not claim complete rendering, glyph outlines, or OCR support.
	Complete bool
}

type FormCall struct {
	ObjectID ObjectID
	Span     Span
	Call     Span
}

type ElementSource struct {
	Page       int
	Spans      []Span
	Operations []int // indexes into the page's flattened Operations slice
	FormPath   []FormCall
}

type Operation struct {
	Operator string
	Operands []Object
	Span     Span // operator token; operands retain their own source spans
	FormPath []FormCall
}

type DetailedPathSegment struct {
	Operator string
	Points   []Point // transformed to page user space when the segment is constructed
	Span     Span
}

type DetailedGraphic struct {
	Source                ElementSource
	Segments              []DetailedPathSegment
	Paint                 string
	Stroke, Fill, EvenOdd bool
	State                 GraphicsState
}

type Glyph struct {
	Code           []byte
	Unicode        string
	Origin         Point
	Advance        Point
	DecodeComplete bool
	WidthKnown     bool
}

type DetailedText struct {
	Source           ElementSource
	RawCodes         []byte
	Unicode          string
	DecodeComplete   bool
	PositionComplete bool
	Font             int // index in DetailedPDF.Fonts; -1 when no font was selected
	FontSize         float64
	Matrix           Matrix // page-space text matrix before the show operation, before font scaling
	RenderingMode    int
	Glyphs           []Glyph
	State            GraphicsState
}

// DetailedImage is one execution of an image XObject; bytes belong to ImageResource.
type DetailedImage struct {
	Source   ElementSource
	Resource int
	Matrix   Matrix
	State    GraphicsState
}

type ImageResource struct {
	Object           Object
	ID               ObjectID
	Stream           Stream
	Width, Height    int
	BitsPerComponent int
	ColorSpace       Object
	ImageMask        bool
}

type Annotation struct {
	Page    int
	Object  Object
	Subtype Name
	Rect    Rect
}
