package content

import (
	"github.com/MyungSub0519/gopd/internal/model"
	"github.com/MyungSub0519/gopd/internal/structure"
)

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
	Structure       *model.Structure
	Document        *structure.Document
	Diagnostics     []model.Diagnostic
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
	Object      model.Object
	MediaBox    model.Rect
	CropBox     model.Rect
	Rotate      int
	UserUnit    float64
	Resources   model.Dictionary
	Contents    []model.Object
	Items       []ElementRef
	Operations  []Operation
	Annotations []int
	// Complete is false when a content effect could not be interpreted.
	// True does not claim complete rendering, glyph outlines, or OCR support.
	Complete bool
}

type FormCall struct {
	ObjectID model.ObjectID
	Span     model.Span
	Call     model.Span
}

type ElementSource struct {
	Page       int
	Spans      []model.Span
	Operations []int // indexes into the page's flattened Operations slice
	FormPath   []FormCall
}

type Operation struct {
	Operator string
	Operands []model.Object
	Span     model.Span // operator token; operands retain their own source spans
	FormPath []FormCall
}

type DetailedPathSegment struct {
	Operator string
	Points   []model.Point // transformed to page user space when the segment is constructed
	Span     model.Span
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
	Origin         model.Point
	Advance        model.Point
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
	Matrix           model.Matrix // page-space text matrix before the show operation, before font scaling
	RenderingMode    int
	Glyphs           []Glyph
	State            GraphicsState
}

// DetailedImage is one execution of an image XObject; bytes belong to ImageResource.
type DetailedImage struct {
	Source   ElementSource
	Resource int
	Matrix   model.Matrix
	State    GraphicsState
}

type ImageResource struct {
	Object           model.Object
	ID               model.ObjectID
	Stream           model.Stream
	Width, Height    int
	BitsPerComponent int
	ColorSpace       model.Object
	ImageMask        bool
}

type Annotation struct {
	Page    int
	Object  model.Object
	Subtype model.Name
	Rect    model.Rect
}
