# Resource limits and interpretation

GoPD reads original input on demand through a bounded ReaderAt buffer instead of
copying the entire file. `ReadOptions.BufferSize` is the buffer capacity in bytes:
zero selects 64 KiB, a positive value selects that capacity, and negative values
are invalid. `MaxFileBytes` still limits the original file size, independently
of the buffer size. Larger accepted files do not require a whole-file allocation.
All content kinds share the same input path. Access follows PDF offsets, so this
is buffered random access rather than a forward-only pass. Revisiting an evicted
block can read it again; each original byte is not guaranteed to be read once.
Tail-marker search uses an additional window of at most 64 KiB plus eight bytes.
Parsed objects, encoded ranges requested for decoding, decoded sources and
results may still occupy memory. `ParsePDF` also retains its detailed result.
Limits bound specific resources; they are not an exact process-memory limit.
Object, token and map overhead and temporary decoding buffers consume additional
memory. Explicit `Document.Bytes` calls allocate the requested range; a range
that cannot fit an addressable slice returns `ErrLimit`. Unfiltered streams whose
known length exceeds the remaining decoded budget are rejected before reading
or allocating their payload.

`ParseFile` and `ParseReader` generate selected results directly. At DetailBasic
and DetailLayout, their returned `Result` does not retain a `Document`, full
font/CMap data, or a detailed result. The input buffer and decoded caches exist
during the call; the Document is closed before return. DetailFull retains Document and a selected DetailedPDF
through Result.Details(), including font resources, raw codes and graphics state.
Both views share one interpretation, the buffered input, glyphs, operations and
immutable geometry; Full does not perform a second analysis. The existing
character-code, clipping and style budgets apply before either view is emitted.
Document and the private detailed view are excluded from Result JSON.
See [detail levels](detail-levels.md) and the [usage example](../README.md#parse-selected-content).

Close Full results, legacy PDF/DetailedPDF results and low-level Documents when
finished, including non-nil partial results. File-backed Documents own their file;
ReaderAt-based Documents never close the caller's reader. Keep input open and
unchanged until its Document is closed. Close invalidates source readers and lazy
Document operations, including cached source access, but materialized output
remains usable. Close is idempotent and is not safe concurrently with lazy access.
This lifetime contract replaces the previous immutable whole-file snapshot.

Use the low-level entry point to set limits before interpreting content:

```go
doc, err := gopd.ReadDocument(readerAt, size, gopd.ReadOptions{
    BufferSize:   64 << 10,
    MaxFileBytes: 32 << 20,
    Limits: gopd.Limits{
        MaxObjects:         100_000,
        MaxValues:          250_000,
        MaxDecodedBytes:    64 << 20,
        MaxContentBytes:    128 << 20,
        MaxSemanticObjects: 250_000,
        MaxRegionWork:      8 << 20,
    },
})
if doc != nil {
    defer doc.Close()
}
if err != nil {
    return err
}
detail, err := gopd.BuildPDF(doc)
if err != nil {
    // detail may contain a partial result; never treat it as complete.
    return err
}
_ = detail
```

Zero fields select defaults. Negative fields are rejected before opening or reading input.
The examples are application choices, not universal recommended capacities.

The same limits are available directly on selective extraction:

```go
result, err := gopd.ParseReader(readerAt, size, gopd.ParseOptions{
    Content: gopd.ContentText | gopd.ContentImages,
    ReadOptions: gopd.ReadOptions{
        MaxFileBytes: 32 << 20,
        Limits: gopd.Limits{
            MaxTokenBytes:      1 << 20,
            MaxDecodedBytes:    64 << 20,
            MaxContentBytes:    128 << 20,
            MaxSemanticObjects: 250_000,
        },
    },
})
if err != nil {
    // result may be partial; always check the error.
    return err
}
_ = result
```

| Limit | Default | Accounting scope |
| --- | ---: | --- |
| `MaxFileBytes` | 256 MiB | Original file size, independent of input buffer capacity |
| `BufferSize` | 64 KiB | Shared original-input buffer capacity; decoded sources and results are accounted separately |
| `MaxDepth` | 256 | Direct syntax/reference loading; semantic traversal also has its existing 128-level ceiling and Forms a 64-level ceiling |
| `MaxTokenBytes` | 16 MiB | Each direct-object syntax token and each token scanned by the selective content cursor; standalone Lex, preliminary detailed-content tokenization, and CMap tokenization retain the fixed 16 MiB ceiling |
| `MaxObjects` | 1,000,000 | Indirect objects, cumulative xref records/ranges, and existing semantic category limits |
| `MaxXRefSections` | 256 | Xref table/stream sections including hybrid sections |
| `MaxDecodedBytes` | 256 MiB | Retained decoded stream bytes across the Document, including nested dependencies |
| `MaxValues` | 1,048,576 | Cumulative direct values/containers across document object parsing; separately, content operands, expanded numeric resources and retained style components per semantic interpretation |
| `MaxContentBytes` | 256 MiB | Decoded content bytes examined per semantic interpretation, charging each execution of reused streams |
| `MaxSemanticObjects` | `MaxObjects` | Page-node visits, content-stream visits, operators, annotations, resource dictionary entries examined and emitted diagnostics per semantic interpretation |
| `MaxRegionWork` | 16,777,216 | Region entries searched, replaced, moved or copied while maintaining the file partition |

Dictionary keys are not counted as separate `MaxValues` values. Each dictionary
entry's value is counted. Cached indirect-object loads do not charge values
again; unsuccessful parsing attempts do charge work already completed.

Semantic interpretation means one `BuildPDF` call or one `ParseFile`/`ParseReader`
call. The value budget charges numeric arrays expanded from resources on each
use. It also charges dash entries and stroke/fill color components for each
retained text, graphic or image occurrence, even when detailed results share
slices. This bounds the style copies made by the basic-result conversion.
Resource dictionary work is charged before validation or lookup on each use,
including cached XObjects. The count bounds repeated scans, not every comparison
made by a lookup.

CMap mapping keys and UTF-8 destinations share a separate semantic byte budget
with emitted Unicode text and diagnostic code/message bytes. That budget is the
smaller of `MaxDecodedBytes` and 256 MiB, per semantic interpretation. A reused CMap is charged
once. Entry limits and byte limits
are checked during expansion, before each mapping is retained. This does not
include Go map overhead or the original decoded CMap source.

These accounting scopes are stricter than counting only operands and operators.
A document accepted under an earlier version's limits may now return `ErrLimit`.
The Detail presets replace the independent detail flags and can request more
work than an old custom combination; see the [migration notes](detail-levels.md#migration).
Diagnostics also
consume resources: exhausting their count or byte budget returns `ErrLimit`
instead of silently dropping warnings or returning an apparently complete result.
An exhausted decoded-stream byte budget still permits streams whose decoded
output is empty.

Use `errors.Is(err, gopd.ErrLimit)` to distinguish resource exhaustion from
ordinary malformed or unsupported input. Invalid option values are configuration
errors. Unsupported font filters may produce diagnostics; resource exhaustion
must return an error rather than silently becoming an unsupported-font warning.

## Selective work accounting

Content kinds share one interpreter. Scanned operators and parsed operands count
toward their limits even if their output category is disabled. Reused Forms are
charged on every execution under the caller's state; cached decoding does not
make repeated interpretation free. A Contents array is one logical sequence for
the cursor, including operands split between streams.

Unrequested interpretation is omitted before creating its results. Basic text
skips font widths, descriptors, and glyph coordinates. Basic skips color and
line-style processing; coordinate transforms are evaluated when positions,
graphics geometry, or style interpretation need them. Layout adds positions and
styles but does not retain glyph arrays or source provenance. Full also retains
glyphs and resolves embedded font references for inspection; it does not execute
the programs or render pixels. The parser still resolves XObject kinds and
enters Forms to find requested content at every level.

Annotation-only extraction skips page Contents and Resources entirely. It still
charges page traversal, geometry, annotations, and other work it performs.
Full records operations only for scanned content; it does not turn annotation-only
extraction into a content scan. Scanned operations may belong to unselected kinds,
but neither result view emits their classified elements.

These choices do not validate skipped resources or every operator's meaning.
A successful extraction can coexist with malformed unrequested resources.
Completeness and diagnostics describe the requested interpretation. Syntax errors
in scanned data, invalid required references, and limits on performed work still
return errors. Existing basic and detailed APIs keep their prior processing and
result data, with the input lifetime changes described above.

## Source provenance

One content stream retains its decoded source identity. A Contents array is
interpreted as a logical concatenation, so arrays such as a TJ operand may span
stream boundaries. A joined source has `Origin.Inputs` listing the decoded spans
in order; its offsets address that concatenation. No separator bytes are added.
`Origin.Input` remains the single input span used by filter transformations.
Follow each input source's derivation to reach the original file.

## Interpretation policy

- Null-valued optional dictionary entries behave as absent entries after indirect
  reference resolution. Raw dictionary occurrences remain available for inspection.
  Ordinary stream predictor parameters are resolved before decoding and before
  reserving the remaining decoded-byte budget. Xref bootstrap fields and xref
  stream predictor parameters retain their direct-object restrictions.
  Duplicate entries remain errors.
- Undefined indirect references (including free objects and unmatched generations)
  resolve to null under ISO 32000-1:2008, 7.3.9–7.3.10. Physical `Document.Load`
  remains strict. Malformed in-use objects, cycles and resource limits still
  return errors; they are not converted to null.
- `ResolveObject` accepts reference chains with exactly `MaxDepth` hops to a
  direct value; the next hop exceeds the limit. Cycles remain errors.
- Rectangles normalize their two opposite corners; original operands remain in
  the source model.
- Optional-content visibility on Form/Image XObjects is not evaluated. Content is
  retained with a diagnostic and incomplete interpretation flags.
- Inline images locate their payload by `/L`, by the size of an unfiltered
  image, or by searching for `EI` followed by clean syntax. The search examines
  at most 512 bytes after each candidate and rejects at most 64 candidates.
- Encrypted content and unsupported filters/effects remain limited;
  success is not a claim of complete rendering. Check errors, diagnostics and
  completeness flags.
- Results are read-only by convention. Lazy Document methods are not concurrent-safe.

Object-stream headers are cached and individual members are read by range.
File-region updates use binary search and in-place splices. Reverse insertion
still moves a sorted slice's suffix; `MaxRegionWork` bounds that worst-case work.
