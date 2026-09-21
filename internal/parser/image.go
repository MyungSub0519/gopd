package parser

import "fmt"

// DetailedImage is one execution of an image XObject; bytes belong to ImageResource.
type DetailedImage struct {
	Source   ElementSource
	Resource int
	Matrix   Matrix
	State    GraphicsState
}

type ImageResource struct {
	Object           Object
	ID               ObjectID
	Stream           Stream
	Width, Height    int
	BitsPerComponent int
	ColorSpace       Object
	ImageMask        bool
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

func (c *contentInterpreter) image(image ImageResource, op Operation, index int, optional bool) error {
	object := image.Object
	stream := image.Stream
	var err error

	resource, exists := c.b.images[object.Span]
	if !exists {
		for _, entry := range []struct {
			name   Name
			target *int
		}{{"Width", &image.Width}, {"Height", &image.Height}, {"BitsPerComponent", &image.BitsPerComponent}} {
			value, ok, e := c.b.get(stream.Dictionary, entry.name)
			if e != nil {
				return e
			}
			if !ok && entry.name != "BitsPerComponent" {
				return fmt.Errorf("image missing /%s", entry.name)
			}
			if ok {
				n, e := Int(value)
				if e != nil || n < 0 || n > 1<<30 {
					return fmt.Errorf("invalid image /%s", entry.name)
				}
				*entry.target = int(n)
			}
		}
		image.ColorSpace, _, err = c.b.get(stream.Dictionary, "ColorSpace")
		if err != nil {
			return err
		}
		if mask, ok, e := c.b.get(stream.Dictionary, "ImageMask"); e != nil {
			return e
		} else if ok {
			v, ok := mask.Value.(Boolean)
			if !ok {
				return fmt.Errorf("invalid ImageMask")
			}
			image.ImageMask = bool(v)
		}
		resource = c.b.emitImageResource(image)
		c.b.images[object.Span] = resource
	}
	placement := DetailedImage{Source: c.source(op, index), Resource: resource, Matrix: c.state.graphics.CTM, State: c.state.graphics}
	if optional {
		placement.State.Complete = false
	}
	if err := c.b.chargeStyle(placement.State, op.Span); err != nil {
		return err
	}
	c.emitImage(placement)
	return nil
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
