package gopd

import (
	"errors"
	"fmt"
)

type semanticBuilder struct {
	extract            *Extraction
	fontInfos          map[int]*FontInfo
	doc                *Document
	pdf                *DetailedPDF
	fonts              map[Span]int
	images             map[Span]int
	activeForms        map[Span]bool
	activePages        map[Span]bool
	page               int
	operations         int
	pageNodes          int
	glyphCodes         int
	cmaps              map[Span]*CMap
	cmapEntries        int
	unicodeBytes       int64
	maxUnicodeBytes    int64
	clipReferences     int
	widthEntries       int
	maxDepth           int
	maxObjects         int
	maxSemanticObjects int
	semanticObjects    int
	maxContentBytes    int64
	contentBytes       int64
	maxValues          int
	semanticValues     int
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
	if err == nil {
		if _, null := object.Value.(Null); null {
			return Object{}, false, nil
		}
	}
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

func (b *semanticBuilder) diag(code, message string, span Span) error {
	if err := b.chargeSemantic("diagnostic", span); err != nil {
		return err
	}
	bytes := int64(len(code)) + int64(len(message))
	if bytes > b.maxUnicodeBytes-b.unicodeBytes {
		return fmt.Errorf("%w: expanded diagnostic byte limit at %+v", ErrLimit, span)
	}
	b.unicodeBytes += bytes
	diagnostic := Diagnostic{Severity: SeverityWarning, Code: code, Message: message, Span: span}
	if b.extract == nil {
		b.pdf.Diagnostics = append(b.pdf.Diagnostics, diagnostic)
		b.pdf.diagnosticPages = append(b.pdf.diagnosticPages, b.page)
	} else {
		b.extract.addDiagnostic(diagnostic, b.page)
	}
	if b.page >= 0 {
		b.pdf.Pages[b.page].Complete = false
	}
	return nil
}

func (b *semanticBuilder) rect(object Object) (Rect, error) {
	values, err := b.numbers(object, 4)
	if err != nil {
		return Rect{}, err
	}
	return Rect{
		Min: Point{X: min(values[0], values[2]), Y: min(values[1], values[3])},
		Max: Point{X: max(values[0], values[2]), Y: max(values[1], values[3])},
	}, nil
}

func (b *semanticBuilder) numbers(object Object, n int) ([]float64, error) {
	array, ok := object.Value.(Array)
	if !ok {
		return nil, fmt.Errorf("expected number array at %+v", object.Span)
	}
	if n >= 0 && len(array.Items) != n {
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
