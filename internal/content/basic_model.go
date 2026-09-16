package content

import (
	"github.com/MyungSub0519/gopd/internal/model"
)

// PDF is the simplified result: text and graphics already grouped by page.
//
// The outer index is the zero-based page number, and a page with no content
// still has a non-nil empty slice, so the shape of the result always matches
// the page count. Within a page, elements are in content execution order, not
// reading order.
//
// This is a projection of a DetailedPDF, which is retained and reachable
// through Details. It is not a cheaper parse: the full interpretation happened
// either way. The retained detail is unexported so that encoding a PDF to JSON
// does not drag the whole document along.
type PDF struct {
	Texts    [][]Text
	Graphics [][]Graphic

	details *DetailedPDF
}

// Text is one text-showing operation, flattened.
//
// Matrix is in unrotated page user space, before font scaling. Per-glyph
// positions and the original bytes are not here; use Details for those.
//
// DecodeComplete and PositionComplete are the honesty flags: false means the
// text or its placement is partly guesswork, usually because the font had no
// usable encoding or no widths.
type Text struct {
	Page             int
	Unicode          string
	Font             *FontInfo
	FontSize         float64
	Matrix           model.Matrix
	RenderingMode    int
	Style            PaintStyle
	DecodeComplete   bool
	PositionComplete bool
}

// Graphic is one path-painting operation.
//
// It is a single fill or stroke, not a figure: a chart drawn as hundreds of
// separate paths yields hundreds of Graphics, and nothing here groups them.
type Graphic struct {
	Page                  int
	Segments              []PathSegment
	Paint                 string
	Stroke, Fill, EvenOdd bool
	Style                 PaintStyle
}

// PathSegment is one segment of a path, with Operator naming the construction
// operator that produced it.
//
// Points have already been transformed into unrotated page user space, so no
// further matrix needs to be applied to place them.
type PathSegment struct {
	Operator string
	Points   []model.Point
}
