package gopd

import (
	"fmt"
	"io"
)

// Extract snapshots a file and directly emits selected content. The file is
// closed before return. A semantic error can accompany a partial Extraction;
// callers must check err even when the result is non-nil.
func Extract(path string, options ExtractOptions) (*Extraction, error) {
	options, err := normalizeExtractOptions(options)
	if err != nil {
		return nil, err
	}
	doc, err := ParseFile(path, options.ReadOptions)
	if err != nil {
		return nil, err
	}
	return extractDocument(doc, options)
}

// ExtractReader takes one bounded input snapshot and never closes r. Content
// kinds share one interpreter; reused Forms still execute under each caller's
// state. This API does not promise bounded total process memory or streaming I/O.
func ExtractReader(r io.ReaderAt, size int64, options ExtractOptions) (*Extraction, error) {
	options, err := normalizeExtractOptions(options)
	if err != nil {
		return nil, err
	}
	doc, err := Parse(r, size, options.ReadOptions)
	if err != nil {
		return nil, err
	}
	return extractDocument(doc, options)
}

func normalizeExtractOptions(options ExtractOptions) (ExtractOptions, error) {
	if options.Content == 0 {
		options.Content = ContentText
	}
	if options.Content & ^ContentAll != 0 {
		return options, fmt.Errorf("unknown extraction content flags: %d", options.Content)
	}
	if options.Glyphs {
		if options.Content&ContentText == 0 {
			return options, fmt.Errorf("glyph extraction requires text")
		}
		options.Positions = true
	}
	return options, nil
}

func extractDocument(doc *Document, options ExtractOptions) (*Extraction, error) {
	result := &Extraction{Content: options.Content, Pages: []ExtractedPage{}, options: options}
	if options.Provenance {
		result.Document = doc
	}
	_, err := buildPDF(doc, result)
	for _, diagnostic := range doc.Structure.Diagnostics {
		result.addDiagnostic(diagnostic, -1)
	}
	return result, err
}

func (b *semanticBuilder) wants(kind ContentKind) bool {
	return b.extract == nil || b.extract.Content&kind != 0
}

func (b *semanticBuilder) wantPositions() bool {
	return b.extract == nil || b.extract.options.Positions
}

func (b *semanticBuilder) wantStyles() bool {
	return b.extract == nil || b.extract.options.Styles
}

func (b *semanticBuilder) wantGlyphs() bool {
	return b.extract == nil || b.extract.options.Glyphs
}

func (b *semanticBuilder) wantProvenance() bool {
	return b.extract == nil || b.extract.options.Provenance
}

func (b *semanticBuilder) wantPaths() bool {
	return b.wants(ContentGraphics) || b.wantStyles()
}

func (b *semanticBuilder) wantTransforms() bool {
	return b.wantPositions() || b.wants(ContentGraphics) || b.wantStyles()
}

func (b *semanticBuilder) wantPageContent() bool {
	return b.wants(ContentText | ContentGraphics | ContentImages)
}

func (p *Extraction) addDiagnostic(diagnostic Diagnostic, page int) {
	d := ExtractionDiagnostic{
		Severity: diagnostic.Severity, Code: diagnostic.Code, Message: diagnostic.Message, Page: page,
	}
	if p.options.Provenance {
		d.Span = &diagnostic.Span
	}
	p.Diagnostics = append(p.Diagnostics, d)
}
