package parser

import "fmt"

type Annotation struct {
	Page    int
	Object  Object
	Subtype Name
	Rect    Rect
}

// ExtractedAnnotation contains annotation metadata; appearance streams are not executed.
type ExtractedAnnotation struct {
	Subtype Name
	Rect    Rect
	Source  *Span `json:",omitempty"`
}

func (b *semanticBuilder) emitAnnotation(annotation Annotation) {
	if b.result == nil {
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
	page := &b.result.Pages[annotation.Page]
	page.Annotations = append(page.Annotations, output)
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
