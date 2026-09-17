package gopd

// Emission is the only boundary that chooses a retained output model. The
// interpreter's transient state and resource caches are shared by both APIs.
func (c *contentInterpreter) emitText(text DetailedText) {
	if c.b.extract == nil {
		c.item(ElementText, len(c.b.pdf.Texts))
		c.b.pdf.Texts = append(c.b.pdf.Texts, text)
		return
	}
	b := c.b
	output := ExtractedText{Unicode: text.Unicode, DecodeComplete: text.DecodeComplete}
	if text.Font >= 0 {
		if b.fontInfos == nil {
			b.fontInfos = make(map[int]*FontInfo)
		}
		info, ok := b.fontInfos[text.Font]
		if !ok {
			font := b.pdf.Fonts[text.Font]
			info = &FontInfo{BaseFont: font.BaseFont, Subtype: font.Subtype}
			b.fontInfos[text.Font] = info
		}
		output.Font = info
	}
	if b.wantPositions() {
		output.Position = &TextPosition{
			FontSize: text.FontSize, Matrix: text.Matrix,
			RenderingMode: text.RenderingMode, Complete: text.PositionComplete,
		}
	}
	if b.wantGlyphs() {
		output.Glyphs = text.Glyphs
	}
	output.Style = b.extractStyle(text.State)
	if b.wantProvenance() {
		source := text.Source
		output.Source = &source
	}
	page := &b.extract.Pages[c.page]
	page.Texts = append(page.Texts, output)
}

func (c *contentInterpreter) emitGraphic(graphic DetailedGraphic) {
	if c.b.extract == nil {
		c.item(ElementGraphic, len(c.b.pdf.Graphics))
		c.b.pdf.Graphics = append(c.b.pdf.Graphics, graphic)
		return
	}
	segments := make([]PathSegment, len(graphic.Segments))
	for i, segment := range graphic.Segments {
		// Path points are immutable after construction; the path accumulator is
		// replaced after paint, so output can own these without another copy.
		segments[i] = PathSegment{Operator: segment.Operator, Points: segment.Points}
	}
	output := ExtractedGraphic{
		Segments: segments, Paint: graphic.Paint, Stroke: graphic.Stroke,
		Fill: graphic.Fill, EvenOdd: graphic.EvenOdd, Style: c.b.extractStyle(graphic.State),
	}
	if c.b.wantProvenance() {
		source := graphic.Source
		output.Source = &source
	}
	page := &c.b.extract.Pages[c.page]
	page.Graphics = append(page.Graphics, output)
}

func (b *semanticBuilder) emitImageResource(resource ImageResource) int {
	if b.extract == nil {
		index := len(b.pdf.ImageResources)
		b.pdf.ImageResources = append(b.pdf.ImageResources, resource)
		return index
	}
	output := ExtractedImageResource{
		ID: resource.ID, Width: resource.Width, Height: resource.Height,
		BitsPerComponent: resource.BitsPerComponent, ColorSpace: resource.ColorSpace,
		ImageMask: resource.ImageMask,
	}
	if b.wantProvenance() {
		object := resource.Object
		output.Object = &object
	}
	index := len(b.extract.ImageResources)
	b.extract.ImageResources = append(b.extract.ImageResources, output)
	return index
}

func (c *contentInterpreter) emitImage(image DetailedImage) {
	if c.b.extract == nil {
		c.item(ElementImage, len(c.b.pdf.Images))
		c.b.pdf.Images = append(c.b.pdf.Images, image)
		return
	}
	output := ExtractedImage{Resource: image.Resource, Style: c.b.extractStyle(image.State)}
	if c.b.wantPositions() {
		// A pointer into image would also retain its graphics state and clips.
		matrix := image.Matrix
		output.Matrix = &matrix
	}
	if c.b.wantProvenance() {
		source := image.Source
		output.Source = &source
	}
	page := &c.b.extract.Pages[c.page]
	page.Images = append(page.Images, output)
}

func (b *semanticBuilder) extractStyle(state GraphicsState) *PaintStyle {
	if !b.wantStyles() {
		return nil
	}
	style := basicStyle(state)
	return &style
}

func (b *semanticBuilder) emitAnnotation(annotation Annotation) {
	if b.extract == nil {
		index := len(b.pdf.Annotations)
		b.pdf.Annotations = append(b.pdf.Annotations, annotation)
		b.pdf.Pages[annotation.Page].Annotations = append(b.pdf.Pages[annotation.Page].Annotations, index)
		return
	}
	output := ExtractedAnnotation{Subtype: annotation.Subtype, Rect: annotation.Rect}
	if b.wantProvenance() {
		span := annotation.Object.Span
		output.Source = &span
	}
	page := &b.extract.Pages[annotation.Page]
	page.Annotations = append(page.Annotations, output)
}
