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

// FontInfo contains display metadata, shared by original font resource identity.
// FontSize belongs to Text because the same font can be used at different sizes.
type FontInfo struct {
	BaseFont Name
	Subtype  Name
}

// Details returns the already parsed detailed snapshot without re-reading the
// file. Detailed content slices use document-wide indexes; detailed Page.Items
// associates those indexes with pages. Treat the result as read-only; lazy
// Document methods are not safe for concurrent calls.
func (p *PDF) Details() *DetailedPDF {
	if p == nil {
		return nil
	}
	return p.details
}

func basicPDF(detail *DetailedPDF) *PDF {
	if detail == nil {
		return nil
	}
	p := &PDF{
		Texts:    make([][]Text, len(detail.Pages)),
		Graphics: make([][]Graphic, len(detail.Pages)),
		details:  detail,
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
			Page:             text.Source.Page,
			Unicode:          text.Unicode,
			Font:             font,
			FontSize:         text.FontSize,
			Matrix:           text.Matrix,
			RenderingMode:    text.RenderingMode,
			Style:            basicStyle(text.State),
			DecodeComplete:   text.DecodeComplete,
			PositionComplete: text.PositionComplete,
		})
	}
	for _, graphic := range detail.Graphics {
		segments := make([]PathSegment, len(graphic.Segments))
		for j, segment := range graphic.Segments {
			segments[j] = PathSegment{Operator: segment.Operator, Points: append([]Point{}, segment.Points...)}
		}
		page := graphic.Source.Page
		p.Graphics[page] = append(p.Graphics[page], Graphic{
			Page:     graphic.Source.Page,
			Segments: segments,
			Paint:    graphic.Paint,
			Stroke:   graphic.Stroke,
			Fill:     graphic.Fill,
			EvenOdd:  graphic.EvenOdd,
			Style:    basicStyle(graphic.State),
		})
	}
	return p
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
