package gopd

// ContentKind selects independently emitted content categories.
type ContentKind uint8

const (
	ContentText ContentKind = 1 << iota
	ContentGraphics
	ContentImages
	ContentAnnotations
	ContentAll = ContentText | ContentGraphics | ContentImages | ContentAnnotations
)

// ExtractOptions controls work and retained output. Zero options extract Unicode
// text and compact font metadata. Glyphs requires text and enables Positions.
// Positions use unrotated page user space, as in the detailed API.
type ExtractOptions struct {
	Content     ContentKind
	Positions   bool
	Styles      bool
	Glyphs      bool
	Provenance  bool
	ReadOptions ReadOptions
}

// Extraction contains only requested output, in content execution order within
// each kind. It does not reconstruct reading order or render pixels. Treat
// results as read-only. Without Provenance, no Document or DetailedPDF is retained.
type Extraction struct {
	Content        ContentKind
	Pages          []ExtractedPage
	ImageResources []ExtractedImageResource `json:",omitempty"`
	Diagnostics    []ExtractionDiagnostic   `json:",omitempty"`
	// Document is present only with Provenance. Its lazy methods are not
	// concurrent-safe. It owns the snapshot; no Close call is required.
	Document *Document `json:"-"`
	options  ExtractOptions
}

// ExtractedPage groups selected elements. An omitted category was not requested
// or has no elements; Extraction.Content distinguishes those cases.
type ExtractedPage struct {
	Index             int
	MediaBox, CropBox Rect
	Rotate            int
	UserUnit          float64
	Texts             []ExtractedText       `json:",omitempty"`
	Graphics          []ExtractedGraphic    `json:",omitempty"`
	Images            []ExtractedImage      `json:",omitempty"`
	Annotations       []ExtractedAnnotation `json:",omitempty"`
	Operations        []Operation           `json:",omitempty"`
	// Complete concerns requested content, not validation of skipped resources.
	// Always check the returned error, including errors before a page is added.
	Complete bool
}

// ExtractedText is one text-show operation, with independently selected details.
type ExtractedText struct {
	Unicode        string
	Font           *FontInfo `json:",omitempty"`
	DecodeComplete bool
	Position       *TextPosition  `json:",omitempty"`
	Style          *PaintStyle    `json:",omitempty"`
	Glyphs         []Glyph        `json:",omitempty"`
	Source         *ElementSource `json:",omitempty"`
}

// TextPosition describes text before the show operation, before font scaling.
type TextPosition struct {
	FontSize      float64
	Matrix        Matrix
	RenderingMode int
	Complete      bool
}

// ExtractedGraphic is a painted path, not a complete chart or figure. Geometry
// is always in unrotated page user space, independent of Positions.
type ExtractedGraphic struct {
	Segments              []PathSegment
	Paint                 string
	Stroke, Fill, EvenOdd bool
	Style                 *PaintStyle    `json:",omitempty"`
	Source                *ElementSource `json:",omitempty"`
}

// ExtractedImage is one placement of a shared image resource.
type ExtractedImage struct {
	Resource int
	Matrix   *Matrix        `json:",omitempty"`
	Style    *PaintStyle    `json:",omitempty"`
	Source   *ElementSource `json:",omitempty"`
}

// ExtractedImageResource retains image metadata, not decoded pixels. ColorSpace
// preserves the PDF value (including complex color-space parameters). Object is
// present only with Provenance; its Stream can be used with Extraction.Document.
type ExtractedImageResource struct {
	ID               ObjectID
	Width, Height    int
	BitsPerComponent int
	ColorSpace       Object
	ImageMask        bool
	Object           *Object `json:",omitempty"`
}

// ExtractedAnnotation contains annotation metadata; appearance streams are not executed.
type ExtractedAnnotation struct {
	Subtype Name
	Rect    Rect
	Source  *Span `json:",omitempty"`
}

// ExtractionDiagnostic reports a requested interpretation limitation. Page is
// zero based, or -1 for a document-level issue. Span needs Provenance to resolve.
type ExtractionDiagnostic struct {
	Severity      Severity
	Code, Message string
	Page          int
	Span          *Span `json:",omitempty"`
}
