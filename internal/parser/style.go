package parser

import (
	"github.com/MyungSub0519/gopd/internal/pdfmodel"
)

type Color struct {
	Space      pdfmodel.Name
	Components []float64
	Pattern    pdfmodel.Name
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
	BlendMode              pdfmodel.Name
	Clipped                bool
	Complete               bool
}

type GraphicsState struct {
	CTM                    pdfmodel.Matrix
	LineWidth              float64
	LineCap, LineJoin      int
	MiterLimit             float64
	Dash                   []float64
	DashPhase              float64
	Stroke, Fill           Color
	StrokeAlpha, FillAlpha float64
	BlendMode              pdfmodel.Name
	Clip                   []ClipPath
	RenderingIntent        pdfmodel.Name
	Complete               bool
}

type ClipPath struct {
	Segments []DetailedPathSegment
	EvenOdd  bool
}

func basicStyle(state GraphicsState) PaintStyle {
	stroke, fill := state.Stroke, state.Fill
	stroke.Components = append([]float64{}, stroke.Components...)
	fill.Components = append([]float64{}, fill.Components...)
	return PaintStyle{
		Stroke:      stroke,
		Fill:        fill,
		LineWidth:   state.LineWidth,
		LineCap:     state.LineCap,
		LineJoin:    state.LineJoin,
		MiterLimit:  state.MiterLimit,
		Dash:        append([]float64{}, state.Dash...),
		DashPhase:   state.DashPhase,
		StrokeAlpha: state.StrokeAlpha,
		FillAlpha:   state.FillAlpha,
		BlendMode:   state.BlendMode,
		Clipped:     len(state.Clip) > 0,
		Complete:    state.Complete,
	}
}

func (b *semanticBuilder) extractStyle(state GraphicsState) *PaintStyle {
	if !b.wantStyles() {
		return nil
	}
	style := basicStyle(state)
	return &style
}
