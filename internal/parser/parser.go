package parser

import (
	"errors"
	"fmt"
	"io"

	"github.com/MyungSub0519/gopd/internal/common/document"
	"github.com/MyungSub0519/gopd/internal/common/pdfmodel"
	"github.com/MyungSub0519/gopd/internal/common/syntax"
)

// DetailedPDF is a semantic snapshot. Slices and the underlying Document are read-only
// by convention. Elements follow content execution order, not reading order.
type DetailedPDF struct {
	Pages           []DetailedPage
	Texts           []DetailedText
	Graphics        []DetailedGraphic
	Images          []DetailedImage
	ImageResources  []ImageResource
	Fonts           []Font
	Annotations     []Annotation
	Structure       *Structure
	Document        *Document
	Diagnostics     []Diagnostic
	diagnosticPages []int // emission context, including reused Form streams
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

// ParsePDF snapshots a file and returns basic text and graphics grouped by page.
// It retains eager detailed analysis for Details; it is not selective parsing.
// A semantic failure can return both a partial PDF and an error. Callers must
// check err before treating the result as successful. No Close call is required.
func ParsePDF(path string) (*PDF, error) {
	detail, err := Open(path)
	return basicPDF(detail), err
}

// Open returns detailed contents, resources, operations, and byte provenance.
// Use ParsePDF for a basic result. Both functions close the input file.
func Open(path string) (*DetailedPDF, error) {
	d, err := ParseFile(path)
	if err != nil {
		return nil, err
	}
	return BuildPDF(d)
}

// Read parses a bounded snapshot of r and returns detailed page contents.
// The caller retains ownership of r; no Close call is required on the result.
func Read(r io.ReaderAt, size int64) (*DetailedPDF, error) {
	d, err := Parse(r, size)
	if err != nil {
		return nil, err
	}
	return BuildPDF(d)
}

// BuildPDF interprets page contents. Invalid syntax and traversal limits return
// an error and any partial result. Unsupported effects are retained as operations
// and diagnostics. No pixel rendering, OCR, or reading-order reconstruction occurs.
func BuildPDF(d *Document) (*DetailedPDF, error) {
	return buildPDF(d, nil)
}

func buildPDF(d *Document, extraction *Extraction) (*DetailedPDF, error) {
	if d == nil {
		return nil, fmt.Errorf("nil Document")
	}
	p := &DetailedPDF{Document: d, Structure: &d.Structure}
	if d.Encrypted {
		return p, fmt.Errorf("semantic decoding of encrypted PDF is unsupported")
	}
	b := &semanticBuilder{
		extract:     extraction,
		doc:         d,
		pdf:         p,
		fonts:       make(map[Span]int),
		images:      make(map[Span]int),
		activeForms: make(map[Span]bool),
		activePages: make(map[Span]bool),
		page:        -1,
		maxDepth:    128,
		maxObjects:  1000000,
	}
	b.cmaps = make(map[Span]*CMap)
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
	err = b.walkPages(pages, map[Name]Object{}, 0)
	return p, err
}

// ParseFile snapshots a file and prepares low-level object access. It closes
// the file before returning. At most one ReadOptions value may be supplied;
// a structural error can return a partial Document together with the error.
func ParseFile(path string, options ...ReadOptions) (*Document, error) {
	return document.ParseFile(path, options...)
}

// Parse snapshots r; it never closes a caller-owned ReaderAt.
func Parse(r io.ReaderAt, size int64, options ...ReadOptions) (*Document, error) {
	return document.Parse(r, size, options...)
}

// Lex scans one complete byte range, preserving whitespace and comments as
// tokens and appending an EOF token. Spans use offset as the range's absolute
// position in source. Tokens are limited to 16 MiB each and the result to
// 1,048,576 non-EOF tokens. On error the valid token prefix is returned.
// Stream payloads must be excluded by the caller; they are not PDF syntax.
func Lex(data []byte, source SourceID, offset int64) ([]Token, error) {
	return syntax.Lex(data, source, offset)
}

// ParseObject parses one direct PDF object or indirect reference. consumed is
// relative to data and includes leading whitespace and comments, but excludes
// trailing trivia. Spans are absolute within source and exclude leading trivia.
// Bare keywords other than true, false and null are not object values.
// Default limits are 256 container levels, 16 MiB per token and 1,048,576 values.
func ParseObject(data []byte, source SourceID, offset int64) (object Object, consumed int, err error) {
	return syntax.ParseObject(data, source, offset)
}

// Int converts a direct PDF Integer into int64, rejecting other types and
// out-of-range values. Resolve indirect references before calling Int.
func Int(object Object) (int64, error) { return pdfmodel.Int(object) }

// Number converts a direct Integer or Real into a finite float64. It does not
// resolve indirect references or preserve exact decimal precision.
func Number(object Object) (float64, error) { return pdfmodel.Number(object) }

// IsStream reports whether object directly contains a Stream. It does not
// follow references; use Document.ResolveObject first when necessary.
func IsStream(object Object) bool { return pdfmodel.IsStream(object) }

// IdentityMatrix returns a transformation that leaves coordinates unchanged.
func IdentityMatrix() Matrix { return pdfmodel.IdentityMatrix() }

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

// Visits count even when the underlying object or decoded source is cached.
func (b *semanticBuilder) chargeSemantic(kind string, span Span) error {
	return b.chargeSemanticWork(1, kind, span)
}

func (b *semanticBuilder) chargeSemanticWork(count int, kind string, span Span) error {
	if count > b.maxSemanticObjects-b.semanticObjects {
		return fmt.Errorf("%w: semantic object limit while visiting %s at %+v", ErrLimit, kind, span)
	}
	b.semanticObjects += count
	return nil
}

// Content operands, expanded numeric resources and retained style components
// share the BuildPDF value budget; the Document's syntax budget is independent.
func (b *semanticBuilder) chargeValues(count int, kind string, span Span) error {
	if count > b.maxValues-b.semanticValues {
		return fmt.Errorf("%w: semantic value count limit for %s at %+v", ErrLimit, kind, span)
	}
	b.semanticValues += count
	return nil
}

func (b *semanticBuilder) chargeStyle(state GraphicsState, span Span) error {
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
