package gopd

import (
	"fmt"

	"github.com/MyungSub0519/gopd/internal/document"
)

func (b *semanticBuilder) walkPages(input Object, inherited map[Name]Object, depth int) error {
	if err := b.chargeSemantic("page tree node", input.Span); err != nil {
		return err
	}
	b.pageNodes++
	if b.pageNodes > b.maxObjects {
		return fmt.Errorf("%w: page tree visit limit at %+v", ErrLimit, input.Span)
	}
	if depth >= b.maxDepth {
		return fmt.Errorf("%w: page tree depth limit at %+v", ErrLimit, input.Span)
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
		if key == "Resources" && !b.wantPageContent() {
			continue
		}
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
			return fmt.Errorf("page tree node missing Kids array at %+v", object.Span)
		}
		if len(array.Items) > b.maxObjects {
			return fmt.Errorf("%w: page tree object limit at %+v", ErrLimit, kids.Span)
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
		return fmt.Errorf("%w: page count limit at %+v", ErrLimit, object.Span)
	}
	page := DetailedPage{Index: len(b.pdf.Pages), Object: object, UserUnit: 1, Complete: true}
	media, ok := values["MediaBox"]
	if !ok {
		return fmt.Errorf("page missing MediaBox at %+v", object.Span)
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
		page.Resources, err = b.resourceDictionary(resources)
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
	if b.wantPageContent() {
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
	}
	b.pdf.Pages = append(b.pdf.Pages, page)
	if b.extract != nil {
		b.extract.Pages = append(b.extract.Pages, ExtractedPage{
			Index: page.Index, MediaBox: page.MediaBox, CropBox: page.CropBox,
			Rotate: page.Rotate, UserUnit: page.UserUnit, Complete: true,
		})
		defer func() {
			output := &b.extract.Pages[page.Index]
			output.Complete = b.pdf.Pages[page.Index].Complete
			if b.wantProvenance() {
				output.Operations = b.pdf.Pages[page.Index].Operations
			}
		}()
	}
	previousPage := b.page
	b.page = page.Index
	defer func() { b.page = previousPage }()
	if err := b.interpretPage(page); err != nil {
		if b.extract != nil {
			b.pdf.Pages[page.Index].Complete = false
		}
		return err
	}
	if b.wants(ContentAnnotations) {
		if err := b.readAnnotations(page.Index, dict); err != nil {
			if b.extract != nil {
				b.pdf.Pages[page.Index].Complete = false
			}
			return err
		}
	}
	return nil
}

func (b *semanticBuilder) interpretPage(page DetailedPage) error {
	interpreter := &contentInterpreter{b: b, page: page.Index, resources: page.Resources, state: initialContentState(), textMatrix: IdentityMatrix(), lineMatrix: IdentityMatrix(), positionComplete: true}
	var inputs []Span
	for _, content := range page.Contents {
		object, e := b.doc.ResolveObject(content)
		if e != nil {
			return e
		}
		stream, ok := object.Value.(Stream)
		if !ok {
			return fmt.Errorf("page Contents member is not a stream at %+v", object.Span)
		}
		source, e := interpreter.contentSource(stream)
		if e != nil {
			return e
		}
		inputs = append(inputs, Span{Source: source.ID, End: source.Size})
	}
	if len(inputs) > 0 {
		source, e := document.JoinContentSources(b.doc, inputs)
		if e != nil {
			return e
		}
		if err := interpreter.interpretSource(source); err != nil {
			return err
		}
	}
	if len(interpreter.operands) > 0 {
		return fmt.Errorf("trailing content operands at %+v", interpreter.operands[0].Span)
	}
	if interpreter.inText || len(interpreter.stack) > 0 {
		if err := b.diag("unbalanced-content-state", "Unclosed text object or saved graphics state", page.Object.Span); err != nil {
			return err
		}
	}
	return nil
}

func (b *semanticBuilder) readAnnotations(pageIndex int, dict Dictionary) error {
	if annots, ok, e := b.get(dict, "Annots"); e != nil {
		return e
	} else if ok {
		array, ok := annots.Value.(Array)
		if !ok {
			return fmt.Errorf("invalid Annots at %+v", annots.Span)
		}
		for _, item := range array.Items {
			if err := b.chargeSemantic("annotation", item.Span); err != nil {
				return err
			}
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
			a := Annotation{Page: pageIndex, Object: annotation, Subtype: subtype}
			if rectangle, ok, e := b.get(ad, "Rect"); e != nil {
				return e
			} else if ok {
				a.Rect, e = b.rect(rectangle)
				if e != nil {
					return e
				}
			}
			b.emitAnnotation(a)
		}
	}
	return nil
}
