package parser

import "fmt"

// Validate resource dictionaries only after reserving work for the full scan.
// Repeated lookups must not turn one cached large dictionary into unbounded work.
func (b *semanticBuilder) resourceDictionary(object Object) (Dictionary, error) {
	var count int
	switch value := object.Value.(type) {
	case Dictionary:
		count = len(value.Entries)
	case Stream:
		count = len(value.Dictionary.Entries)
	default:
		return Dictionary{}, fmt.Errorf("expected resource dictionary at %+v", object.Span)
	}
	if err := b.chargeSemanticWork(count, "resource dictionary entries", object.Span); err != nil {
		return Dictionary{}, err
	}
	return semDictionary(object)
}

func (b *semanticBuilder) resolvedNumber(object Object) (float64, error) {
	value, err := b.doc.ResolveObject(object)
	if err != nil {
		return 0, err
	}
	return Number(value)
}

func (b *semanticBuilder) resourceNumbers(object Object) ([]float64, error) {
	value, err := b.doc.ResolveObject(object)
	if err != nil {
		return nil, err
	}
	array, ok := value.Value.(Array)
	if !ok {
		return nil, fmt.Errorf("expected resource number array at %+v", value.Span)
	}
	if err := b.chargeValues(len(array.Items), "resource number array", value.Span); err != nil {
		return nil, err
	}
	return b.numbers(value, -1)
}

func (c *contentInterpreter) resource(kind Name, name Name) (Object, error) {
	if err := c.b.chargeSemanticWork(len(c.resources.Entries), "resource dictionary entries", Span{}); err != nil {
		return Object{}, err
	}
	group, ok, err := c.b.get(c.resources, kind)
	if err != nil {
		return Object{}, err
	}
	if !ok {
		return Object{}, fmt.Errorf("missing /%s resources", kind)
	}
	dict, err := c.b.resourceDictionary(group)
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
	// Even a cached image needs Subtype and OC lookups on each invocation.
	if err := c.b.chargeSemanticWork(len(stream.Dictionary.Entries), "XObject dictionary entries", object.Span); err != nil {
		return err
	}
	subtype, err := c.b.name(stream.Dictionary, "Subtype")
	if err != nil {
		return err
	}
	if subtype == "Image" && !c.b.wants(ContentImages) {
		return nil
	}
	_, optional, err := c.b.get(stream.Dictionary, "OC")
	if err != nil {
		return err
	}
	if optional {
		if err := c.b.diag("unsupported-content-effect", "XObject optional-content visibility is retained without evaluation", op.Span); err != nil {
			return err
		}
	}
	switch subtype {
	case "Image":
		return c.image(
			ImageResource{ID: semID(input), Object: object, Stream: stream},
			op,
			index,
			optional,
		)

	case "Form":
		if c.formDepth >= 64 || c.formDepth >= c.b.maxDepth {
			return fmt.Errorf("%w: Form depth limit", ErrLimit)
		}
		if c.b.activeForms[object.Span] {
			return fmt.Errorf("form cycle at %+v", object.Span)
		}
		c.b.activeForms[object.Span] = true
		defer delete(c.b.activeForms, object.Span)
		child := &contentInterpreter{b: c.b, page: c.page, resources: c.resources, state: c.state, textMatrix: c.textMatrix, lineMatrix: c.lineMatrix, positionComplete: c.positionComplete}
		if optional {
			child.state.graphics.Complete = false
		}
		// Form depth must be tracked even when provenance is disabled.
		child.formDepth = c.formDepth + 1
		if c.b.wantProvenance() {
			child.formPath = append(append([]FormCall(nil), c.formPath...), FormCall{ObjectID: semID(input), Span: object.Span, Call: op.Span})
		}
		if c.b.wantTransforms() {
			if matrix, ok, e := c.b.get(stream.Dictionary, "Matrix"); e != nil {
				return e
			} else if ok {
				v, e := c.b.numbers(matrix, 6)
				if e != nil {
					return e
				}
				child.state.graphics.CTM = c.state.graphics.CTM.Mul(Matrix(v))
				if !finiteMatrix(child.state.graphics.CTM) {
					return fmt.Errorf("form transformation overflow")
				}
			}
		}
		if c.b.wantStyles() {
			bbox, ok, e := c.b.get(stream.Dictionary, "BBox")
			if e != nil {
				return e
			}
			if !ok {
				return fmt.Errorf("form missing BBox")
			}
			r, e := c.b.rect(bbox)
			if e != nil {
				return e
			}
			if e = child.addRectClip(r, bbox.Span); e != nil {
				return e
			}
		}
		if resources, ok, e := c.b.get(stream.Dictionary, "Resources"); e != nil {
			return e
		} else if ok {
			child.resources, e = c.b.resourceDictionary(resources)
			if e != nil {
				return e
			}
		}
		if c.b.wantStyles() {
			if _, ok, e := c.b.get(stream.Dictionary, "Group"); e != nil {
				return e
			} else if ok {
				if err := child.unsupported(op, "Form transparency groups are preserved but not composited"); err != nil {
					return err
				}
			}
		}
		if err = child.stream(stream); err != nil {
			return err
		}
		if len(child.operands) > 0 {
			return fmt.Errorf("trailing Form operands")
		}
		if child.inText || len(child.stack) > 0 {
			return c.b.diag("unbalanced-content-state", "Unclosed text or graphics state in Form", object.Span)
		}
	default:
		return c.unsupported(op, fmt.Sprintf("XObject subtype /%s is unsupported", subtype))
	}
	return nil
}

