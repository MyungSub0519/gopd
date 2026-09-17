# Selective extraction

Approved direction: the September 17 conversation requested implementation after reviewing a shared interpreter with optional content handlers. Preserve all existing entry points and `PDF.Details()` behavior.

## Contract

- Add `Extract(path string, options ExtractOptions) (*Extraction, error)` and `ExtractReader(r io.ReaderAt, size int64, options ExtractOptions) (*Extraction, error)`.
- `ExtractOptions.Content` is a bit set: `ContentText`, `ContentGraphics`, `ContentImages`, `ContentAnnotations`, `ContentAll`. Zero defaults to text. Unknown bits fail before input is read.
- Independent `Positions`, `Styles`, `Glyphs`, and `Provenance` flags control optional data. Glyphs requires text and enables positions. `ReadOptions` carries existing input/work limits.
- Results are grouped by page. Disabled kinds and optional fields are absent from JSON. A page index is zero based. Completeness describes requested interpretation, not validation of skipped resources or rendering.
- Text includes Unicode, shared compact font metadata and decoding completeness. Positions add font size, matrix, rendering mode and positioning completeness. Glyphs adds character positions. Styles add the basic paint style. Graphics includes path/paint geometry. Images includes metadata and optional placement/style, referencing shared image resources; this API does not decode pixels. Annotations include subtype and rectangle.
- Provenance retains a Document (excluded from JSON), operations for interpreted page content and element source spans/call paths. Annotation-only extraction does not interpret page content or record its operations. Without provenance no Document, raw page dictionary, full font/CMap, or detailed snapshot is reachable from the result. Image raw streams remain available through Document only with provenance.
- Return partial extraction with an error when interpretation fails; nil for invalid options or structural input errors. Inline images and encrypted content remain unsupported. Budgets charge traversal and parsing even for unselected content. Skipping is not full-document validation.

## Engine

One common object loader, decoder, page traversal and content interpreter serves old and new APIs. Each content execution is scanned once per interpreter invocation (reused Forms execute again under their caller state). Optional handlers avoid constructing disabled result objects. Font decoding is omitted without text; widths and glyph positions are omitted for Unicode-only extraction. Required state, resource lookup for XObject subtype, and Form traversal remain shared. Paths are retained when graphics or style/clipping needs them. Basic styles and compact elements are emitted directly, without building a DetailedPDF projection.

A sequential internal syntax cursor replaces full token buffering on the extraction path. Existing detailed entry points retain their current scanning/error behavior. Input snapshots and decoded caches are still retained during a call; provenance opts into their post-call retention. This work does not promise bounded total process memory or streaming file I/O.

## Validation

Compare all selected outputs to existing detailed extraction on the synthetic fixture and nested Forms. Cover all selection combinations, styles/positions/glyphs, null/invalid options, malformed skipped resources, content arrays split inside operands, provenance byte access, partial errors and limits. Confirm one ReaderAt snapshot and no hidden DetailedPDF ownership. Benchmark full versus selective extraction with allocation reporting. Run build, vet and the full test suite.
