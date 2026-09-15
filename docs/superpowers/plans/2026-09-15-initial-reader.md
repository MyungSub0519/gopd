# GoPD Initial Reader Implementation Plan

> Agent workflow: use test-driven development for parser behavior and parallel agents for the independent lexer/object parser and page interpreter, with fixed interfaces. Root owns document/xref/filter integration and final review.

**Goal:** Implement the designed Go types and an initial pure-Go reader producing document-wide text, vector graphic, and image placement slices, with original source spans, verified using `테스트PDF.pdf`.

**Architecture:** Retain an immutable, bounded input snapshot. Parse syntax objects separately from xref resolution, stream decoding, and page content interpretation. Semantic elements retain source operations and page order. Unsupported features return diagnostics or explicit errors rather than invented results.

**Tech Stack:** Go 1.25, standard library only, package `pdf`, module `gopd`.

**Spec:** `docs/superpowers/specs/2026-09-15-pdf-structure-design.md`, including section 14.

## Global constraints and first-version decisions

- Preserve `테스트PDF.pdf` unchanged and do not upload its contents.
- This directory is not a git repository; work in place without creating a remote or committing the user PDF.
- Implement real type declarations and an executable reader; do not claim complete PDF rendering, OCR, or editable round-trip support.
- Source IDs distinguish file data and decoded data. Keep unknown or unsupported information in raw objects and diagnostics.
- First version uses a bounded immutable input snapshot in memory, defaults to 256 MiB. Stream decoding is lazy and separately bounded. ReaderAt is caller-owned and is not closed by Parse.
- File scope: ordinary xref tables, incremental Prev links, hybrid/xref streams and compressed object streams. Encryption is explicit unsupported for semantic decoding; structural inspection remains available.
- Content scope: ordered page traversal, text show operations, ToUnicode mappings including the fixture's simple TrueType font, paths, image placements, graphics state and Form traversal. Preserve operations and diagnose unsupported rendering effects.
- Fixed shared API: `ParseObject(data []byte, source SourceID, offset int64) (Object, int, error)`, `Lex(data []byte, source SourceID, offset int64) ([]Token, error)`, `Parse(r io.ReaderAt, size int64, options ...ReadOptions) (*Document, error)`, `ParseFile(path string, options ...ReadOptions) (*Document, error)`, `(*Document).Resolve(Reference) (Object, error)`, `(*Document).ResolveObject(Object) (Object, error)`, `(*Document).Load(ObjectID) (*IndirectObject, error)`, `(*Document).DecodeStream(Stream) (Source, error)`, `(*Document).Bytes(Span) ([]byte, error)`, `(*Document).Catalog() (Object, error)`.
- `Document.Structure` is an exported `Structure` value. `Document.Sources` is `map[SourceID]Source`. `Dictionary.Get(Name) (Object, error)` returns `ErrMissingKey` for missing keys and errors for duplicates. `Number(Object) (float64, error)` and `Int(Object) (int64, error)` convert validated number values.

## Task 1: syntax types and object parsing

Files: source_types.go, token_types.go, object_types.go, stream_types.go, indirect_types.go, xref_types.go, structure_types.go; lexer.go, parser.go, syntax_test.go.

- [x] Extract the reviewed declarations into real Go files and compile them.
- [x] Write tests using literal bytes for whitespace/comments, nested literal strings, octal escapes, names, odd hex digits, numeric spelling, dictionaries with duplicate keys, indirect references and malformed/truncated tokens. Example: ParseObject([]byte(`<< /A (a\101) /Ref 12 0 R >>`),1,100) must retain dictionary spans and produce bytes `aA` and Reference(12,0).
- [x] Run the syntax tests before implementation and record the expected missing API failure.
- [x] Implement bounded lexical scanning and recursive object parsing with depth and token limits. Bare content operators are TokenKeyword, not PDF object values. ParseObject stops after one object and returns the exact consumed byte count.
- [x] Run syntax tests and fuzz seeds; commit no files (no git repository).

## Task 2: document/xref/stream reader

Files: document.go, document_objects.go, document_xref.go, filters.go, values.go, document_test.go, filters_test.go.

