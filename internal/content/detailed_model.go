package content

import (
	"github.com/MyungSub0519/gopd/internal/model"
	"github.com/MyungSub0519/gopd/internal/structure"
)

// DetailedPDF is the full result of interpreting a document's page contents.
//
// Content is stored document-wide rather than per page: Texts, Graphics and
// Images are flat slices, and each DetailedPage lists the indexes belonging to
// it through Items. That layout keeps an element in one place even when it is
// drawn by a form XObject that several pages share.
//
// Elements are in execution order, which is the order the content stream drew
// them. That is not reading order: nothing here reconstructs columns, sorts
// text by position, or infers where a paragraph begins.
//
// Treat the result and the Document it points at as read-only.
type DetailedPDF struct {
	Pages          []DetailedPage
	Texts          []DetailedText
	Graphics       []DetailedGraphic
	Images         []DetailedImage
	ImageResources []ImageResource
	Fonts          []Font
	Annotations    []Annotation
	Structure      *model.Structure
	Document       *structure.Document
	Diagnostics    []model.Diagnostic
	// diagnosticPages records which page each diagnostic was emitted from,
	// parallel to Diagnostics. It is kept separate because a diagnostic's
	// span may lie in a form stream that several pages reuse, so the span
	// alone cannot say which page was being drawn at the time.
	diagnosticPages []int
}

// ElementKind names which of the document-wide slices an ElementRef points at.
type ElementKind uint8

const (
	ElementText ElementKind = iota + 1
	ElementGraphic
	ElementImage
)

// ElementRef points at one element in the document-wide slices, naming both
// which slice and which index.
type ElementRef struct {
	Kind  ElementKind
	Index int
}

// DetailedPage is one page: its geometry, its resources, the operations that
// drew it, and references to the elements those operations produced.
//
// MediaBox and CropBox may have been inherited from an ancestor node rather
// than stated here. Rotate and UserUnit are recorded but not applied, so
// coordinates elsewhere are in unrotated user space.
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

// FormCall is one frame of the form XObject call stack: which form was
// invoked, where its content lives, and the Do operator that invoked it.
type FormCall struct {
	ObjectID model.ObjectID
	Span     model.Span
	Call     model.Span
}

// ElementSource records where an element came from.
//
// FormPath is what disambiguates a shared form: the same bytes drawn from two
// pages produce two elements whose Spans are identical and whose FormPath and
// Page differ.
type ElementSource struct {
	Page       int
	Spans      []model.Span
	Operations []int // indexes into the page's flattened Operations slice
	FormPath   []FormCall
}

// Operation is one content stream operator with the operands it was given.
//
// Every operator is recorded, including those the interpreter could not act
// on, so that the original instruction sequence stays inspectable even where
// the effect was not reproduced.
type Operation struct {
	Operator string
	Operands []model.Object
	Span     model.Span // operator token; operands retain their own source spans
	FormPath []FormCall
}

// DetailedPathSegment is one segment of a path, with Operator naming the
// construction operator that produced it.
//
// Points are already in page space, transformed by the CTM that was in force
// when the segment was constructed, which is not necessarily the one in force
// when the path was painted.
type DetailedPathSegment struct {
	Operator string
	Points   []model.Point // transformed to page user space when the segment is constructed
	Span     model.Span
}

// DetailedGraphic is one path-painting operation.
//
// Paint is the operator that painted it, and the three flags decompose what
// that operator meant: whether it stroked, whether it filled, and whether
// filling used the even-odd rule rather than the nonzero winding rule.
type DetailedGraphic struct {
	Source                ElementSource
	Segments              []DetailedPathSegment
	Paint                 string
	Stroke, Fill, EvenOdd bool
	State                 GraphicsState
}

// Glyph is one character code from a show operation, after decoding.
//
// Origin and Advance are in page space. The two completeness flags travel with
// the glyph because they vary within a single show: one code may decode and
// measure cleanly while the next has no mapping and no width.
type Glyph struct {
	Code           []byte
	Unicode        string
	Origin         model.Point
	Advance        model.Point
	DecodeComplete bool
	WidthKnown     bool
}

// DetailedText is one text-showing operation.
//
// Matrix is the text matrix in page space as it stood before the operation ran
// and before font scaling is applied, so it is where the text began rather
// than where it ended.
//
// Unicode is the whole operation's text; Glyphs breaks it down per code when
// per-glyph positions are needed. Font indexes into DetailedPDF.Fonts, or is
// -1 when no font had been selected, which is itself a sign of malformed
// content.
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

// DetailedImage is one placement of an image: an occurrence of a Do operator,
// not the image data.
//
// Resource indexes into DetailedPDF.ImageResources, which is where the shared
// data lives, so an image drawn ten times is ten DetailedImages and one
// ImageResource.
type DetailedImage struct {
	Source   ElementSource
	Resource int
	Matrix   model.Matrix
	State    GraphicsState
}

// ImageResource is an image XObject's metadata and the location of its bytes.
//
// Width and Height are in pixels, not page units; the page-space size comes
// from the placement's matrix. The payload is left encoded: this library does
// not decode image codecs, so Stream locates the bytes rather than holding
// them decoded.
type ImageResource struct {
	Object           model.Object
	ID               model.ObjectID
	Stream           model.Stream
	Width, Height    int
	BitsPerComponent int
	ColorSpace       model.Object
	ImageMask        bool
}

// Annotation is one page annotation: a link, a note, a form field and so on.
//
// Only the identifying entries are lifted out; Object holds the whole
// dictionary for callers that need more. Appearance streams are not
// interpreted, so an annotation contributes no text or graphics elements.
type Annotation struct {
	Page    int
	Object  model.Object
	Subtype model.Name
	Rect    model.Rect
}
