# Release preparation

The parser-hardening work fixes the concrete review regressions while retaining
the existing public API and snapshot ownership model. The following decisions
remain separate release work:

- [ ] Maintainer selects a license and adds LICENSE; confirm provenance of any
  copied code and redistributable PDF/font fixtures.
- [ ] Enable a private vulnerability reporting channel and publish supported
  versions and response expectations.
- [ ] Publish a versioned feature matrix for PDF versions, filters, font
  encodings, encryption and strict/recovery behavior.
- [ ] Establish a redistributable real-world PDF corpus alongside synthetic
  regression and fuzz fixtures. The checked-in `testdata/synthetic.pdf` is
  required by integration and CLI tests; a missing fixture fails these tests.
- [ ] Decide the stable public model boundary: mutable Document fields and
  internal type aliases currently expose implementation choices. Definitions now
  live in `internal/common` and `internal/parser`; document the resulting changes
  to reflection package paths and `%T` output when versioning the release.
- [ ] Design selective/page-at-a-time interpretation and bounded lazy ReaderAt
  access without changing existing snapshot lifetimes.
- [ ] Design cancellation and consistent structured errors across all parser
  layers. ErrLimit currently classifies resource exhaustion; syntax errors still
  carry their location primarily in the message.
- [ ] Define compatibility/versioning for result structs, enum values and JSON.

The current file-region representation remains an immediately inspectable sorted
slice. Binary search and in-place splices improve normal access, and a work budget
bounds worst-case insertion costs; replacing it with a lazy interval view is an
API design decision for a later version.
