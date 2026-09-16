package content

import (
	"fmt"

	"github.com/MyungSub0519/gopd/internal/model"
	"github.com/MyungSub0519/gopd/internal/structure"
)

// BuildPDF interprets page contents. Invalid syntax and traversal limits return
// an error and any partial result. Unsupported effects are retained as
// operations and diagnostics.
func BuildPDF(d *structure.Document) (*DetailedPDF, error) {
	if d == nil {
		return nil, fmt.Errorf("nil Document")
	}
	p := &DetailedPDF{Document: d, Structure: &d.Structure}
	if d.Encrypted {
		return p, fmt.Errorf("semantic decoding of encrypted PDF is unsupported")
	}
	b := &semanticBuilder{doc: d, pdf: p, fonts: make(map[model.Span]int), images: make(map[model.Span]int), activeForms: make(map[model.Span]bool), activePages: make(map[model.Span]bool), page: -1, maxDepth: 128, maxObjects: 1000000}
	b.cmaps = make(map[model.Span]*CMap)
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
	err = b.walkPages(pages, map[model.Name]model.Object{}, 0)
	return p, err
}

// Details returns the already parsed detailed snapshot without re-reading the
// file.
func (p *PDF) Details() *DetailedPDF {
	if p == nil {
		return nil
	}
	return p.details
}