func (c *contentInterpreter) extGState(name Name, op Operation) error {
	if !c.b.wants(ContentText) && !c.b.wantStyles() {
		return nil
	}
	object, err := c.resource("ExtGState", name)
	if err != nil {
		return err
	}
	object, err = c.b.doc.ResolveObject(object)
	if err != nil {
		return err
	}
	dict, err := c.b.resourceDictionary(object)
	if err != nil {
		return err
	}
	for _, entry := range dict.Entries {
		if entry.Key == "Font" && !c.b.wants(ContentText) {
			continue
		}
		if entry.Key != "Font" && !c.b.wantStyles() {
			continue
		}
		value, err := c.b.doc.ResolveObject(entry.Value)
		if err != nil {
			return err
		}
		// PDF Reference 1.6, 3.2.6: null-valued dictionary entries are absent.
		if _, null := value.Value.(Null); null {
			continue
		}
		switch entry.Key {
		case "Type":
		case "LW", "LC", "LJ", "ML":
			mapped := map[Name]string{"LW": "w", "LC": "J", "LJ": "j", "ML": "M"}[entry.Key]
			number, err := Number(value)
			if err != nil {
				return err
			}
			if err := c.setLineParameter(mapped, number); err != nil {
				return err
			}
		case "CA", "ca":
			n, e := Number(value)
			if e != nil || n < 0 || n > 1 {
				return fmt.Errorf("invalid alpha value")
			}
			if entry.Key == "CA" {
				c.state.graphics.StrokeAlpha = n
			} else {
				c.state.graphics.FillAlpha = n
			}
		case "BM":
			mode, ok := value.Value.(Name)
			if !ok {
				if err := c.unsupported(op, "Blend mode arrays are retained without choosing a renderer-supported mode"); err != nil {
					return err
				}
			} else {
				c.state.graphics.BlendMode = mode
			}
		case "D":
			array, ok := value.Value.(Array)
			if !ok || len(array.Items) != 2 {
				return fmt.Errorf("invalid ExtGState D")
			}
			dash, err := c.b.resourceNumbers(array.Items[0])
			if err != nil {
				return err
			}
			phase, err := c.b.resolvedNumber(array.Items[1])
			if err != nil {
				return err
			}
			if err := c.setDash(dash, phase); err != nil {
				return err
			}
		case "RI":
			intent, ok := value.Value.(Name)
			if !ok {
				return fmt.Errorf("ExtGState RI must be a name at %+v", value.Span)
			}
			c.state.graphics.RenderingIntent = intent
		case "Font":
			array, ok := value.Value.(Array)
			if !ok || len(array.Items) != 2 {
				return fmt.Errorf("invalid ExtGState Font")
			}
			font, e := c.b.font(array.Items[0])
			if e != nil {
				return e
			}
			size, e := c.b.resolvedNumber(array.Items[1])
			if e != nil {
				return e
			}
			c.state.text.font = font
			c.state.text.size = size
		case "SMask":
			if mode, ok := value.Value.(Name); !ok || mode != "None" {
				if err := c.unsupported(op, "Soft masks are retained without evaluating their transparency"); err != nil {
					return err
				}
			}
		default:
			if err := c.unsupported(op, fmt.Sprintf("ExtGState /%s is retained but its rendering effect is unsupported", entry.Key)); err != nil {
				return err
			}
		}
	}
	return nil
}
