package parser

import (
	"fmt"
	"io"
)

// ContentKind selects independently emitted content categories.
type ContentKind uint8

const (
	ContentText ContentKind = 1 << iota
	ContentGraphics
	ContentImages
	ContentAnnotations
	ContentAll = ContentText | ContentGraphics | ContentImages | ContentAnnotations
)

// ParseOptions controls work and retained output. Zero options extract Unicode
// text and compact font metadata. Glyphs requires text and enables Positions.
// Positions use unrotated page user space, as in the detailed API.
type ParseOptions struct {
	Content     ContentKind
	Positions   bool
	Styles      bool
	Glyphs      bool
	Provenance  bool
	ReadOptions ReadOptions
}

// Result contains only requested output, in content execution order within
// each kind. It does not reconstruct reading order or render pixels. Treat
// results as read-only. Without Provenance, no Document or DetailedPDF is retained.
type Result struct {
	Content        ContentKind
	Pages          []ExtractedPage
	ImageResources []ExtractedImageResource `json:",omitempty"`
	Diagnostics    []ResultDiagnostic       `json:",omitempty"`
	// Document is present only with Provenance. Its lazy methods are not
	// concurrent-safe. It owns the snapshot; no Close call is required.
	Document *Document `json:"-"`
	options  ParseOptions
}

// ResultDiagnostic reports a requested interpretation limitation. Page is
// zero based, or -1 for a document-level issue. Span needs Provenance to resolve.
type ResultDiagnostic struct {
	Severity      Severity
	Code, Message string
	Page          int
	Span          *Span `json:",omitempty"`
}

// ParseFile snapshots a file and directly emits selected content. The file is
// closed before return. A semantic error can accompany a partial Result;
// callers must check err even when the result is non-nil.
func ParseFile(path string, options ParseOptions) (*Result, error) {
	options, err := normalizeParseOptions(options)
	if err != nil {
		return nil, err
	}
	doc, err := LoadDocument(path, options.ReadOptions)
	if err != nil {
		return nil, err
	}
	return buildResult(doc, options)
}

// ParseReader takes one bounded input snapshot and never closes r. Content
// kinds share one interpreter; reused Forms still execute under each caller's
// state. This API does not promise bounded total process memory or streaming I/O.
func ParseReader(r io.ReaderAt, size int64, options ParseOptions) (*Result, error) {
	options, err := normalizeParseOptions(options)
	if err != nil {
		return nil, err
	}
	doc, err := ReadDocument(r, size, options.ReadOptions)
	if err != nil {
		return nil, err
	}
	return buildResult(doc, options)
}

func normalizeParseOptions(options ParseOptions) (ParseOptions, error) {
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

func buildResult(doc *Document, options ParseOptions) (*Result, error) {
	result := &Result{Content: options.Content, Pages: []ExtractedPage{}, options: options}
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
	return b.result == nil || b.result.Content&kind != 0
}

func (b *semanticBuilder) wantPositions() bool {
	return b.result == nil || b.result.options.Positions
}

func (b *semanticBuilder) wantStyles() bool {
	return b.result == nil || b.result.options.Styles
}

func (b *semanticBuilder) wantGlyphs() bool {
	return b.result == nil || b.result.options.Glyphs
}

func (b *semanticBuilder) wantProvenance() bool {
	return b.result == nil || b.result.options.Provenance
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

func (p *Result) addDiagnostic(diagnostic Diagnostic, page int) {
	d := ResultDiagnostic{
		Severity: diagnostic.Severity, Code: diagnostic.Code, Message: diagnostic.Message, Page: page,
	}
	if p.options.Provenance {
		d.Span = &diagnostic.Span
	}
	p.Diagnostics = append(p.Diagnostics, d)
}
