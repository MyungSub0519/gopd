# Parser content layout implementation plan

**Goal:** Organize `internal/parser` around the content returned to callers, with
`parser.go` coordinating `page.go`, `text.go`, `graphic.go`, `image.go`, and
`annotation.go`.

**Design:** Keep one Go package and the existing shared interpreter state.
Each content file owns its result types, interpretation helpers, and emission.
`interpreter.go` retains ordered scanning, shared state, and operator dispatch;
`resource.go` retains resource lookup, Form execution, and ExtGState handling.
`font.go` and `cmap.go` support text. Public root aliases and API contracts remain.
This implements the content-oriented structure discussed and requested in chat.

## Constraints

- Preserve current uncommitted work; do not create commits or reset the tree.
- Keep all public types, fields, constants, JSON tags, and entry points.
- Preserve selection gates, diagnostic order, error/partial-result behavior,
  shared state, cache keys, source spans, and cumulative resource limits.
- Retain tests and their existing coverage; test-file consolidation is separate.
- Preserve function bodies mechanically where possible; operator and image
  extraction are the only changes to call boundaries.

## Tasks

- [x] Establish a passing full-suite baseline and map current declarations.
- [x] Snapshot implementation, redistribute declarations by content, and extract
  text/graphic operator dispatch plus image handling without changing behavior.
- [x] Update current documentation and verify public API, all tests, build,
  vet, formatting, and relocated source references.
- [x] Independently review the delta against the snapshot and address findings.

## Review focus

- Text state and graphics transforms survive stream boundaries and Form calls.
- Disabled output categories still process state needed by requested content.
- Cached images retain identity and per-use placement/diagnostic behavior.
- Malformed syntax preserves the existing partial results and error wrapping.
- Budget accounting and provenance options retain their exact execution order.

## Verification

- Full test suite passed before and after the refactor; final run used
  `go test ./... -count=1 -shuffle=on -timeout=5m`.
- `go build ./...` and `go vet ./...` passed.
- Snapshot comparison verified 213 unchanged declarations. Only `execute` and
  `xobject` changed call boundaries; three content handlers were added.
- All 71 operator cases retain their original statement tokens after dispatch.
- Independent review found no actionable defects in state, caching, budgets,
  optional selection, errors, result types, or JSON tags.
- Active documentation links were checked against the new file locations.
