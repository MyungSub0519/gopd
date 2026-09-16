// Package gopd parses PDF page content and preserves its underlying structure.
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
// Results retain an in-memory input snapshot; no Close call is needed. Treat
// results as read-only. Lazy Document methods are not safe for concurrent calls.
// Basic parsing also retains detailed analysis and is not a low-memory mode.
// Always check returned errors, including when a partial result is non-nil.
// Unsupported effects may instead be reported in detailed diagnostics.
// Image bytes are located but never decoded, for inline and XObject images
// alike; image codecs such as DCTDecode are outside the current implementation.
// Coordinates use unrotated PDF user space, and content order is drawing order,
// not reconstructed reading order. Rendering, OCR, and PDF editing are outside
// the current implementation.
package gopd
