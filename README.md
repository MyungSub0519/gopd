![GoPD](docs/readme/GoPD-logo-withtext.png)

**A powerful PDF parser written in native Go.**

English | [한국어](README.ko.md)

## Motivation

I struggled to find an open-source PDF parser as powerful as MuPDF in the Go ecosystem. Many projects relied on importing DLLs written in other languages, and I did not find that approach elegant.

There were also native Go projects used commercially, but I was not happy with that either.

This project draws on MuPDF as a reference.

## Extract selected content

Use `Extract` to generate only the content you need. Zero options return Unicode
text and compact font metadata, grouped by page.

```go
package main

import (
    "fmt"
    "log"

    "github.com/MyungSub0519/gopd"
)

func main() {
    result, err := gopd.Extract("testdata/synthetic.pdf", gopd.ExtractOptions{})
    if err != nil {
        log.Fatal(err)
    }
    for _, page := range result.Pages {
        for _, text := range page.Texts {
            fmt.Println(page.Index+1, text.Unicode)
        }
    }
}
```

Combine `ContentText`, `ContentGraphics`, `ContentImages`, and
`ContentAnnotations` with `|`, or select `ContentAll`. `Positions`, `Styles`,
`Glyphs`, and `Provenance` control optional details; `Glyphs` requires text and
enables positions. `ExtractReader` accepts an `io.ReaderAt` and input size.

Selected kinds share the content interpreter. Unrequested results are not
constructed, and the result retains the input snapshot only with `Provenance`.
Input and decoded caches still occupy memory during the call; this is not
streaming file I/O or full validation of skipped content.

Existing `ParsePDF` and detailed APIs remain available. `ParsePDF` still keeps
its detailed result for `Details()`. See [selective extraction](docs/selective-extraction.md)
and the [basic API guide](docs/basic-pdf.md) for options, ownership, and examples.

## Supported Features

GoPD aims to provide detailed interpretation of PDF content and the internal structures that define it.

- **Text** — Analyze not only strings but also individual character codes, Unicode mappings, fonts, sizes, positions, and transformations.
- **Vector graphics** — Analyze paths made of lines and Bézier curves, strokes and fills, colors, line widths, dash patterns, and clipping information.
- **Images** — Analyze original image streams, pixel dimensions, color spaces, image mask status, and placement positions, sizes, and rotations on the page.
- **Content execution structure** — Track the drawing order of text, graphics, and images, state changes, and nested Form XObject calls.
- **Internal file structure** — Analyze PDF objects and indirect references, compressed object streams, xref tables and streams, and the chain of incremental updates.
- **Source traceability** — Trace interpreted elements back to the commands and objects that produced them, down to byte ranges in the original or decoded data.

These goals are partially implemented. Interpretation of complex color spaces, transparency effects, inline images, and encrypted content remains limited.

See [resource limits and interpretation](docs/resource-limits.md),
[contributing](CONTRIBUTING.md), and the [release checklist](docs/release-checklist.md)
for the current processing contract and remaining release work.

## Project structure

The root package exposes the public API. Content interpretation, extraction, and
result models live in `internal/parser`. Shared PDF models, syntax, document
reading, and test fixtures live in subpackages of `internal/common`. See the
[project structure guide](docs/project-structure.md) for file roles and dependencies.
