package gopd

// ParsePDF snapshots a file and returns basic text and graphics grouped by page.
// It retains eager detailed analysis for Details; it is not selective parsing.
// A semantic failure can return both a partial PDF and an error. Callers must
// check err before treating the result as successful. No Close call is required.
func ParsePDF(path string) (*PDF, error) {
	detail, err := Open(path)
	return basicPDF(detail), err
}

// Details returns the already parsed detailed snapshot without re-reading the
// file. Detailed content slices use document-wide indexes; detailed Page.Items
// associates those indexes with pages. Treat the result as read-only; lazy
// Document methods are not safe for concurrent calls.
func (p *PDF) Details() *DetailedPDF {
	if p == nil {
		return nil
	}
	return p.details
}
