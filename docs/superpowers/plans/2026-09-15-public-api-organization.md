# Public API Organization Implementation Plan

**Goal:** List supported library features and organize their entry points and types without changing existing API behavior.

**Spec:** [공개 API 역할 정리](../specs/2026-09-15-public-api-organization-design.md)

**Constraints:** Keep the module/package name, 12 public functions, 12 methods on public types, all existing public types/fields/constants, current two-field basic JSON, error semantics, and the eager detailed parser. Work on `refactor/public-api-organization` in the current checkout.

## Task 1: Record the external contract

- [x] Add tests in `package gopd_test` using synthetic PDF bytes. Cover `ParsePDF`, `Open`, `Read`, `Parse`, `ParseFile`, `BuildPDF`, and the Document methods through real inputs.
- [x] Verify independent `Lex`, `ParseObject`, dictionary lookup, number conversion, stream identification, and matrix operations from an external package.
- [x] Run `go test ./...` to establish the contract before moving production declarations. These behavior-preserving tests must pass against the baseline.

## Task 2: Organize production declarations

- [x] Move basic/detailed/document/syntax entry points into their `api_*.go` files; keep algorithm bodies unchanged.
- [x] Split projection, basic/detailed models, geometry, style, font, and read options by responsibility.
- [x] Retain unused public model types in `compat_types.go` with Deprecated guidance. Keep field types and order unchanged.
- [x] Add missing API comments for error/partial-result/input-ownership behavior. Do not add forwarding layers or rename public functions.
- [x] Compare public declaration signatures and run the existing and external-package tests.

## Task 3: Publish the feature map and verify

- [x] Update `docs/public-api.md` with the six feature groups, recommended entry points, type ownership, compatibility types, and implementation-file map.
- [x] Correct active documentation links and add links to API documentation in both READMEs. Historical plans remain design history.
- [x] Run `gofmt`, `go test ./... -count=1`, `go vet ./...`, and Markdown link checks.
- [x] Obtain an independent code review and resolve material findings.
- [x] Record verification and report the resulting API organization.

## Verification record

- Baseline: `52a08681129b42a1c48126f0677df3fe69e03376`.
- External-package contract tests passed before and after the declaration moves.
- A temporary `go/types` audit compared 145 public API records against the baseline. All 12 functions, 12 public-type methods, 84 types, field order/types, and constant values are unchanged. All 84 types appear in the API inventory.
- Independent review compared all 136 production function/method bodies after whitespace normalization and found no behavior changes or material issues. Its three documentation findings were addressed.
- `gofmt -l` returned no files; `go test ./... -count=1`, `go vet ./...`, and `git diff --check` passed.
- Checked 64 relative links in active documentation and parsed all 4 JSON examples. The Matrix.Transform comment now appears in `go doc`.
