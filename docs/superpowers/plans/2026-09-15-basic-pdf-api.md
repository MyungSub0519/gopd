# Basic PDF API Implementation Plan

> **For agentic workers:** Execute tasks in order, with failing tests before implementation and an independent review before completion.

**Goal:** Return a basic `*pdf.PDF` through a single `pdfparse(path)` call in the CLI while retaining detailed analysis.

**Architecture:** Keep the existing detailed interpreter and project its result into a compact public model. Retain the detailed result privately and expose it through `Details()`; use shared basic font/image descriptors.

**Tech Stack:** Go 1.25+, standard library, PowerShell workspace.

**Spec:** [기본 PDF 반환 API](../specs/2026-09-15-basic-pdf-api-design.md)

## Global Constraints

- Go 1.25 이상, 표준 라이브러리만 사용한다.
- 기존 PDF 및 상세 JSON 파일을 변경하지 않는다.
- 기존 `Open`, `Read`, `BuildPDF`의 상세 분석 동작을 유지한다.
- 새로운 선택적 파싱이나 렌더링 기능을 추가하지 않는다.
- 처음에는 Git 저장소가 아니었으나 사용자의 후속 게시 요청으로 빈 원격 저장소를 확인하고 `main`을 초기화했다. 검증 후 초기 커밋을 `origin/main`에 푸시한다.

## Task 1: Basic result and detailed model separation

Files: create `basic_model.go`, `basic.go`, `basic_test.go`; modify `model.go`, `read.go`, `content.go`, `content_state.go`, `content_test.go`.

Interfaces:

```go
func ParsePDF(path string) (*PDF, error)
func (p *PDF) Details() *DetailedPDF
func basicPDF(detail *DetailedPDF) *PDF
```

- [x] Write tests using real synthetic PDF bytes and the supplied fixture; assert `Texts[0].Font == Texts[1].Font` for a shared resource and inequality for distinct same-name resources. Assert transformed path coordinates and image matrices match expected values.
- [x] Run `go test ./...`; observe the new basic API is missing before production edits.
- [x] Rename the detailed model's six colliding types, preserve detailed operations, and implement the basic result projection. Capture semantic diagnostic page indexes at emission.
- [x] Verify basic JSON contains exactly the five public root arrays and no `RawCodes`, `Glyphs`, `Operations`, `Span`, `ToUnicode`, `Stream`, or `Document` keys. Check empty results serialize as arrays and basic slice edits do not mutate detailed paths.
- [x] Check file errors, malformed PDFs, partial semantic failures, and repeated-Form diagnostic page attribution.

## Task 2: Single-function CLI entry

Files: modify `cmd/gopd/main.go`, `cmd/gopd/main_test.go`.

```go
func pdfparse(path string) (*pdf.PDF, error) {
    return pdf.ParsePDF(path)
}
```

- [x] Add CLI tests calling `pdfparse` directly on the fixture and a missing path.
- [x] Use `pdfparse` in `run`; obtain advanced summary counts via `doc.Details()`.
- [x] Run `go test ./cmd/gopd` and execute both existing CLI output modes.

## Task 3: Documentation and verification

Files: modify `README.md`, `docs/json-structure.md`; create `docs/basic-pdf.md`.

- [x] Document the concrete one-call example, basic fields, detailed entry points, error handling, and eager retention limits.
- [x] Clarify that the earlier saved JSON documents the detailed snapshot, which is unchanged.
- [x] Run `gofmt`, `go test ./...`, and `go vet ./...`.
- [x] Review the public/detailed boundary, resource identity, diagnostics, and error propagation independently; resolve material findings.
- [x] Record verification results and mark this plan complete.

## Verification record

- `gofmt -l`: all Go files clean.
- `go test ./... -count=1`: both packages passed, including the user fixture.
- `go vet ./...`: passed.
- CLI `-json`: 3 pages, 104 texts, 2,639 graphics, 1 image/resource, 1 font, 8 diagnostics.
- Independent review: no remaining findings after a regression test and fix for detailed byte locations appearing in basic ToUnicode diagnostic messages.
- Documentation: UTF-8, relative links, and JSON examples verified.
- Original PDF and detailed JSON hashes unchanged; both remain local ignored files.