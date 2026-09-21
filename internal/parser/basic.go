package parser

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
