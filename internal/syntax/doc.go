// Package syntax scans PDF byte ranges and parses direct objects and references.
// It preserves source spans and enforces syntax limits without loading files or
// resolving document objects. Stream payloads must be excluded by the caller.
package syntax
