// Package parser interprets PDF pages, content operations, fonts, images, and
// annotations into basic, detailed, or selectively extracted results. It owns
// the shared content state and uses document, syntax, and
// pdfmodel for input, object access, syntax, and PDF values.
// The root gopd package exposes its entry points and result types to callers.
// Content-specific files keep their result types, interpretation, and emission
// together; one interpreter preserves execution order and shared graphics state.
package parser

import (
	"errors"
	"fmt"
	"io"

	"github.com/MyungSub0519/gopd/internal/document"
	"github.com/MyungSub0519/gopd/internal/pdfmodel"
)

// DetailedPDF retains eager semantic results and a Document for lazy access.
// Slices and the Document are read-only by convention. Elements follow content
// execution order, not reading order. Close releases the retained Document.
type DetailedPDF struct {
	Pages           []DetailedPage
	Texts           []DetailedText
	Graphics        []DetailedGraphic
	Images          []DetailedImage
	ImageResources  []ImageResource
	Fonts           []Font
	Annotations     []Annotation
	Structure       *pdfmodel.Structure
	Document        *document.Document
	Diagnostics     []pdfmodel.Diagnostic
	diagnosticPages []int // emission context, including reused Form streams
}

// Close releases the shared Document and invalidates its sources and lazy
// methods. Materialized results remain usable. It is safe to call more than
// once or on a nil result, and never closes a caller-owned reader.
func (p *DetailedPDF) Close() error {
	if p == nil || p.Document == nil {
		return nil
	}
	return p.Document.Close()
}

type ElementKind uint8

const (
	ElementText ElementKind = iota + 1
	ElementGraphic
	ElementImage
)

type ElementRef struct {
	Kind  ElementKind
	Index int
}

