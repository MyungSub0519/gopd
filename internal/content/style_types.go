package content

import (
	"github.com/MyungSub0519/gopd/internal/model"
)

type Color struct {
	Space      model.Name
	Components []float64
	Pattern    model.Name
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
	BlendMode              model.Name
	Clipped                bool
	Complete               bool
}

type GraphicsState struct {
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

type ClipPath struct {
	Segments []DetailedPathSegment
	EvenOdd  bool
}
