![GoPD](docs/readme/GoPD-logo-withtext.png)

**A powerful PDF parser written in native Go.**

English | [한국어](README.ko.md)

## Motivation

I struggled to find an open-source PDF parser as powerful as MuPDF in the Go ecosystem. Many projects relied on importing DLLs written in other languages, and I did not find that approach elegant.

There were also native Go projects used commercially, but I was not happy with that either.

This project draws on MuPDF as a reference.

## Supported Features

GoPD aims to provide detailed interpretation of PDF content and the internal structures that define it.

- **Text** — Analyze not only strings but also individual character codes, Unicode mappings, fonts, sizes, positions, and transformations.
- **Vector graphics** — Analyze paths made of lines and Bézier curves, strokes and fills, colors, line widths, dash patterns, and clipping information.
- **Images** — Analyze original image streams, pixel dimensions, color spaces, image mask status, and placement positions, sizes, and rotations on the page.
- **Content execution structure** — Track the drawing order of text, graphics, and images, state changes, and nested Form XObject calls.
- **Internal file structure** — Analyze PDF objects and indirect references, compressed object streams, xref tables and streams, and the chain of incremental updates.
- **Source traceability** — Trace interpreted elements back to the commands and objects that produced them, down to byte ranges in the original or decoded data.

These goals are partially implemented. Interpretation of complex color spaces, transparency effects, inline images, and encrypted content remains limited.
