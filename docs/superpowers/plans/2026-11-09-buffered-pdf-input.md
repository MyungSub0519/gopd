# Buffered PDF input implementation plan

> Execution: implement in this session with focused delegated components and review. User authorized implementation after discussing 64 KiB configurable chunked input.

**Goal:** Remove the full original-file memory snapshot and expose ReadOptions.BufferSize (bytes, zero = 64 KiB, negative invalid).

**Architecture:** One shared bounded ReaderAt buffer owns original input access. Xref and indirect-object cursors parse across buffer boundaries, skipping encoded payloads by Length. Objects and decoded sources retain existing budgets; this is not a promise of bounded total output memory. Caller-owned ReaderAt stays open and unchanged until Document.Close; file-backed Documents own their file.

**Constraints:** Go standard library only; preserve spans, xref precedence, malformed-input checks and ErrLimit. Existing uncommitted work stays intact. No commit or push.

- [x] Add buffered source/cursor and tests for tiny buffers, comments, strings, references, short reads and limits.
- [x] Replace d.data access in read/xref/objects with int64 range access; prove a virtual 500 MiB PDF does not get read or allocated in full.
- [x] Add idempotent Close and integrate ownership into Basic/Layout/Full, legacy APIs and CLI.
- [x] Update snapshot-specific tests and documentation to explicit borrowed-source lifetime semantics; retain original-coordinate provenance.
- [x] Run full tests, vet, 386/CGO-disabled checks, fuzz, lint and vulnerability checks; review resource and lifetime regressions.

Review focus: read errors must retain causes; Length must skip binary data; nested object loading must not invalidate cursors; tokenizer must handle boundaries without quadratic reparsing; rejected options must not open/read input.

## Verification evidence

- `go test ./... -count=1`, `go vet ./...`, 386 tests/build: passed.
- golangci-lint v2.13.0: 0 issues; govulncheck v1.8.0: no vulnerabilities found.
- Cursor differential fuzz: 653,266 executions / 30s; document fuzz: 1,784,919 / 30s; BuildPDF fuzz: 4,291 / 30s; all passed.
- Race verification was attempted but unavailable: this Windows environment has CGO disabled and no configured C compiler. CI's Linux race job remains necessary.
- Virtual 500 MiB original with an unused large stream: default 64 KiB buffer, maximum backing request 65,536 bytes, total read 262,144 bytes. This measures structure/stream-boundary access, not decoding or full extraction throughput.
- Tiny buffers (1/2/3/7 bytes), nested indirect Length, binary stream payloads, xref/object streams and original spans passed.
- Virtual 3 GiB and MaxInt64 address spaces exercise 32-bit offsets and overflow-safe tail searching; oversized 32-bit range allocation returns ErrLimit.
- Review found tail-overlap overflow and 32-bit allocation error classification; both have failing-before/fixed-after regressions. Known unfiltered stream length now rejects an exhausted decoded budget before payload allocation/read.

## Remaining contract boundaries

The shared ReaderAt path may re-read evicted blocks. BufferSize is not a total memory cap: direct objects, encoded ranges used for decoding, decoded sources and outputs still consume memory under their documented limits. Full and legacy results require Close; borrowed readers must remain open and unchanged. No commits or pushes were made.
