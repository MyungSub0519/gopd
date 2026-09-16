package content

import (
	"errors"
	"fmt"

	"github.com/MyungSub0519/gopd/internal/model"
	"github.com/MyungSub0519/gopd/internal/structure"
	"github.com/MyungSub0519/gopd/internal/syntax"
)

// semanticBuilder turns a Document's object graph into a DetailedPDF.
//
// It owns everything that spans the whole build: the running result, the
// caches that let a shared resource be described once, the cycle guards, and
// the counters that bound the work. One builder handles the whole document;
// each page gets its own contentInterpreter.
//
// The caches are keyed by model.Span rather than by object identity, because a
// span identifies the actual bytes and so recognises the same resource even
// when it is reached by different routes.
type semanticBuilder struct {
	doc    *structure.Document
	pdf    *DetailedPDF
	fonts  map[model.Span]int
	images map[model.Span]int
	// Cycle guards for the two recursive walks: the page tree, and form
	// XObjects invoking one another. An entry is present only while that
	// node is on the stack.
	activeForms     map[model.Span]bool
	activePages     map[model.Span]bool
	page            int
	operations      int
	pageNodes       int
	glyphCodes      int
	cmaps           map[model.Span]*CMap
	cmapEntries     int
	unicodeBytes    int64
	maxUnicodeBytes int64
	// Running totals and their caps, document-wide rather than per object,
	// so that many small structures cannot together exhaust what a single
	// large one would be denied.
	clipReferences int
	widthEntries   int
	maxDepth       int
	maxObjects     int
}

// get reads a dictionary entry and resolves it, reporting absence as ok=false
// rather than as an error, since most PDF entries are optional.
func (b *semanticBuilder) get(dict model.Dictionary, key model.Name) (model.Object, bool, error) {
	object, err := dict.Get(key)
	if errors.Is(err, model.ErrMissingKey) {
		return model.Object{}, false, nil
	}
	if err != nil {
		return model.Object{}, false, err
	}
	object, err = b.doc.ResolveObject(object)
	return object, true, err
}

// name reads a dictionary entry that must be a name. An absent entry yields
// the empty name with no error, which callers test for where the key is
// optional.
func (b *semanticBuilder) name(dict model.Dictionary, key model.Name) (model.Name, error) {
	object, ok, err := b.get(dict, key)
	if err != nil || !ok {
		return "", err
	}
	name, ok := object.Value.(model.Name)
	if !ok {
		return "", fmt.Errorf("/%s must be a name at %+v", key, object.Span)
	}
	return name, nil
}

// semDictionary takes the dictionary out of an object, accepting a stream by
// returning its dictionary, and rejects duplicate keys.
//
// Duplicates are refused here rather than at parse time because syntax alone
// cannot say whether a repeated key is meaningful; by the time a dictionary is
// being read for meaning, it cannot be, and choosing one occurrence would be
// guesswork.
func semDictionary(object model.Object) (model.Dictionary, error) {
	var dict model.Dictionary
	switch value := object.Value.(type) {
	case model.Dictionary:
		dict = value
	case model.Stream:
		dict = value.Dictionary
	default:
		return model.Dictionary{}, fmt.Errorf("expected dictionary at %+v", object.Span)
	}
	seen := make(map[model.Name]bool, len(dict.Entries))
	for _, entry := range dict.Entries {
		if seen[entry.Key] {
			return model.Dictionary{}, fmt.Errorf("duplicate dictionary key /%s at %+v", entry.Key, entry.KeySpan)
		}
		seen[entry.Key] = true
	}
	return dict, nil
}

// semID returns the identity an object was referenced by, or the zero ObjectID
// when it was written inline and so has no identity of its own.
func semID(object model.Object) model.ObjectID {
	if ref, ok := object.Value.(model.Reference); ok {
		return ref.ID
	}
	return model.ObjectID{}
}

// diag records a finding and marks the current page incomplete.
//
// Raising a diagnostic is how the builder reports something it understood but
// could not fully honour; it never aborts the build. Page.Complete going false
// is the page-level consequence, which is why it happens here rather than
// being the caller's responsibility to remember.
func (b *semanticBuilder) diag(code, message string, span model.Span) {
	b.pdf.Diagnostics = append(b.pdf.Diagnostics, model.Diagnostic{Severity: model.SeverityWarning, Code: code, Message: message, Span: span})
	b.pdf.diagnosticPages = append(b.pdf.diagnosticPages, b.page)
	if b.page >= 0 {
		b.pdf.Pages[b.page].Complete = false
	}
}

