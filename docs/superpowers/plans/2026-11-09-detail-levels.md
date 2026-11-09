# Detail levels implementation plan

> Execute inline using superpowers:executing-plans and test-driven-development.

**Goal:** Offer Content + Detail through gopd's two common parse entry points.
**Architecture:** Fixed DetailLevel presets gate the shared interpreter. Full
retains its selected DetailedPDF alongside Result without interpreting twice.
**Tech stack:** Go 1.25+, standard library only.
**Spec:** [Detail levels](../../detail-levels.md), implementing the approved chat design.

## Constraints and review focus

- Preserve PDF semantics, source provenance, ErrLimit and current package boundaries.
- Preserve existing uncommitted work; no commits or worktree moves in this task.
- Content selection remains independent of detail, including annotations-only.
- Partial Full results must expose matching operations, indexes and completeness.
- Validate Content/Detail before opening or reading input; keep the existing
  ReadOptions normalization, reader ownership and one-snapshot semantics.
- Both result views share one interpretation and existing resource accounting.
- Retain legacy entry points; document the bool-option migration explicitly.

## Tasks

1. Add public tests for all levels through both entry points, invalid options,
   Full selected details, JSON, partial errors, source spans and resource limits.
   Run `go test ./... -count=1` and observe missing-API failures before implementation.
2. Add DetailLevel/Detail, replace bool options, retain Details at Full, and
   adapt existing selective tests to fixed levels. Run the complete test suite.
3. Migrate CLI, examples, comments and both READMEs; update resource-limit docs
   for Full retention and migration. Verify CLI behavior through existing tests.
4. Run test/vet/lint, CGO-disabled 386 tests, selected fuzz targets and
   govulncheck. Race testing requires a C compiler; report inability if absent.
   Obtain an independent final review and address concrete findings.

## Execution record

- Baseline: `go test ./... -count=1` passed before edits.
- User authorized implementing the discussed design; no additional approval gate.
- Tasks 1-3 complete: fixed levels, selected Full details, CLI, examples and
  documentation. Public tests first failed on the missing new API, then passed.
- Task 4 complete: independent core and final documentation reviews found no
  remaining actionable issues; strengthened partial output/source index checks.
- `go test ./... -count=1`, `go vet ./...`, CGO-disabled GOARCH=386 tests,
  golangci-lint v2.13.0 (0 issues), govulncheck v1.8.0 (no vulnerabilities),
  gopls diagnostics, formatting, diff whitespace and local link checks passed.
- FuzzExtractReader: 30 seconds, 76,881 executions, passed. FuzzBuildPDF:
  15 seconds, 52,941 executions, passed.
- Lint/vulnerability tools used process-local GOTOOLCHAIN=auto (Go 1.26.10);
  go.mod remains unchanged at Go 1.25. Tests used the installed Go 1.25.4.
- Race testing attempted with CGO_ENABLED=1; unavailable because gcc is absent.
