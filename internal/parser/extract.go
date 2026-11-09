package parser

import (
	"errors"
	"fmt"
	"io"

	"github.com/MyungSub0519/gopd/internal/document"
	"github.com/MyungSub0519/gopd/internal/pdfmodel"
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

// DetailLevel selects retained information and work, not PDF support or accuracy.
type DetailLevel uint8

const (
	// DetailBasic retains selected content and compact font metadata.
	DetailBasic DetailLevel = iota
	// DetailLayout adds text/image positions and paint styles.
	DetailLayout
	// DetailFull adds glyphs, operations, source provenance and a detailed view.
	// It does not render pixels, execute font programs or enable unselected kinds.
	DetailFull
)

// ParseOptions selects content and one fixed detail level. Zero options return
// text with DetailBasic. Graphics always include paths and annotations include
// rectangles. Coordinates use unrotated PDF user space at every level.
type ParseOptions struct {
	Content     ContentKind
	Detail      DetailLevel
	ReadOptions document.ReadOptions
}

// Result contains only requested output, in content execution order within
// each kind. It does not reconstruct reading order or render pixels. Treat
// results as read-only. Only DetailFull retains a Document and DetailedPDF.
type Result struct {
	Content        ContentKind
	Detail         DetailLevel `json:",omitempty"`
	Pages          []ExtractedPage
	ImageResources []ExtractedImageResource `json:",omitempty"`
	Diagnostics    []ResultDiagnostic       `json:",omitempty"`
	// Document is present only at DetailFull. Its lazy methods are not
	// concurrent-safe. Call Close when finished with its sources and lazy methods.
	Document *document.Document `json:"-"`
	details  *DetailedPDF
}

// Details returns the selected detailed view at DetailFull, otherwise nil.
// It shares the input Document and immutable data with Result, without parsing
// again. Content still selects classified elements; operations describe all
// scanned content. The detailed view and Document are excluded from Result JSON.
func (p *Result) Details() *DetailedPDF {
	if p == nil {
		return nil
	}
	return p.details
}

// Close releases the retained Document at DetailFull. It is safe to call more
// than once, on nil results, and at other detail levels. Materialized output
// remains usable; shared document sources and lazy methods become unavailable.
// Caller-owned readers are never closed.
func (p *Result) Close() error {
	if p == nil || p.Document == nil {
		return nil
	}
	return p.Document.Close()
}

// ResultDiagnostic reports a requested interpretation limitation. Page is
// zero based, or -1 for a document-level issue. Span is present at DetailFull.
type ResultDiagnostic struct {
	Severity      pdfmodel.Severity
	Code, Message string
	Page          int
	Span          *pdfmodel.Span `json:",omitempty"`
}

// ParseFile reads a file through a bounded buffer and emits selected content.
// Basic and Layout close the file before return. Full keeps it open until Close.
// A semantic error can accompany a partial Result; check err and close any
// non-nil Full result, including when an error is returned.
func ParseFile(path string, options ParseOptions) (*Result, error) {
	options, err := normalizeParseOptions(options)
	if err != nil {
		return nil, err
	}
	doc, err := document.Load(path, options.ReadOptions)
	if err != nil {
		if doc != nil {
			err = errors.Join(err, doc.Close())
		}
		return nil, err
	}
	return buildResult(doc, options)
}

// ParseReader reads through a bounded buffer and never closes r. Keep r open
// and unchanged during the call and, at Full, until the returned Result is closed.
// Close any non-nil Full result, including partial results returned with errors.
// Decoded streams and results retain their existing memory budgets; this API
// does not promise bounded total process memory.
func ParseReader(r io.ReaderAt, size int64, options ParseOptions) (*Result, error) {
	options, err := normalizeParseOptions(options)
	if err != nil {
		return nil, err
	}
	doc, err := document.Read(r, size, options.ReadOptions)
	if err != nil {
		if doc != nil {
			err = errors.Join(err, doc.Close())
		}
		return nil, err
	}
	return buildResult(doc, options)
}

func normalizeParseOptions(options ParseOptions) (ParseOptions, error) {
	if options.Detail > DetailFull {
		return options, fmt.Errorf("unknown detail level: %d", options.Detail)
	}
	if options.Content == 0 {
		options.Content = ContentText
	}
	if options.Content & ^ContentAll != 0 {
		return options, fmt.Errorf("unknown extraction content flags: %d", options.Content)
	}
	return options, nil
}

func buildResult(doc *document.Document, options ParseOptions) (result *Result, err error) {
	if options.Detail != DetailFull {
		defer func() { err = errors.Join(err, doc.Close()) }()
	}
	result = &Result{Content: options.Content, Detail: options.Detail, Pages: []ExtractedPage{}}
	if options.Detail == DetailFull {
		result.Document = doc
	}
	detail, err := buildPDF(doc, result)
	if options.Detail == DetailFull {
		result.details = detail
	}
	for _, diagnostic := range doc.Structure.Diagnostics {
		result.addDiagnostic(diagnostic, -1)
	}
	return result, err
}

func (b *semanticBuilder) wants(kind ContentKind) bool {
	return b.result == nil || b.result.Content&kind != 0
}

func (b *semanticBuilder) wantPositions() bool {
	return b.result == nil || b.result.Detail >= DetailLayout
}

func (b *semanticBuilder) wantStyles() bool {
	return b.result == nil || b.result.Detail >= DetailLayout
}

func (b *semanticBuilder) wantGlyphs() bool {
	return b.result == nil || b.result.Detail == DetailFull
}

func (b *semanticBuilder) wantProvenance() bool {
	return b.result == nil || b.result.Detail == DetailFull
}

func (b *semanticBuilder) keepDetails() bool {
	return b.result == nil || b.result.Detail == DetailFull
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

func (p *Result) addDiagnostic(diagnostic pdfmodel.Diagnostic, page int) {
	d := ResultDiagnostic{
		Severity: diagnostic.Severity, Code: diagnostic.Code, Message: diagnostic.Message, Page: page,
	}
	if p.Detail == DetailFull {
		d.Span = &diagnostic.Span
	}
	p.Diagnostics = append(p.Diagnostics, d)
}
