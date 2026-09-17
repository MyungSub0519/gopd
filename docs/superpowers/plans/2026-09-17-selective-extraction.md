# Selective Extraction Implementation Plan

> **For agentic workers:** Use superpowers:executing-plans for the shared interpreter work; delegate the independent syntax cursor under the Go refactoring skill. Steps use checkbox syntax for tracking.

**Goal:** Extract selected PDF content directly without retaining a full detailed snapshot.

**Architecture:** Share the existing document loader and content execution state. Add a compact output target and gate allocations/resource interpretation by requirements. Use a sequential syntax cursor for selective extraction.

**Tech Stack:** Go 1.25, standard library only.

**Spec:** `docs/superpowers/specs/2026-09-17-selective-extraction-design.md`

## Global Constraints

- Preserve all existing entry points and `PDF.Details()` behavior.
- Go 1.25; no external dependencies.
- Never disable traversal, syntax, recursion or decoding resource limits.
- No pixel rendering, OCR or reading-order reconstruction.

## Task 1: Sequential content cursor

Files: `internal/syntax/content.go`, `internal/syntax/content_test.go`.

Interface: `NewContentScanner(data []byte, source pdfmodel.SourceID, offset int64, limits pdfmodel.Limits) (*ContentScanner, error)`; `Next(remainingValues int) (pdfmodel.Object, bool, int, error)`, where bool marks an operator keyword, `Object.Value` holds `pdfmodel.Name` for operators, and `io.EOF` terminates. All source spans remain absolute.

- [x] Write failing tests for mixed operands/operators, nested values, budgets, trailing malformed input, spans and EOF.
- [x] Run `go test ./internal/syntax -run ContentScanner` and confirm missing implementation.
- [x] Implement the cursor using the existing syntax scanner/object parser, without whole-input tokenization or parsing each operand twice.
- [x] Run `go test ./internal/syntax`.

## Task 2: Selective output and common interpreter

Files: `extract.go`, `extract_model.go`, `extract_output.go`, `extract_test.go`; existing `api.go`, `semantic.go`, `content.go`, `content_state.go`, `text.go`, `fonts.go`, `pages.go`, `content_resources.go`, `semantic_budget.go`.

Interfaces: public Extract/ExtractReader and options/result contracts in the spec. `semanticBuilder` uses a nil extraction target for unchanged detailed behavior. Requirement helpers drive content, style, position, glyph and provenance work.

- [x] Write failing tests: extract text from `testdata/synthetic.pdf`, select all combinations, compare Unicode/geometry/styles to Open, verify omitted JSON data and option validation before ReadAt.
- [x] Run `go test . -run Extract` to observe missing API.
- [x] Add compact types and validate options; share BuildPDF setup through an internal builder entry.
- [x] Gate result construction at showText, paint, image and annotation handlers. Only provenance records operations and ElementSource. Preserve required Form/state dependencies.
- [x] Integrate the cursor for extraction; preserve legacy lexer behavior for detailed APIs.
- [x] Add tests for skipped font/image resources, nested Forms, split Contents operands, limits, partial errors and source access. Run `go test ./...`.

## Task 3: Documentation, measurements and review

Files: `docs/selective-extraction.md`, `docs/basic-pdf.md`, `docs/resource-limits.md`, `README.md`, `README.ko.md`, extraction examples and benchmarks.

- [x] Add runnable ExampleExtract and benchmarks comparing ParsePDF, text extraction and all-content extraction.
- [x] Document defaults, optional fields, omitted resources, errors, provenance ownership and peak-memory limits.
- [x] Run `go build ./...`, `go vet ./...`, `go test ./...`, focused extraction benchmarks with `-benchmem`.
- [x] Review the diff for retained detailed objects, disabled resource work, aliasing and budget bypasses; fix findings and rerun affected checks.