- [x] Write generated small PDF fixtures with exact xref offsets. Check Catalog resolution, indirect stream Length, payload containing `endstream`, latest incremental object selection, free-entry shadowing, generation mismatch, xref stream type 2 and object-stream offsets.
- [x] Run targeted tests before implementing document behavior.
- [x] Implement the fixed shared API, bounded ReadOptions, ParseError/source spans, xref cycle detection and current-version caching.
- [x] Decode Flate, ASCIIHex, ASCII85 and RunLength with explicit errors for unknown filters; handle Flate PNG/TIFF predictors needed by xref/image streams. Maintain a total decoded-byte budget and cache decoded sources.
- [x] Confirm the user fixture has 3 pages and 28 xref slots, including reserved object zero, as independently inspected from its trailer and page tree.

## Task 3: semantic page model and interpretation

Files: model.go, content.go, content_state.go, fonts.go, cmap.go, read.go, content_test.go, cmap_test.go.

- [x] Add PDF with Pages, Texts, Graphics, Images, Fonts, Annotations, Structure and source Document access. Preserve per-page typed element references in content execution order.
- [x] Write synthetic tests showing mixed path/text/image order, repeated image placements, graphics-state restore, Form reuse/cycles, page inheritance, text positioning and ToUnicode mappings. For ToUnicode, a hand-authored mapping `<01> <AC00>` must extract `가`.
- [x] Run failing targeted tests before implementing the relevant behavior.
- [x] Implement `BuildPDF(*Document) (*PDF,error)`, `Read(io.ReaderAt,int64) (*PDF,error)`, `Open(string) (*PDF,error)` using the shared document API. Parse content operators through Lex/ParseObject, tracking source provenance and graphics/text state. Keep unsupported operators/effects diagnosable.
- [x] Run synthetic content and CMap tests plus the real fixture. Use independent literal Korean text from its decoded ToUnicode/content streams for an integration assertion.

## Task 4: CLI, documentation, integration and review

Files: cmd/gopd/main.go, README.md, integration_test.go.

- [x] Provide `go run ./cmd/gopd "테스트PDF.pdf"` with page and element counts and sample extracted text. Add a text output option without serializing source readers or font binaries.
- [x] Document public API, immutable snapshot/ownership, source provenance, supported features and material extraction limits. Generated semantic fields are an initial model, not a fidelity guarantee.
- [x] Run `gofmt`, `go test ./...`, `go vet ./...`, bounded fuzzing of the lexer/object parser and the CLI on the user fixture.
- [x] Review API and parser boundaries for unbounded allocation, silent loss of unsupported features, reference cycles, wrong byte spans, and incorrect Unicode claims. Fix found regressions with tests.
- [x] Record actual fixture results and only claim the checks whose final outputs pass.

## Verification record

- Syntax lexer/object parser tests passed; each fuzz target ran 15 seconds with two workers (1,533,669 object parses and 1,679,533 lexical inputs).
- Document/xref/filter unit tests and reader review regressions passed. Document fuzzing ran 15 seconds / 1,583,330 executions without failure.
- User fixture integration and CLI: 3 pages, 104 text show elements, 2,639 vector graphics, 1 image placement/resource, 1 font, 8 unsupported-effect diagnostics. Source spans, per-page references and unchanged fixture bytes verified.
- Reader review: limit overflow, ASCII85 tight-budget handling, strict stream boundaries, cumulative xref budgets fixed and independently re-reviewed.
- Semantic review: WinAnsi aliases, finite derived geometry, cumulative clip snapshots and CID-width expansion fixed, covered by targeted regressions and independently re-reviewed with no unresolved findings in scope. Unicode-output and expanded-font-map budgets also verified.
- Final formatting check: clean. Final `go test ./...`: root and CLI packages pass. Final `go vet ./...`: exit 0. Final CLI run: expected fixture counts and 8 diagnostics.
- Original fixture SHA256 remains `45D18ED4CF3F6B6A5F3415320B0E7D428FFE414343A87CB7991E88486C1B40B9`.
