// Package parser interprets PDF pages, content operations, fonts, images, and
// annotations into basic, detailed, or selectively extracted results. It owns
// the shared content state and uses common/document, common/syntax, and
// common/pdfmodel for input, object access, syntax, and PDF values.
// The root gopd package exposes its entry points and result types to callers.
// Content-specific files keep their result types, interpretation, and emission
// together; one interpreter preserves execution order and shared graphics state.
package parser
