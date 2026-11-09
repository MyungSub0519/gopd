package parser

import (
	"fmt"

	"github.com/MyungSub0519/gopd/internal/common/syntax"
)

// DetailedImage is one placement of an image XObject or inline image; bytes
// belong to ImageResource.
type DetailedImage struct {
	Source   ElementSource
	Resource int
	Matrix   Matrix
	State    GraphicsState
}

// ImageResource is an image's metadata and the location of its encoded bytes.
// An inline image has a zero ID; its Object spans the BI dictionary and holds
// the Stream, whose StartKeyword is ID and EndKeyword is EI.
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
// present only with Provenance; its Stream can be used with Result.Document.
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
	return c.placeImage(resource, op, index, optional)
}

// inlineImage reads the dictionary, payload and EI after a BI operator and
// stores the located image as the operator's single operand, so that execute
// can place it like an image XObject.
func (c *contentInterpreter) inlineImage(scanner *syntax.ContentScanner, op *Operation) error {
	if len(op.Operands) != 0 {
		return fmt.Errorf("BI takes no operands")
	}
	stream, values, err := scanner.InlineImage(c.b.maxValues - c.b.semanticValues)
	c.b.semanticValues += values
	if err != nil {
		return err
	}
	op.Operands = []Object{{Span: stream.DictionarySpan, Value: stream}}
	return nil
}

// placeInlineImage registers an inline image read by inlineImage. Its payload
// is located but, as for image XObjects, never decoded.
func (c *contentInterpreter) placeInlineImage(op Operation, index int) error {
	if !c.b.wants(ContentImages) {
		return nil
	}
	object := op.Operands[0]
	resource, exists := c.b.images[object.Span]
	if !exists {
		stream := object.Value.(Stream)
		image := ImageResource{Object: object, Stream: stream}
		for _, entry := range []struct {
			short, full Name
			target      *int
		}{{"W", "Width", &image.Width}, {"H", "Height", &image.Height}, {"BPC", "BitsPerComponent", &image.BitsPerComponent}} {
			value, ok := syntax.InlineImageEntry(stream.Dictionary, entry.short, entry.full)
			if !ok {
				continue
			}
			n, err := Int(value)
			if err != nil || n < 0 || n > 1<<30 {
				return fmt.Errorf("invalid inline image /%s", entry.short)
			}
			*entry.target = int(n)
		}
		image.ColorSpace, _ = syntax.InlineImageEntry(stream.Dictionary, "CS", "ColorSpace")
		if mask, ok := syntax.InlineImageEntry(stream.Dictionary, "IM", "ImageMask"); ok {
			v, ok := mask.Value.(Boolean)
			if !ok {
				return fmt.Errorf("invalid inline image /IM")
			}
			image.ImageMask = bool(v)
		}
		if err := c.b.chargeSemantic("inline image", object.Span); err != nil {
			return err
		}
		resource = c.b.emitImageResource(image)
		c.b.images[object.Span] = resource
	}
	return c.placeImage(resource, op, index, false)
}

// placeImage emits one placement of a registered image resource. Images are
// drawn into the unit square, so the CTM alone gives the placement.
func (c *contentInterpreter) placeImage(resource int, op Operation, index int, optional bool) error {
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
	if b.result == nil {
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
	index := len(b.result.ImageResources)
	b.result.ImageResources = append(b.result.ImageResources, output)
	return index
}

func (c *contentInterpreter) emitImage(image DetailedImage) {
	if c.b.result == nil {
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
	page := &c.b.result.Pages[c.page]
	page.Images = append(page.Images, output)
}
