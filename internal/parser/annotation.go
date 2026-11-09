package parser

import (
	"github.com/MyungSub0519/gopd/internal/pdfmodel"
)

import "fmt"

type Annotation struct {
	Page    int
	Object  pdfmodel.Object
	Subtype pdfmodel.Name
	Rect    pdfmodel.Rect
}

// ExtractedAnnotation contains annotation metadata; appearance streams are not executed.
type ExtractedAnnotation struct {
	Subtype pdfmodel.Name
	Rect    pdfmodel.Rect
	Source  *pdfmodel.Span `json:",omitempty"`
}

func (b *semanticBuilder) emitAnnotation(annotation Annotation) {
	if b.keepDetails() {
		index := len(b.pdf.Annotations)
		b.pdf.Annotations = append(b.pdf.Annotations, annotation)
		b.pdf.Pages[annotation.Page].Annotations = append(b.pdf.Pages[annotation.Page].Annotations, index)
	}
	if b.result == nil {
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

func (b *semanticBuilder) readAnnotations(pageIndex int, dict pdfmodel.Dictionary) error {
	if annots, ok, e := b.get(dict, "Annots"); e != nil {
		return e
	} else if ok {
		array, ok := annots.Value.(pdfmodel.Array)
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
