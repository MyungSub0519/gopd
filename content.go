package gopd

import (
	"errors"
	"fmt"
)

type semanticBuilder struct {
	doc             *Document
	pdf             *DetailedPDF
	fonts           map[Span]int
	images          map[Span]int
	activeForms     map[Span]bool
	activePages     map[Span]bool
	page            int
	operations      int
	pageNodes       int
	glyphCodes      int
	cmaps           map[Span]*CMap
	cmapEntries     int
	unicodeBytes    int64
	maxUnicodeBytes int64
	clipReferences  int
	widthEntries    int
	maxDepth        int
	maxObjects      int
}

// BuildPDF interprets page contents. Invalid syntax and traversal limits return
// an error and any partial result. Unsupported effects are retained as operations
// and diagnostics. No pixel rendering, OCR, or reading-order reconstruction occurs.
func BuildPDF(d *Document) (*DetailedPDF, error) {
	if d == nil {
		return nil, fmt.Errorf("nil Document")
	}
	p := &DetailedPDF{Document: d, Structure: &d.Structure}
	if d.Encrypted {
		return p, fmt.Errorf("semantic decoding of encrypted PDF is unsupported")
	}
	b := &semanticBuilder{doc: d, pdf: p, fonts: make(map[Span]int), images: make(map[Span]int), activeForms: make(map[Span]bool), activePages: make(map[Span]bool), page: -1, maxDepth: 128, maxObjects: 1000000}
	b.cmaps = make(map[Span]*CMap)
	b.maxUnicodeBytes = 256 << 20
	if d.Options.Limits.MaxDecodedBytes > 0 && d.Options.Limits.MaxDecodedBytes < b.maxUnicodeBytes {
		b.maxUnicodeBytes = d.Options.Limits.MaxDecodedBytes
	}
	if d.Options.Limits.MaxDepth > 0 && d.Options.Limits.MaxDepth < b.maxDepth {
		b.maxDepth = d.Options.Limits.MaxDepth
	}
	if d.Options.Limits.MaxObjects > 0 && d.Options.Limits.MaxObjects < b.maxObjects {
		b.maxObjects = d.Options.Limits.MaxObjects
	}
	catalog, err := d.Catalog()
	if err != nil {
		return p, err
	}
	dict, err := semDictionary(catalog)
	if err != nil {
		return p, err
	}
	pages, err := dict.Get("Pages")
	if err != nil {
		return p, err
	}
	err = b.walkPages(pages, map[Name]Object{}, 0)
	return p, err
}