type semanticBuilder struct {
	result             *Result
	fontInfos          map[int]*FontInfo
	doc                *document.Document
	pdf                *DetailedPDF
	fonts              map[pdfmodel.Span]int
	images             map[pdfmodel.Span]int
	activeForms        map[pdfmodel.Span]bool
	activePages        map[pdfmodel.Span]bool
	page               int
	operations         int
	pageNodes          int
	glyphCodes         int
	cmaps              map[pdfmodel.Span]*CMap
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

// ParsePDF reads a file and returns basic text and graphics grouped by page.
// It retains eager detailed analysis for Details; it is not selective parsing.
// A semantic failure can return both a partial PDF and an error. Callers must
// check err and close any non-nil result, including partial results.
func ParsePDF(path string) (*PDF, error) {
	detail, err := Open(path)
	return basicPDF(detail), err
}

// Open returns detailed contents, resources, operations, and byte provenance.
// Close any non-nil result to release its input file, including partial results.
func Open(path string) (*DetailedPDF, error) {
	d, err := document.Load(path)
	if err != nil {
		if d != nil {
			err = errors.Join(err, d.Close())
		}
		return nil, err
	}
	return BuildPDF(d)
}

// Read parses r through a bounded buffer and returns detailed page contents.
// Keep r open and unchanged until closing the returned result. Close any non-nil
// result, including partial results. The caller retains ownership of r.
func Read(r io.ReaderAt, size int64) (*DetailedPDF, error) {
	d, err := document.Read(r, size)
	if err != nil {
		if d != nil {
			err = errors.Join(err, d.Close())
		}
		return nil, err
	}
	return BuildPDF(d)
}

// BuildPDF interprets page contents. Invalid syntax and traversal limits return
// an error and any partial result. Unsupported effects are retained as operations
// and diagnostics. No pixel rendering, OCR, or reading-order reconstruction occurs.
// The result shares d; closing either closes their shared source access.
func BuildPDF(d *document.Document) (*DetailedPDF, error) {
	return buildPDF(d, nil)
}

func buildPDF(d *document.Document, result *Result) (*DetailedPDF, error) {
	if d == nil {
		return nil, fmt.Errorf("nil Document")
	}
	p := &DetailedPDF{Document: d, Structure: &d.Structure}
	if d.Encrypted {
		return p, fmt.Errorf("semantic decoding of encrypted PDF is unsupported")
	}
	b := &semanticBuilder{
		result:      result,
		doc:         d,
		pdf:         p,
		fonts:       make(map[pdfmodel.Span]int),
		images:      make(map[pdfmodel.Span]int),
		activeForms: make(map[pdfmodel.Span]bool),
		activePages: make(map[pdfmodel.Span]bool),
		page:        -1,
		maxDepth:    128,
		maxObjects:  1000000,
	}
	b.cmaps = make(map[pdfmodel.Span]*CMap)
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
	b.maxSemanticObjects = d.Options.Limits.MaxSemanticObjects
	if b.maxSemanticObjects == 0 {
		b.maxSemanticObjects = b.maxObjects
	}
	b.maxContentBytes = d.Options.Limits.MaxContentBytes
	if b.maxContentBytes == 0 {
		b.maxContentBytes = 256 << 20
	}
	b.maxValues = d.Options.Limits.MaxValues
	if b.maxValues == 0 {
		b.maxValues = 1 << 20
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
	err = b.walkPages(pages, map[pdfmodel.Name]pdfmodel.Object{}, 0)
	return p, err
}

func (b *semanticBuilder) get(dict pdfmodel.Dictionary, key pdfmodel.Name) (pdfmodel.Object, bool, error) {
	object, err := dict.Get(key)
	if errors.Is(err, pdfmodel.ErrMissingKey) {
		return pdfmodel.Object{}, false, nil
	}
	if err != nil {
		return pdfmodel.Object{}, false, err
	}
	object, err = b.doc.ResolveObject(object)
	if err == nil {
		if _, null := object.Value.(pdfmodel.Null); null {
			return pdfmodel.Object{}, false, nil
		}
	}
	return object, true, err
}

func (b *semanticBuilder) name(dict pdfmodel.Dictionary, key pdfmodel.Name) (pdfmodel.Name, error) {
	object, ok, err := b.get(dict, key)
	if err != nil || !ok {
		return "", err
	}
	name, ok := object.Value.(pdfmodel.Name)
	if !ok {
		return "", fmt.Errorf("/%s must be a name at %+v", key, object.Span)
	}
	return name, nil
}

func semDictionary(object pdfmodel.Object) (pdfmodel.Dictionary, error) {
	var dict pdfmodel.Dictionary
	switch value := object.Value.(type) {
	case pdfmodel.Dictionary:
		dict = value
	case pdfmodel.Stream:
		dict = value.Dictionary
	default:
		return pdfmodel.Dictionary{}, fmt.Errorf("expected dictionary at %+v", object.Span)
	}
	seen := make(map[pdfmodel.Name]bool, len(dict.Entries))
	for _, entry := range dict.Entries {
		if seen[entry.Key] {
			return pdfmodel.Dictionary{}, fmt.Errorf("duplicate dictionary key /%s at %+v", entry.Key, entry.KeySpan)
		}
		seen[entry.Key] = true
	}
	return dict, nil
}

func semID(object pdfmodel.Object) pdfmodel.ObjectID {
	if ref, ok := object.Value.(pdfmodel.Reference); ok {
		return ref.ID
	}
	return pdfmodel.ObjectID{}
}

func (b *semanticBuilder) diag(code, message string, span pdfmodel.Span) error {
	if err := b.chargeSemantic("diagnostic", span); err != nil {
		return err
	}
	bytes := int64(len(code)) + int64(len(message))
	if bytes > b.maxUnicodeBytes-b.unicodeBytes {
		return fmt.Errorf("%w: expanded diagnostic byte limit at %+v", pdfmodel.ErrLimit, span)
	}
	b.unicodeBytes += bytes
	diagnostic := pdfmodel.Diagnostic{Severity: pdfmodel.SeverityWarning, Code: code, Message: message, Span: span}
	if b.keepDetails() {
		b.pdf.Diagnostics = append(b.pdf.Diagnostics, diagnostic)
		b.pdf.diagnosticPages = append(b.pdf.diagnosticPages, b.page)
	}
	if b.result != nil {
		b.result.addDiagnostic(diagnostic, b.page)
	}
	if b.page >= 0 {
		b.pdf.Pages[b.page].Complete = false
	}
	return nil
}

func (b *semanticBuilder) rect(object pdfmodel.Object) (pdfmodel.Rect, error) {
	values, err := b.numbers(object, 4)
	if err != nil {
		return pdfmodel.Rect{}, err
	}
	return pdfmodel.Rect{
		Min: pdfmodel.Point{X: min(values[0], values[2]), Y: min(values[1], values[3])},
		Max: pdfmodel.Point{X: max(values[0], values[2]), Y: max(values[1], values[3])},
	}, nil
}

func (b *semanticBuilder) numbers(object pdfmodel.Object, n int) ([]float64, error) {
	array, ok := object.Value.(pdfmodel.Array)
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
		out[i], err = pdfmodel.Number(value)
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

// Visits count even when the underlying object or decoded source is cached.
func (b *semanticBuilder) chargeSemantic(kind string, span pdfmodel.Span) error {
	return b.chargeSemanticWork(1, kind, span)
}

func (b *semanticBuilder) chargeSemanticWork(count int, kind string, span pdfmodel.Span) error {
	if count > b.maxSemanticObjects-b.semanticObjects {
		return fmt.Errorf("%w: semantic object limit while visiting %s at %+v", pdfmodel.ErrLimit, kind, span)
	}
	b.semanticObjects += count
	return nil
}

// Content operands, expanded numeric resources and retained style components
// share the BuildPDF value budget; the Document's syntax budget is independent.
func (b *semanticBuilder) chargeValues(count int, kind string, span pdfmodel.Span) error {
	if count > b.maxValues-b.semanticValues {
		return fmt.Errorf("%w: semantic value count limit for %s at %+v", pdfmodel.ErrLimit, kind, span)
	}
	b.semanticValues += count
	return nil
}

func (b *semanticBuilder) chargeStyle(state GraphicsState, span pdfmodel.Span) error {
	if !b.wantStyles() {
		return nil
	}
	// Charge every occurrence even when detailed results share the slices.
	// The basic projection copies these components into each element's style.
	for _, values := range [][]float64{state.Dash, state.Stroke.Components, state.Fill.Components} {
		if err := b.chargeValues(len(values), "retained graphics style", span); err != nil {
			return err
		}
	}
	return nil
}
