// Package structure reads the PDF file container: the header, the xref chain,
// indirect and compressed objects, and stream filters. Document owns the input
// snapshot and resolves references on demand.
package structure