func (b *semanticBuilder) get(dict Dictionary, key Name) (Object, bool, error) {
	object, err := dict.Get(key)
	if errors.Is(err, ErrMissingKey) {
		return Object{}, false, nil
	}
	if err != nil {
		return Object{}, false, err
	}
	object, err = b.doc.ResolveObject(object)
	return object, true, err
}
func (b *semanticBuilder) name(dict Dictionary, key Name) (Name, error) {
	object, ok, err := b.get(dict, key)
	if err != nil || !ok {
		return "", err
	}
	name, ok := object.Value.(Name)
	if !ok {
		return "", fmt.Errorf("/%s must be a name at %+v", key, object.Span)
	}
	return name, nil
}
func semDictionary(object Object) (Dictionary, error) {
	var dict Dictionary
	switch value := object.Value.(type) {
	case Dictionary:
		dict = value
	case Stream:
		dict = value.Dictionary
	default:
		return Dictionary{}, fmt.Errorf("expected dictionary at %+v", object.Span)
	}
	seen := make(map[Name]bool, len(dict.Entries))
	for _, entry := range dict.Entries {
		if seen[entry.Key] {
			return Dictionary{}, fmt.Errorf("duplicate dictionary key /%s at %+v", entry.Key, entry.KeySpan)
		}
		seen[entry.Key] = true
	}
	return dict, nil
}
func semID(object Object) ObjectID {
	if ref, ok := object.Value.(Reference); ok {
		return ref.ID
	}
	return ObjectID{}
}
func (b *semanticBuilder) diag(code, message string, span Span) {
	b.pdf.Diagnostics = append(b.pdf.Diagnostics, Diagnostic{Severity: SeverityWarning, Code: code, Message: message, Span: span})
	b.pdf.diagnosticPages = append(b.pdf.diagnosticPages, b.page)
	if b.page >= 0 {
		b.pdf.Pages[b.page].Complete = false
	}
}
func (b *semanticBuilder) rect(object Object) (Rect, error) {
	values, err := b.numbers(object, 4)
	if err != nil {
		return Rect{}, err
	}
	if values[2] < values[0] || values[3] < values[1] {
		return Rect{}, fmt.Errorf("inverted rectangle at %+v", object.Span)
	}
	return Rect{Point{values[0], values[1]}, Point{values[2], values[3]}}, nil
}
func (b *semanticBuilder) numbers(object Object, n int) ([]float64, error) {
	array, ok := object.Value.(Array)
	if !ok || (n >= 0 && len(array.Items) != n) {
		return nil, fmt.Errorf("expected array of %d numbers at %+v", n, object.Span)
	}
	out := make([]float64, len(array.Items))
	for i, item := range array.Items {
		value, err := b.doc.ResolveObject(item)
		if err != nil {
			return nil, err
		}
		out[i], err = Number(value)
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (b *semanticBuilder) walkPages(input Object, inherited map[Name]Object, depth int) error {
	b.pageNodes++
	if b.pageNodes > b.maxObjects {
		return fmt.Errorf("page tree visit limit at %+v", input.Span)
	}
	if depth >= b.maxDepth {
		return fmt.Errorf("page tree depth limit at %+v", input.Span)
	}
	object, err := b.doc.ResolveObject(input)
	if err != nil {
		return err
	}
	if b.activePages[object.Span] {
		return fmt.Errorf("page tree cycle at %+v", object.Span)
	}
	b.activePages[object.Span] = true
	defer delete(b.activePages, object.Span)
	dict, err := semDictionary(object)
	if err != nil {
		return err
	}
	values := make(map[Name]Object, len(inherited))
	for key, value := range inherited {
		values[key] = value
	}
	for _, key := range []Name{"MediaBox", "CropBox", "Resources", "Rotate"} {
		value, ok, err := b.get(dict, key)
		if err != nil {
			return err
		}
		if ok {
			values[key] = value
		}
	}
	kind, err := b.name(dict, "Type")
	if err != nil {
		return err
	}
	if kind == "Pages" {
		kids, ok, err := b.get(dict, "Kids")
		if err != nil {
			return err
		}
		array, isArray := kids.Value.(Array)
		if !ok || !isArray {
			return fmt.Errorf("Pages missing Kids array at %+v", object.Span)
		}
		if len(array.Items) > b.maxObjects {
			return fmt.Errorf("page tree object limit at %+v", kids.Span)
		}
		for _, kid := range array.Items {
			if err = b.walkPages(kid, values, depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	if kind != "Page" {
		return fmt.Errorf("page tree node has Type /%s at %+v", kind, object.Span)
	}
	if len(b.pdf.Pages) >= b.maxObjects {
		return fmt.Errorf("page count limit at %+v", object.Span)
	}
	page := DetailedPage{Index: len(b.pdf.Pages), Object: object, UserUnit: 1, Complete: true}
	media, ok := values["MediaBox"]
	if !ok {
		return fmt.Errorf("Page missing MediaBox at %+v", object.Span)
	}
	page.MediaBox, err = b.rect(media)
	if err != nil {
		return err
	}
	page.CropBox = page.MediaBox
	if crop, ok := values["CropBox"]; ok {
		page.CropBox, err = b.rect(crop)
		if err != nil {
			return err
		}
	}
	if resources, ok := values["Resources"]; ok {
		page.Resources, err = semDictionary(resources)
		if err != nil {
			return err
		}
	}
	if rotate, ok := values["Rotate"]; ok {
		value, e := Int(rotate)
		if e != nil || value%90 != 0 {
			return fmt.Errorf("invalid page Rotate at %+v", rotate.Span)
		}
		page.Rotate = int(value % 360)
	}
	if unit, ok, e := b.get(dict, "UserUnit"); e != nil {
		return e
	} else if ok {
		page.UserUnit, e = Number(unit)
		if e != nil || page.UserUnit <= 0 {
			return fmt.Errorf("invalid page UserUnit at %+v", unit.Span)
		}
	}
	if contents, ok, e := b.get(dict, "Contents"); e != nil {
		return e
	} else if ok {
		switch value := contents.Value.(type) {
		case Array:
			page.Contents = value.Items
		case Stream:
			page.Contents = []Object{contents}
		case Null:
		default:
			return fmt.Errorf("invalid Page Contents at %+v", contents.Span)
		}
	}
	b.pdf.Pages = append(b.pdf.Pages, page)
	b.page = page.Index
	interpreter := &contentInterpreter{b: b, page: page.Index, resources: page.Resources, state: initialContentState(), textMatrix: IdentityMatrix(), lineMatrix: IdentityMatrix(), positionComplete: true}
	for _, content := range page.Contents {
		object, e := b.doc.ResolveObject(content)
		if e != nil {
			return e
		}
		stream, ok := object.Value.(Stream)
		if !ok {
			return fmt.Errorf("page Contents member is not a stream at %+v", object.Span)
		}
		if err = interpreter.stream(stream); err != nil {
			return err
		}
	}
	if len(interpreter.operands) > 0 {
		return fmt.Errorf("trailing content operands at %+v", interpreter.operands[0].Span)
	}
	if interpreter.inText || len(interpreter.stack) > 0 {
		b.diag("unbalanced-content-state", "Unclosed text object or saved graphics state", object.Span)
	}
	if annots, ok, e := b.get(dict, "Annots"); e != nil {
		return e
	} else if ok {
		array, ok := annots.Value.(Array)
		if !ok {
			return fmt.Errorf("invalid Annots at %+v", annots.Span)
		}
		for _, item := range array.Items {
			annotation, e := b.doc.ResolveObject(item)
			if e != nil {
				return e
			}
			ad, e := semDictionary(annotation)
			if e != nil {
				return e
			}
			subtype, e := b.name(ad, "Subtype")
			if e != nil {
				return e
			}
			a := Annotation{Page: page.Index, Object: annotation, Subtype: subtype}
			if rectangle, ok, e := b.get(ad, "Rect"); e != nil {
				return e
			} else if ok {
				a.Rect, e = b.rect(rectangle)
				if e != nil {
					return e
				}
			}
			index := len(b.pdf.Annotations)
			b.pdf.Annotations = append(b.pdf.Annotations, a)
			b.pdf.Pages[page.Index].Annotations = append(b.pdf.Pages[page.Index].Annotations, index)
		}
	}
	b.page = -1
	return nil
}

type contentInterpreter struct {
	b                      *semanticBuilder
	page                   int
	resources              Dictionary
	state                  contentState
	stack                  []contentState
	textMatrix, lineMatrix Matrix
	inText                 bool
	positionComplete       bool
	path                   []DetailedPathSegment
	pathOperations         []int
	pendingClip            bool
	clipEvenOdd            bool
	operands               []Object
	formPath               []FormCall
}

func (c *contentInterpreter) stream(stream Stream) error {
	source, err := c.b.doc.DecodeStream(stream)
	if err != nil {
		return err
	}
	data, err := c.b.doc.Bytes(Span{source.ID, 0, source.Size})
	if err != nil {
		return err
	}
	tokens, err := Lex(data, source.ID, 0)
	if err != nil {
		return fmt.Errorf("content at source %d: %w", source.ID, err)
	}
	for i := 0; i < len(tokens); {
		token := tokens[i]
		if token.Kind == TokenWhitespace || token.Kind == TokenComment || token.Kind == TokenEOF {
			i++
			continue
		}
		word := string(data[token.Span.Start:token.Span.End])
		if token.Kind == TokenKeyword && word != "true" && word != "false" && word != "null" {
			c.b.operations++
			if c.b.operations > c.b.maxObjects {
				return fmt.Errorf("content operation limit at %+v", token.Span)
			}
			op := Operation{Operator: word, Operands: c.operands, Span: token.Span, FormPath: append([]FormCall(nil), c.formPath...)}
			c.operands = nil
			index := len(c.b.pdf.Pages[c.page].Operations)
			c.b.pdf.Pages[c.page].Operations = append(c.b.pdf.Pages[c.page].Operations, op)
			if err = c.execute(op, index); err != nil {
				return fmt.Errorf("operator %s at source %d offset %d: %w", word, token.Span.Source, token.Span.Start, err)
			}
			i++
			continue
		}
		object, consumed, e := parseObjectWithLimits(data[token.Span.Start:], source.ID, token.Span.Start, c.b.doc.Options.Limits)
		if e != nil {
			return e
		}
		if consumed <= 0 {
			return fmt.Errorf("content parser made no progress at %+v", token.Span)
		}
		c.operands = append(c.operands, object)
		if len(c.operands) > 65536 {
			return fmt.Errorf("content operand limit at %+v", token.Span)
		}
		end := token.Span.Start + int64(consumed)
		for i < len(tokens) && tokens[i].Span.Start < end {
			i++
		}
	}
	return nil
}

func (c *contentInterpreter) source(op Operation, index int) ElementSource {
	spans := make([]Span, 0, len(op.Operands)+1)
	for _, operand := range op.Operands {
		spans = append(spans, operand.Span)
	}
	spans = append(spans, op.Span)
	return ElementSource{Page: c.page, Spans: spans, Operations: []int{index}, FormPath: append([]FormCall(nil), c.formPath...)}
}
func (c *contentInterpreter) item(kind ElementKind, index int) {
	page := &c.b.pdf.Pages[c.page]
	page.Items = append(page.Items, ElementRef{kind, index})
}
func (c *contentInterpreter) unsupported(op Operation, message string) {
	c.b.diag("unsupported-content-effect", message, op.Span)
	c.state.graphics.Complete = false
}

func (c *contentInterpreter) resource(kind Name, name Name) (Object, error) {
	group, ok, err := c.b.get(c.resources, kind)
	if err != nil {
		return Object{}, err
	}
	if !ok {
		return Object{}, fmt.Errorf("missing /%s resources", kind)
	}
	dict, err := semDictionary(group)
	if err != nil {
		return Object{}, err
	}
	resource, err := dict.Get(name)
	if err != nil {
		return Object{}, fmt.Errorf("resource /%s: %w", name, err)
	}
	return resource, nil
}

func (c *contentInterpreter) xobject(name Name, op Operation, index int) error {
	input, err := c.resource("XObject", name)
	if err != nil {
		return err
	}
	object, err := c.b.doc.ResolveObject(input)
	if err != nil {
		return err
	}
	stream, ok := object.Value.(Stream)
	if !ok {
		return fmt.Errorf("XObject is not a stream")
	}
	subtype, err := c.b.name(stream.Dictionary, "Subtype")
	if err != nil {
		return err
	}
	switch subtype {
	case "Image":
		resource, exists := c.b.images[object.Span]
		if !exists {
			image := ImageResource{ID: semID(input), Object: object, Stream: stream}
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
			resource = len(c.b.pdf.ImageResources)
			c.b.pdf.ImageResources = append(c.b.pdf.ImageResources, image)
			c.b.images[object.Span] = resource
		}
		placement := DetailedImage{Source: c.source(op, index), Resource: resource, Matrix: c.state.graphics.CTM, State: c.state.graphics}
		c.item(ElementImage, len(c.b.pdf.Images))
		c.b.pdf.Images = append(c.b.pdf.Images, placement)
	case "Form":
		if len(c.formPath) >= 64 || len(c.formPath) >= c.b.maxDepth {
			return fmt.Errorf("Form depth limit")
		}
		if c.b.activeForms[object.Span] {
			return fmt.Errorf("Form cycle at %+v", object.Span)
		}
		c.b.activeForms[object.Span] = true
		defer delete(c.b.activeForms, object.Span)
		child := &contentInterpreter{b: c.b, page: c.page, resources: c.resources, state: c.state, textMatrix: c.textMatrix, lineMatrix: c.lineMatrix, positionComplete: c.positionComplete}
		child.formPath = append(append([]FormCall(nil), c.formPath...), FormCall{ObjectID: semID(input), Span: object.Span, Call: op.Span})
		if matrix, ok, e := c.b.get(stream.Dictionary, "Matrix"); e != nil {
			return e
		} else if ok {
			v, e := c.b.numbers(matrix, 6)
			if e != nil {
				return e
			}
			child.state.graphics.CTM = c.state.graphics.CTM.Mul(Matrix(v))
			if !finiteMatrix(child.state.graphics.CTM) {
				return fmt.Errorf("Form transformation overflow")
			}
		}
		bbox, ok, e := c.b.get(stream.Dictionary, "BBox")
		if e != nil {
			return e
		}
		if !ok {
			return fmt.Errorf("Form missing BBox")
		}
		r, e := c.b.rect(bbox)
		if e != nil {
			return e
		}
		if e = child.addRectClip(r, bbox.Span); e != nil {
			return e
		}
		if resources, ok, e := c.b.get(stream.Dictionary, "Resources"); e != nil {
			return e
		} else if ok {
			child.resources, e = semDictionary(resources)
			if e != nil {
				return e
			}
		}
		if _, ok, e := c.b.get(stream.Dictionary, "Group"); e != nil {
			return e
		} else if ok {
			child.unsupported(op, "Form transparency groups are preserved but not composited")
		}
		if err = child.stream(stream); err != nil {
			return err
		}
		if len(child.operands) > 0 {
			return fmt.Errorf("trailing Form operands")
		}
		if child.inText || len(child.stack) > 0 {
			c.b.diag("unbalanced-content-state", "Unclosed text or graphics state in Form", object.Span)
		}
	default:
		c.unsupported(op, fmt.Sprintf("XObject subtype /%s is unsupported", subtype))
	}
	return nil
}
