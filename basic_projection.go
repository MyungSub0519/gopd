package gopd

func basicPDF(detail *DetailedPDF) *PDF {
	if detail == nil {
		return nil
	}
	p := &PDF{
		Texts: make([][]Text, len(detail.Pages)), Graphics: make([][]Graphic, len(detail.Pages)),
		details: detail,
	}
	for page := range detail.Pages {
		p.Texts[page] = []Text{}
		p.Graphics[page] = []Graphic{}
	}
	fonts := make([]*FontInfo, len(detail.Fonts))
	for i, font := range detail.Fonts {
		fonts[i] = &FontInfo{BaseFont: font.BaseFont, Subtype: font.Subtype}
	}
	for _, text := range detail.Texts {
		var font *FontInfo
		if text.Font >= 0 && text.Font < len(fonts) {
			font = fonts[text.Font]
		}
		page := text.Source.Page
		p.Texts[page] = append(p.Texts[page], Text{
			Page: text.Source.Page, Unicode: text.Unicode, Font: font,
			FontSize: text.FontSize, Matrix: text.Matrix, RenderingMode: text.RenderingMode,
			Style: basicStyle(text.State), DecodeComplete: text.DecodeComplete, PositionComplete: text.PositionComplete,
		})
	}
	for _, graphic := range detail.Graphics {
		segments := make([]PathSegment, len(graphic.Segments))
		for j, segment := range graphic.Segments {
			segments[j] = PathSegment{Operator: segment.Operator, Points: append([]Point{}, segment.Points...)}
		}
		page := graphic.Source.Page
		p.Graphics[page] = append(p.Graphics[page], Graphic{
			Page: graphic.Source.Page, Segments: segments, Paint: graphic.Paint,
			Stroke: graphic.Stroke, Fill: graphic.Fill, EvenOdd: graphic.EvenOdd, Style: basicStyle(graphic.State),
		})
	}
	return p
}

func basicStyle(state GraphicsState) PaintStyle {
	stroke, fill := state.Stroke, state.Fill
	stroke.Components = append([]float64{}, stroke.Components...)
	fill.Components = append([]float64{}, fill.Components...)
	return PaintStyle{
		Stroke: stroke, Fill: fill, LineWidth: state.LineWidth,
		LineCap: state.LineCap, LineJoin: state.LineJoin, MiterLimit: state.MiterLimit,
		Dash: append([]float64{}, state.Dash...), DashPhase: state.DashPhase,
		StrokeAlpha: state.StrokeAlpha, FillAlpha: state.FillAlpha, BlendMode: state.BlendMode,
		Clipped: len(state.Clip) > 0, Complete: state.Complete,
	}
}
