# Resource limits and interpretation

GoPD snapshots the input and keeps parsed objects and decoded sources in memory.
`ParsePDF` also retains its detailed result. Limits bound specific resources;
they are not an exact process-memory limit. Object, token and map overhead,
temporary decoding buffers and the input snapshot consume additional memory.

Use the low-level entry point to set limits before interpreting content:

```go
doc, err := gopd.Parse(readerAt, size, gopd.ReadOptions{
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

Zero fields select defaults. Negative fields are rejected before reading input.
The examples are application choices, not universal recommended capacities.

| Limit | Default | Accounting scope |
| --- | ---: | --- |
| `MaxFileBytes` | 256 MiB | Original snapshot |
| `MaxDepth` | 256 | Direct syntax/reference loading; semantic traversal also has its existing 128-level ceiling and Forms a 64-level ceiling |
| `MaxTokenBytes` | 16 MiB | Each direct-object syntax token; standalone Lex and preliminary content/CMap tokenization use the fixed 16 MiB ceiling |
| `MaxObjects` | 1,000,000 | Indirect objects, cumulative xref records/ranges, and existing semantic category limits |
| `MaxXRefSections` | 256 | Xref table/stream sections including hybrid sections |
| `MaxDecodedBytes` | 256 MiB | Retained decoded stream bytes across the Document, including nested dependencies |
| `MaxValues` | 1,048,576 | Cumulative direct values/containers across document object parsing; separately, content operand values per BuildPDF |
| `MaxContentBytes` | 256 MiB | Decoded content bytes examined per BuildPDF, charging each execution of reused streams |
| `MaxSemanticObjects` | `MaxObjects` | Page-node visits, content-stream visits, operators and annotation occurrences per BuildPDF |
| `MaxRegionWork` | 16,777,216 | Region entries searched, replaced, moved or copied while maintaining the file partition |

Dictionary keys are not counted as separate `MaxValues` values. Each dictionary
entry's value is counted. Cached indirect-object loads do not charge values
again; unsuccessful parsing attempts do charge work already completed.

CMap mapping keys and UTF-8 destinations share a separate semantic byte budget
with emitted Unicode text. That budget is the smaller of `MaxDecodedBytes` and
256 MiB, per BuildPDF. A reused CMap is charged once. Entry limits and byte limits
are checked during expansion, before each mapping is retained. This does not
include Go map overhead or the original decoded CMap source.

Use `errors.Is(err, gopd.ErrLimit)` to distinguish resource exhaustion from
ordinary malformed or unsupported input. Invalid option values are configuration
errors. Unsupported font filters may produce diagnostics; resource exhaustion
must return an error rather than silently becoming an unsupported-font warning.

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
- Rectangles normalize their two opposite corners; original operands remain in
  the source model.
- Optional-content visibility on Form/Image XObjects is not evaluated. Content is
  retained with a diagnostic and incomplete interpretation flags.
- Encrypted content, inline images and unsupported filters/effects remain limited;
  success is not a claim of complete rendering. Check errors, diagnostics and
  completeness flags.
- Results are read-only by convention. Lazy Document methods are not concurrent-safe.

Object-stream headers are cached and individual members are read by range.
File-region updates use binary search and in-place splices. Reverse insertion
still moves a sorted slice's suffix; `MaxRegionWork` bounds that worst-case work.
