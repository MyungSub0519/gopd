# Page-grouped Basic PDF Implementation Plan

**Goal:** Return only page-grouped Texts and Graphics from the basic PDF API.

**Architecture:** Preserve detailed parsing and change its basic projection. The CLI uses the detailed result for its existing summary and text output.

**Tech Stack:** Go 1.25+, standard library only.

**Spec:** [Page-grouped basic PDF design](../specs/2026-09-15-page-grouped-basic-pdf-design.md)

## Global Constraints

- Preserve `ParsePDF(path) (*PDF, error)` and `Details() *DetailedPDF`.
- Keep detailed parsing, shared fonts, slice isolation and partial error results.
- Preserve empty page slots as JSON arrays.
- Update the existing workspace without committing unrelated local changes.

## Task 1: Basic response and its consumers

- [x] Add a synthetic PDF regression covering text/graphics on pages 1 and 3 with an empty page 2. Verify the two-key JSON shape, literal text order, page counts and empty arrays.
- [x] Run `go test -count=1 -run '^TestBasicContentGroupedByPage$' .` and confirm failure against the existing five-field response.
- [x] Change `PDF.Texts` to `[][]Text` and `PDF.Graphics` to `[][]Graphic`; remove Pages, Images and Diagnostics from PDF. Allocate non-nil slices for all detailed pages and append converted elements by source page.
- [x] Update `basic_test.go` and `cmd/gopd/main_test.go` for nested arrays. Preserve assertions for shared font identity, coordinates, detailed resources, diagnostics, slice independence and errors.
- [x] Update `cmd/gopd/main.go` to obtain the existing page order, counts, resources and diagnostics from the detailed result.
- [x] Run `go test -count=1 ./...`, `go build ./...` and `go vet ./...`.

## Task 2: Documentation and inspectable sample

- [x] Update `docs/basic-pdf.md`, `docs/public-api.md` and `docs/json-structure.md` for page-grouped usage and detail access; link the previous basic API design to the current design. README was independently rewritten during this task and its new content is preserved.
- [x] Regenerate `테스트PDF.basic.json` with `json.Encoder` from `ParsePDF` and verify its two root keys and per-page counts.
- [x] Run the sample CLI in JSON and text modes; verify existing output counts and extracted text count.
- [x] Check documentation references and `git diff --check`; review the final diff for unrelated changes.

## Verification record

- The new regression failed before implementation: expected two basic fields, received five.
- `go test -count=1 ./...`, `go build ./...` and `go vet ./...` passed after implementation.
- gopls reported no diagnostics in the changed Go files.
- The regenerated basic JSON has exactly Texts and Graphics, with per-page counts 58/39/7 and 1052/1571/16.
- CLI JSON matches the saved inspection summary. Text output contains 104 non-empty lines and two page separators.
- Both runnable documentation examples passed; 24 public signatures match `go doc` and 36 local documentation links resolve.
- Independent code review found one diagnostic help-text issue: the count includes both diagnostic collections. The message now points to `PDF.Details()` for all detailed information.
