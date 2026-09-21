# Contributing

Use Go 1.25 or newer. The parser uses the Go standard library; discuss changes
that introduce a PDF parsing dependency before adding one.

For parser fixes, include a small failing regression fixture and the relevant
PDF specification clause. Prefer synthetic fixtures built with
`internal/common/pdftest` and literal expected results. Do not use private documents or
personal information as test data. A checked-in PDF corpus must include its
origin and redistribution permission. The checked-in
[`testdata/synthetic.pdf`](testdata/README.md) is generated from invented content
using the Go standard library and is required by integration and CLI tests.

Keep the root package focused on public forwarding functions and aliases.
Content interpretation, extraction, and result models live in `internal/parser`;
shared PDF models, syntax, document reading, and fixture builders live in
`internal/common`. Keep implementation tests with their package and public API
tests in the root `gopd_test` package. See the
[project structure](docs/project-structure.md) for the dependency boundaries.

Run these checks before submitting a change:

```text
go test ./... -count=1
go vet ./...
golangci-lint run ./...
go test -race ./... -count=1
go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 ./...
```

The race detector requires a supported CGO toolchain. CI runs it on Linux and
also runs tests for 386. Use `GOARCH=386` and `CGO_ENABLED=0` in your shell to
reproduce the 32-bit check.

Run one fuzz target per invocation, for example:

```text
go test ./internal/common/syntax -run '^$' -fuzz '^FuzzParseObject$' -fuzztime=30s -parallel=2
go test ./internal/common/document -run '^$' -fuzz '^FuzzDocument$' -fuzztime=30s -parallel=2
go test ./internal/common/document -run '^$' -fuzz '^FuzzDecodeFilters$' -fuzztime=30s -parallel=2
go test ./internal/parser -run '^$' -fuzz '^FuzzToUnicodeBounded$' -fuzztime=30s -parallel=2
go test ./internal/parser -run '^$' -fuzz '^FuzzBuildPDF$' -fuzztime=30s -parallel=2
```

Keep fuzz inputs and resource limits small. Preserve minimized failures in
the target package's `testdata/fuzz` directory as regression seeds, and test successful results' spans, indices,
budgets and completeness rather than only checking for panics.

Performance changes should include allocation/read-count evidence or a benchmark
with multiple input sizes. Do not assert wall-clock thresholds in unit tests.
Update [resource accounting](docs/resource-limits.md) when changing a limit.

License selection is still a release prerequisite; see the
[release checklist](docs/release-checklist.md).
