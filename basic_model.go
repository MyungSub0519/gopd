package gopd

// PDF is the basic result returned by ParsePDF. The outer slice index is the
// zero-based page index; empty pages contain non-nil empty slices. Each inner
// slice follows content execution order for its kind, not reading order.
// Treat results and shared resources as read-only. Detailed analysis is retained
// privately and excluded from JSON encoding.
type PDF struct {
	Texts    [][]Text
	Graphics [][]Graphic

	details *DetailedPDF
}

// Page describes page geometry and references into the PDF's content slices.
// Complete reports interpreter diagnostics, not full rendering support.
type Page struct {
	Index    int
	MediaBox Rect
	CropBox  Rect
	Rotate   int
	UserUnit float64
	Items    []ElementRef
	Complete bool
}

// Text is one text-show operation. Matrix is in unrotated page user space,
// before font scaling. Glyph positions and source bytes are available in Details.
type Text struct {
	Page             int
	Unicode          string
	Font             *FontInfo
	FontSize         float64
	Matrix           Matrix
	RenderingMode    int
	Style            PaintStyle
	DecodeComplete   bool
	PositionComplete bool
}

// FontInfo contains display metadata, shared by original font resource identity.
// FontSize belongs to Text because the same font can be used at different sizes.
type FontInfo struct {
	BaseFont Name
	Subtype  Name
}

// Graphic is one path painting operation; it is not a whole chart or figure.
type Graphic struct {
	Page                  int
	Segments              []PathSegment
	Paint                 string
	Stroke, Fill, EvenOdd bool
	Style                 PaintStyle
}

// PathSegment points are already transformed into unrotated page user space.
type PathSegment struct {
	Operator string
	Points   []Point
}

// Image is one placement of a shared image resource.
type Image struct {
	Page     int
	Resource *ImageInfo
	Matrix   Matrix
	Style    PaintStyle
}

// ImageInfo describes image pixels, not page-space display dimensions.
// ColorSpace is a name or family (e.g. Indexed); parameters remain in Details.
// An empty ColorSpace means no name was available, including image masks.
type ImageInfo struct {
	Width, Height    int
	BitsPerComponent int
	ColorSpace       Name
	ImageMask        bool
}

// PaintStyle omits transformation and clipping geometry. Clipped indicates that
// clipping paths exist in the detailed state. Complete retains the interpreter's
// effect support flag; neither flag promises a complete rendering description.
type PaintStyle struct {
	Stroke, Fill           Color
	LineWidth              float64
	LineCap, LineJoin      int
	MiterLimit             float64
	Dash                   []float64
	DashPhase              float64
	StrokeAlpha, FillAlpha float64
	BlendMode              Name
	Clipped                bool
	Complete               bool
}

// ParseDiagnostic omits byte locations. Page is zero-based, or -1 for a
// document-wide issue. Detailed diagnostics retain their original source spans.
type ParseDiagnostic struct {
	Page     int
	Severity Severity
	Code     string
	Message  string
}
