// Package gopd parses PDF page content and preserves its underlying structure.
//
// # Selective content
//
// Extract and ExtractReader return an Extraction grouped by page, generating
// only selected content. Zero ExtractOptions select Unicode text. Content flags
// combine text, graphics, images, and annotations; Positions, Styles, Glyphs,
// and Provenance select details. Glyphs requires text and enables Positions.
// Graphics always include path geometry. Without Provenance, the returned
// extraction retains neither a Document nor a DetailedPDF. Provenance retains
// source access and page operations; its Document is excluded from JSON.
//
// # Basic content
//
// ParsePDF reads a file and returns Texts and Graphics grouped by page. Use
// PDF.Details to access the detailed result already produced by that parse.
//
// # Detailed content
//
// Open and Read return a DetailedPDF containing page geometry, text glyphs,
// graphics, images, fonts, annotations, content operations, and source spans.
// BuildPDF interprets a Document obtained through the low-level input API.
//
// # Low-level inspection
//
// ParseFile and Parse create a Document with bounded input and decoding limits.
// Document methods resolve objects, decode streams, and read original or
// decoded byte ranges. Lex and ParseObject inspect independent syntax ranges.
// Dictionary methods, Int, Number, and IsStream inspect direct object values.
// Matrix methods provide coordinate transformation and composition.
//
// # Ownership and interpretation limits
//
// Parsing snapshots input and caches decoded sources in memory during a call.
// Detailed and basic results retain the snapshot; extraction retains it only
// with Provenance. This is not streaming I/O or an exact process-memory bound.
// No Close call is needed. Treat results as read-only. Lazy Document methods
// are not safe for concurrent calls.
// Basic parsing also retains detailed analysis and is not a low-memory mode.
// Always check returned errors, including when a partial result is non-nil.
// Unsupported effects may instead be reported in result diagnostics. Selective
// extraction does not validate skipped resources or unrequested interpretation.
// Coordinates use unrotated PDF user space, and content order is drawing order,
// not reconstructed reading order. Rendering, OCR, and PDF editing are outside
// the current implementation.
package gopd
