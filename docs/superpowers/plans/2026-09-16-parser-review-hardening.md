# Parser review hardening implementation plan

> Execution: apply Superpowers TDD and independent task review. The user approved the preceding review's concrete fixes and requested implementation.

**Goal:** Fix the twelve reproduced correctness/resource/performance findings without replacing existing public entry points or snapshot ownership contracts.

**Architecture:** Keep syntax, document and semantic layers. Enforce explicit budgets before expansion, cache object-stream indexes, retain exact source provenance for logically joined content, and preserve unsupported content with honest diagnostics.

**Constraints:** Native Go standard library, Go 1.25, Windows/Linux amd64 and 386 tests; existing result schemas remain usable. No automatic license selection or broad public-type redesign.

## Task 1: Document decoding and object access
- [x] Add failing regressions for recursive filter dependency accounting, optional null filter/external-file entries, and repeated object-stream reads.
- [x] Resolve filter dependencies before taking the decode budget; validate current budget before registering output.
- [x] Cache validated object-stream headers and use selected ranges rather than full-container copies.
- [x] Replace per-load full region reconstruction with efficient interval updates while preserving an immediately inspectable file partition.
- [x] Run `go test ./internal/document -count=1`; retain a size-scaled benchmark for object loading/regions.

## Task 2: Fonts and CMaps
- [x] Add failing tests for CMap destination-byte expansion, Differences-only Helvetica, indirect CID width components and bytewise codespaces.
- [x] Enforce remaining mapping count/byte budgets during expansion; resource-limit failures must propagate, not become unsupported-font diagnostics.
- [x] Initialize implicit base encoding before Differences; resolve nested CID width objects; share bytewise codespace checks.
- [x] Run focused font tests and add a bounded CMap fuzz target.

## Task 3: Content interpretation
- [x] Add failing tests for repeated whitespace streams, annotation occurrences, split TJ arrays, optional null page values, reversed rectangles and XObject optional content.
- [x] Bound stream visits, bytes examined, direct values and output occurrences before work/allocation; keep existing MaxObjects semantics documented.
- [x] Parse Contents arrays as one logical token sequence, with registered source derivation for cross-source objects and bounded copying.
- [x] Normalize rectangle corners; treat resolved null as absent; diagnose unsupported XObject visibility and mark affected results incomplete.
- [x] Run focused content tests and add bounded semantic fuzzing with a valid PDF container.

## Task 4: Shared limits, documentation and CI
- [x] Add independent zero-default limits for direct syntax values, content bytes and semantic output where needed, with negative-value rejection and boundary tests.
- [x] Document defaults, total vs per-object limits, source concatenation and supported behavior.
- [x] Add CI for tests/vet, race on Linux, 386, and bounded fuzz smoke runs; add contributor/security reporting guidance without assigning a license.
- [x] Record deferred API ownership, selective/lazy I/O, context and license decisions as a release checklist.

## Verification
- [x] Confirm each regression fails before its fix and passes afterward.
- [x] Review the complete diff for limit/accounting and provenance interactions.
- [x] Run `go test ./... -count=1`, `go vet ./...`, and `GOARCH=386 go test ./... -count=1`.
- [x] Run all affected fuzz targets for bounded smoke durations with two workers.
- [x] Record CGO/race limitations honestly; leave no temporary probes or build artifacts.

## Results (2026-09-16)

- Regression cases were observed failing before their fixes, including follow-up
  classification tests for font and xref limits. Independent review found no
  further issues in document/content changes.
- Go 1.25.4: full shuffled tests and vet passed; all packages also passed on 386.
  Coverage: root 74.9%, document 76.2%, syntax 95.8%, model 97.2%, CLI 58.3%.
- Six fuzz targets ran for 15 seconds each with two workers: Lex 631,698,
  ParseObject 446,480, Document 577,101, DecodeFilters 339,099,
  ToUnicodeBounded 346,922 and BuildPDF 116,637 executions; no failures.
- golangci-lint v2.13.0 reported zero issues. actionlint v1.7.7 accepted the CI
  workflow. govulncheck v1.8.0 reported no vulnerabilities with Go 1.26.8.
  The newer toolchain was downloaded for these development tools; go.mod
  remains Go 1.25.0 and the library has no new dependencies.
- Local CGO is disabled, so race testing remains unverified locally. Linux CI
  is configured to run it; the remote workflow has not been executed here.
- Region updates preserve the public sorted slice. Reverse insertion still
  has quadratic movement, bounded by MaxRegionWork; this is not a claim of
  an asymptotically optimal region data structure.
- Existing public entry points and snapshot lifetime remain. Limits and
  Derivation gained fields, and ErrLimit was added. Broader API/lazy-I/O,
  cancellation, corpus and release decisions are in docs/release-checklist.md.
