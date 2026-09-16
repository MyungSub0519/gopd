# Internal PDF Byte Reader Implementation Plan

**Goal:** Move byte reading, document loading and syntax analysis into internal domain packages while retaining the public facade.

**Architecture:** `gopd → document → syntax → pdfmodel`, with direct `gopd → syntax` calls for content operands. Document state and methods move together; only type aliases remain at the public boundary.

**Spec:** [Byte reader design](../specs/2026-09-16-byte-reader-design.md)

## Tasks

- [x] Extract `lexer.go`, `parser.go`, syntax API implementations and `syntax_test.go` into `internal/syntax`; qualify shared model names and export only `ParseObjectWithLimits` for internal consumers.
- [x] Move `document.go`, `document_objects.go`, `document_xref.go`, `filters.go`, `read_options.go` and low-level parse entry implementations into `internal/document`.
- [x] Move document/filter tests and their parsing helpers into that package, retaining all test/fuzz names and assertions.
- [x] Preserve root public functions as forwarding wrappers and Document/ReadOptions as aliases; update content operand parsing to the internal syntax API.
- [x] Add focused input-boundary characterization tests for ReaderAt errors, copying, size validation and partial results, and run them before/after extraction.
- [x] Update the structure guide and source links in current API/JSON docs.
- [x] Verify test inventory, full tests, build, vet, formatting and independent review.

## Constraints

No parsing or decoding algorithm changes, external dependencies, commits or changes to user-owned edits. Keep the current snapshot memory model and error/limit semantics. Root APIs remain callable at the existing import path.

## Verification result

- `go test -count=1 ./...`, `go build ./...`, and `go vet ./...` passed.
- Existing tests and all three fuzz targets are preserved. Four input-boundary tests passed before and after extraction.
- Formatting, diff checks and current documentation links are clean.
- Independent review confirmed moved logic and assertions, public forwarding behavior and dependency direction, with no concrete regressions found.
