# Common PDF Domains Implementation Plan

> **For agentic workers:** Use the Go refactoring workflow for the bounded common-model extraction, and request an independent review before completion.

**Goal:** Group shared PDF primitives and test fixtures by domain while preserving the root library API.

**Architecture:** Root `gopd` exposes aliases and forwarding functions. Internal packages own the low-level PDF model, geometry, and parser-independent test fixtures. Engine-specific state stays in the root for this first pass.

**Tech Stack:** Go 1.25, standard library, gopls.

**Spec:** [Common domain design](../specs/2026-09-16-common-domain-design.md)

## Constraints

- Preserve existing public imports, names, methods and JSON output; document reflection package identity changes.
- Keep sealed interfaces and all their implementations together.
- Add no external dependencies; preserve existing user changes.
- Keep unit tests with implementation and public API tests in `gopd_test`.

## Task 1: Extract common models and geometry

- [x] Move `source_types.go`, `object_types.go`, `stream_types.go`, `indirect_types.go`, `xref_types.go`, `token_types.go`, `structure_types.go` and `values.go` implementations to `internal/pdfmodel`.
- [x] Consolidate root type/constant aliases in `model_types.go`; keep `values.go` as the public forwarding API and error sentinel alias.
- [x] Move geometry implementation to `internal/geometry`; preserve root aliases and `IdentityMatrix`.
- [x] Add field names to root literals of moved structs so `go vet` accepts the new package boundary.
- [x] Check dictionary duplicate/missing keys, numeric validation and matrix composition with focused package tests; run `go test ./internal/...` and public API tests.

## Task 2: Consolidate fixtures and organize tests

- [x] Put the synthetic PDF and stream writers in `internal/pdftest`; preserve current fixture byte formatting, including optional trailer entries.
- [x] Collect root parsing setup helpers in `test_helpers_test.go`, and use the shared writer from `public_api_test.go`.
- [x] Move review regressions into `filters_test.go`, `document_objects_test.go`, `document_xref_test.go`, `fonts_test.go`, and `content_state_test.go` without changing assertions.
- [x] Compare the test inventory and run `go test -count=1 ./...`.

## Task 3: Document and verify

- [x] Add `docs/project-structure.md` with the current package tree, dependency rules, test placement, and later reader/syntax/content/font boundaries.
- [x] Update source links and layout in `docs/public-api.md` and `docs/json-structure.md`; link the structure guide from both READMEs.
- [x] Run `go build ./...`, `go vet ./...`, formatting and diff checks.
- [x] Obtain an independent code review.

## Recorded baseline

`go test -count=1 ./...` passed before edits.

## Verification result

After extraction and keyed-literal updates, `go test -count=1 ./...`, `go build ./...`, and `go vet ./...` passed. The existing test inventory is preserved after the nine regression-test renames; seven focused common-package tests were added. Formatting, diff checks, and current documentation links are clean. Independent review found no concrete regressions and confirmed the moved definitions, aliases, fixture bytes, and regression-test assertions.