// rect reads a four-number array as a rectangle, requiring the lower-left
// corner first. See model.Rect on why an inverted rectangle is rejected rather
// than normalised.
func (b *semanticBuilder) rect(object model.Object) (model.Rect, error) {
	values, err := b.numbers(object, 4)
	if err != nil {
		return model.Rect{}, err
	}
	if values[2] < values[0] || values[3] < values[1] {
		return model.Rect{}, fmt.Errorf("inverted rectangle at %+v", object.Span)
	}
	return model.Rect{Min: model.Point{X: values[0], Y: values[1]}, Max: model.Point{X: values[2], Y: values[3]}}, nil
}

// numbers reads an array of numbers, resolving each element. A negative n
// accepts any length.
func (b *semanticBuilder) numbers(object model.Object, n int) ([]float64, error) {
	array, ok := object.Value.(model.Array)
	if !ok || (n >= 0 && len(array.Items) != n) {
		return nil, fmt.Errorf("expected array of %d numbers at %+v", n, object.Span)
	}
	out := make([]float64, len(array.Items))
	for i, item := range array.Items {
		value, err := b.doc.ResolveObject(item)
		if err != nil {
			return nil, err
		}
		out[i], err = model.Number(value)
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

// walkPages descends the page tree, interpreting each page it reaches.
//
// The tree is made of /Pages nodes with /Kids and /Page leaves. Four
// attributes — MediaBox, CropBox, Resources and Rotate — are inheritable: a
// page that does not state one takes it from its nearest ancestor that does,
// which is what inherited carries down and why a node overrides it with its
// own entries before recursing.
//
// The recursion is guarded three ways, because the tree is a graph in a
// damaged file: by depth, by total nodes visited, and by activePages, which
// catches a node reachable from its own subtree.
func (b *semanticBuilder) walkPages(input model.Object, inherited map[model.Name]model.Object, depth int) error {
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
	// Copy rather than mutate: siblings must not see each other's overrides.
	values := make(map[model.Name]model.Object, len(inherited))
	for key, value := range inherited {
		values[key] = value
	}
	for _, key := range []model.Name{"MediaBox", "CropBox", "Resources", "Rotate"} {
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
		array, isArray := kids.Value.(model.Array)
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
	// CropBox defaults to MediaBox when absent.
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
		value, e := model.Int(rotate)
		if e != nil || value%90 != 0 {
			return fmt.Errorf("invalid page Rotate at %+v", rotate.Span)
		}
		// Rotation must be a multiple of 90 and is normalised into
		// (-360, 360); the sign is kept, since negative rotations are legal.
		page.Rotate = int(value % 360)
	}
	if unit, ok, e := b.get(dict, "UserUnit"); e != nil {
		return e
	} else if ok {
		page.UserUnit, e = model.Number(unit)
		if e != nil || page.UserUnit <= 0 {
			return fmt.Errorf("invalid page UserUnit at %+v", unit.Span)
		}
	}
	if contents, ok, e := b.get(dict, "Contents"); e != nil {
		return e
	} else if ok {
		// /Contents may be one stream or an array of them. An array is a
		// single logical content stream split across objects, so an
		// operator may legally straddle the boundary between two members.
		switch value := contents.Value.(type) {
		case model.Array:
			page.Contents = value.Items
		case model.Stream:
			page.Contents = []model.Object{contents}
		case model.Null:
		default:
			return fmt.Errorf("invalid Page Contents at %+v", contents.Span)
		}
	}
	b.pdf.Pages = append(b.pdf.Pages, page)
	b.page = page.Index
	interpreter := &contentInterpreter{b: b, page: page.Index, resources: page.Resources, state: initialContentState(), textMatrix: model.IdentityMatrix(), lineMatrix: model.IdentityMatrix(), positionComplete: true}
	for _, content := range page.Contents {
		object, e := b.doc.ResolveObject(content)
		if e != nil {
			return e
		}
		stream, ok := object.Value.(model.Stream)
		if !ok {
			return fmt.Errorf("page Contents member is not a stream at %+v", object.Span)
		}
		if err = interpreter.stream(stream); err != nil {
			return err
		}
	}
	// Operands left over mean the stream ended mid-instruction, with values
	// pushed that no operator consumed.
	if len(interpreter.operands) > 0 {
		return fmt.Errorf("trailing content operands at %+v", interpreter.operands[0].Span)
	}
	if interpreter.inText || len(interpreter.stack) > 0 {
		b.diag("unbalanced-content-state", "Unclosed text object or saved graphics state", object.Span)
	}
	if annots, ok, e := b.get(dict, "Annots"); e != nil {
		return e
	} else if ok {
		array, ok := annots.Value.(model.Array)
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

// contentInterpreter executes one content stream against a graphics state.
//
// One interpreter handles one page, and a nested one handles each form XObject
// the page invokes, inheriting the caller's state. The interpreter holds the
// state machine; the builder it points at holds everything document-wide.
//
// Content streams are postfix: operands accumulate until an operator consumes
// them. That is what operands is, and a non-empty operands at the end of a
// stream means the content was truncated mid-instruction.
type contentInterpreter struct {
	b         *semanticBuilder
	page      int
	resources model.Dictionary
	state     contentState
	stack     []contentState
	// textMatrix is where the next glyph goes; lineMatrix is the start of
	// the current line, which T* and Td return to. Both are only meaningful
	// between BT and ET, which inText tracks.
	textMatrix, lineMatrix model.Matrix
	inText                 bool
	positionComplete       bool
	path                   []DetailedPathSegment
	pathOperations         []int
	// A W operator does not clip immediately: it marks the current path as
	// the next clip, which takes effect only once the painting operator
	// that follows has run.
	pendingClip bool
	clipEvenOdd bool
	operands    []model.Object
	formPath    []FormCall
}

// stream executes one content stream.
//
// It scans one token at a time, accumulating operands until a keyword token
// arrives and executes as an operator.
//
// Scanning incrementally rather than tokenising the whole stream first is what
// makes inline images possible: the bytes between an ID and its EI are raw
// image data, not PDF syntax, and where they end is only knowable from the
// dictionary that precedes them. BI is therefore intercepted here, where the
// scanner can be repositioned past the payload, rather than in execute.
func (c *contentInterpreter) stream(stream model.Stream) error {
	source, err := c.b.doc.DecodeStream(stream)
	if err != nil {
		return err
	}
	data, err := c.b.doc.Bytes(model.Span{Source: source.ID, Start: 0, End: source.Size})
	if err != nil {
		return err
	}
	scanner, err := syntax.NewScanner(data, source.ID, 0, c.b.doc.Options.Limits)
	if err != nil {
		return fmt.Errorf("content at source %d: %w", source.ID, err)
	}
	for {
		token, err := scanner.NextNonTrivia()
		if err != nil {
			return fmt.Errorf("content at source %d: %w", source.ID, err)
		}
		if token.Kind == model.TokenEOF {
			return nil
		}
		// A keyword token is an operator, except for the three that are
		// object values and belong on the operand stack instead.
		word := string(data[token.Span.Start:token.Span.End])
		if token.Kind == model.TokenKeyword && word != "true" && word != "false" && word != "null" {
			c.b.operations++
			if c.b.operations > c.b.maxObjects {
				return fmt.Errorf("content operation limit at %+v", token.Span)
			}
			if word == "BI" {
				if err = c.inlineImage(scanner, token); err != nil {
					return fmt.Errorf("inline image at source %d offset %d: %w", token.Span.Source, token.Span.Start, err)
				}
				continue
			}
			op := Operation{Operator: word, Operands: c.operands, Span: token.Span, FormPath: append([]FormCall(nil), c.formPath...)}
			c.operands = nil
			index := len(c.b.pdf.Pages[c.page].Operations)
			c.b.pdf.Pages[c.page].Operations = append(c.b.pdf.Pages[c.page].Operations, op)
			if err = c.execute(op, index); err != nil {
				return fmt.Errorf("operator %s at source %d offset %d: %w", word, token.Span.Source, token.Span.Start, err)
			}
			continue
		}
		// An operand may be a whole array or dictionary, which the object
		// parser understands and the scanner alone does not, so it is
		// reparsed from the token's start and the scanner follows.
		object, err := scanner.ParseObjectAt(int(token.Span.Start))
		if err != nil {
			return err
		}
		c.operands = append(c.operands, object)
		if len(c.operands) > 65536 {
			return fmt.Errorf("content operand limit at %+v", token.Span)
		}
	}
}

// source records where an element came from: the spans of the operator and
// its operands, the operation's index within the page, and the form call stack
// in effect.
func (c *contentInterpreter) source(op Operation, index int) ElementSource {
	spans := make([]model.Span, 0, len(op.Operands)+1)
	for _, operand := range op.Operands {
		spans = append(spans, operand.Span)
	}
	spans = append(spans, op.Span)
	return ElementSource{Page: c.page, Spans: spans, Operations: []int{index}, FormPath: append([]FormCall(nil), c.formPath...)}
}

// item links a newly added element back to the page that drew it, which is
// what keeps the document-wide slices navigable per page.
func (c *contentInterpreter) item(kind ElementKind, index int) {
	page := &c.b.pdf.Pages[c.page]
	page.Items = append(page.Items, ElementRef{kind, index})
}

// unsupported records that an operator's effect could not be reproduced.
//
// The operation itself is still retained, and interpretation continues; only
// the completeness flags change. This is the normal way an unimplemented
// effect is reported, as opposed to returning an error, which abandons the
// page.
func (c *contentInterpreter) unsupported(op Operation, message string) {
	c.b.diag("unsupported-content-effect", message, op.Span)
	c.state.graphics.Complete = false
}

// resource looks a named resource up in the current resource dictionary, for
// example the font /F1 under /Font.
//
// Resources are scoped: a form XObject with its own /Resources replaces the
// dictionary its caller was using, which is why this reads from the
// interpreter rather than from the page.
func (c *contentInterpreter) resource(kind model.Name, name model.Name) (model.Object, error) {
	group, ok, err := c.b.get(c.resources, kind)
	if err != nil {
		return model.Object{}, err
	}
	if !ok {
		return model.Object{}, fmt.Errorf("missing /%s resources", kind)
	}
	dict, err := semDictionary(group)
	if err != nil {
		return model.Object{}, err
	}
	resource, err := dict.Get(name)
	if err != nil {
		return model.Object{}, fmt.Errorf("resource /%s: %w", name, err)
	}
	return resource, nil
}

// xobject executes a Do operator.
//
// The two subtypes behave quite differently. An image is placed: its data is
// described once as an ImageResource and each Do adds a DetailedImage giving
// the matrix it was drawn with. A form is executed: its content stream is
// interpreted here and now, with the caller's graphics state inherited, so its
// contents become ordinary elements of the current page.
func (c *contentInterpreter) xobject(name model.Name, op Operation, index int) error {
	input, err := c.resource("XObject", name)
	if err != nil {
		return err
	}
	object, err := c.b.doc.ResolveObject(input)
	if err != nil {
		return err
	}
	stream, ok := object.Value.(model.Stream)
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
				name   model.Name
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
					n, e := model.Int(value)
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
				v, ok := mask.Value.(model.Boolean)
				if !ok {
					return fmt.Errorf("invalid ImageMask")
				}
				image.ImageMask = bool(v)
			}
			resource = len(c.b.pdf.ImageResources)
			c.b.pdf.ImageResources = append(c.b.pdf.ImageResources, image)
			c.b.images[object.Span] = resource
		}
		// An image is drawn into the unit square, so the CTM alone gives
		// its position, size and orientation on the page.
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
		// The child inherits the current state by value, so whatever the
		// form does to it is discarded when the form returns, exactly as
		// if the form were wrapped in q and Q.
		child := &contentInterpreter{b: c.b, page: c.page, resources: c.resources, state: c.state, textMatrix: c.textMatrix, lineMatrix: c.lineMatrix, positionComplete: c.positionComplete}
		child.formPath = append(append([]FormCall(nil), c.formPath...), FormCall{ObjectID: semID(input), Span: object.Span, Call: op.Span})
		if matrix, ok, e := c.b.get(stream.Dictionary, "Matrix"); e != nil {
			return e
		} else if ok {
			v, e := c.b.numbers(matrix, 6)
			if e != nil {
				return e
			}
			child.state.graphics.CTM = c.state.graphics.CTM.Mul(model.Matrix(v))
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
		// /BBox clips the form's contents, so it is applied as a clip path
		// rather than merely recorded.
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
