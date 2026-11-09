# Content and detail levels

Use `gopd.ParseFile` or `gopd.ParseReader` with `ParseOptions.Content` and
`ParseOptions.Detail`. Both return `*gopd.Result`; no type assertion is needed.

| Detail | Retained information |
| --- | --- |
| `DetailBasic` (zero) | Selected content and compact font metadata |
| `DetailLayout` | Basic information plus text/image positions and paint styles |
| `DetailFull` | Layout information plus glyphs, operations, source spans, the input Document, and `Result.Details()` |

Zero Content selects text. Graphics always include path geometry and annotations
always include rectangles, including at Basic. Full does not enable unselected
content. Annotation-only requests skip page drawing at every level. Operations
at Full describe scanned content, including operations from unselected kinds.
The classified elements in `Details()` still follow Content selection.

`Details()` is nil outside Full (and for a nil Result). It exposes the existing
DetailedPDF model, including selected elements' full graphics state, drawing
order, font resources and raw character codes. Its Document is the same instance
as Result.Document. Extracted glyphs, operations and immutable geometry can share
storage with the detailed view. Treat all returned data as read-only.

Detail selects retained information and work, not conformance or rendering
quality. No level renders pixels, executes font programs, performs OCR, or
reconstructs reading order. Layout resolves font metrics; Full additionally
resolves embedded font references for inspection without executing the programs.
Unsupported features remain diagnostics; semantic errors can return partial
results. Always check the error and completeness flags, including at Basic.

The existing PDF 1.7 / ISO 32000-1:2008 interpretation rules remain unchanged:
content streams share execution state (7.8.2, 8.4, 9.4). This is an API/output
selection change, not a new claim of PDF conformance. The existing [resource
limit contracts](resource-limits.md) apply to all levels and both result views.
Full runs the interpreter once; it does not run a second unlimited analysis.

## Input lifetime

Original input is read on demand through a bounded buffer. Set
`ParseOptions.ReadOptions.BufferSize` in bytes; zero selects 64 KiB and negative
values are rejected before opening or reading input. Decoded streams and results
retain their existing memory budgets. This does not bound total process memory.

Basic and Layout finish reading input and close their internal Document before
returning, including on semantic errors. They never close a caller-owned ReaderAt.
Full retains its Document: call `Result.Close()` when finished, including for a
non-nil partial result returned with an error. Keep the file or ReaderAt open and
unchanged while its Document remains live. `ParseFile` owns its file and closes
it with the result; `ParseReader` always leaves ownership with the caller.

```go
result, err := gopd.ParseFile(path, gopd.ParseOptions{Detail: gopd.DetailFull})
if result != nil {
    defer result.Close()
}
if err != nil {
    return err
}
// Use result.Document and result.Details() before the deferred Close.
```

Close is idempotent and is a no-op for Basic, Layout and nil results. Materialized
output remains usable after Close, but all Document sources and lazy methods
become unavailable. Result, Details and Document share one lifetime: closing any
one closes their shared Document. Lazy methods and Close are not concurrent-safe.

## Migration

The independent `Positions`, `Styles`, `Glyphs`, and `Provenance` fields are
replaced by the fixed Detail levels. This is a source-breaking ParseOptions
change: use Layout for positions/styles or Full for glyphs/provenance. Arbitrary
old flag combinations are no longer available; the presets can do more work.
Unknown detail levels are rejected before opening or reading the input.

`ParsePDF`, `Open`, and `Read` retain their prior return types and extracted data,
but now require `PDF.Close()` or `DetailedPDF.Close()` to release their Document.
`LoadDocument` and `ReadDocument` require `Document.Close()`. Code that closed or
modified a borrowed ReaderAt immediately after parsing must keep it open and
unchanged until closing the retained Document. Close partial non-nil results too.
Prefer ParseFile/ParseReader for new code. Low-level
Document and syntax helpers remain in gopd. The CLI uses the new entry points.

JSON contains the selected page output, including operations at Full. It omits
Document and the private detailed view; it is not a complete source snapshot.
Result.Detail records the level (omitted from JSON when zero).
