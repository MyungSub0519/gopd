package content

import (
	"github.com/MyungSub0519/gopd/internal/model"
)

// Color is a colour as the content stream set it.
//
// Components are the raw operands, uninterpreted: their meaning depends on
// Space, and no conversion to a device colour is performed. Pattern names a
// pattern resource when the colour space is /Pattern, in which case Components
// may also carry the underlying colour of an uncoloured pattern.
type Color struct {
	Space      model.Name
	Components []float64
	Pattern    model.Name
}

// PaintStyle is the flattened appearance of one painting operation, for
// callers that want to know how something was drawn without walking the full
// graphics state.
//
// It deliberately omits geometry: the transformation matrix and the clipping
// path are not here, only a Clipped flag saying that clipping was in effect.
// Complete carries the interpreter's own verdict on whether it understood
// every effect that applied. Neither flag promises that this is enough to
// reproduce the drawing; use GraphicsState when the geometry matters.
type PaintStyle struct {
	Stroke, Fill           Color
	LineWidth              float64
	LineCap, LineJoin      int
	MiterLimit             float64
	Dash                   []float64
	DashPhase              float64
	StrokeAlpha, FillAlpha float64
	BlendMode              model.Name
	Clipped                bool
	Complete               bool
}

// GraphicsState is the full graphics state in effect when an element was
// drawn, captured at that moment.
//
// It is a snapshot, not a reference: the q and Q operators save and restore
// this whole structure, so an element keeps the state as it stood rather than
// as it ended up.
type GraphicsState struct {
	// CTM maps user space to page space at the time of capture.
	CTM                    model.Matrix
	LineWidth              float64
	LineCap, LineJoin      int
	MiterLimit             float64
	Dash                   []float64
	DashPhase              float64
	Stroke, Fill           Color
	StrokeAlpha, FillAlpha float64
	BlendMode              model.Name
	Clip                   []ClipPath
	RenderingIntent        model.Name
	Complete               bool
}

// ClipPath is one clipping path that was in effect.
//
// Clipping accumulates: each W operator intersects a new path with whatever
// was already clipping, so GraphicsState.Clip holds the whole stack and the
// effective region is the intersection of all of them. They are kept separate
// rather than intersected because computing the intersection is a rendering
// operation.
type ClipPath struct {
	Segments []DetailedPathSegment
	EvenOdd  bool
}
