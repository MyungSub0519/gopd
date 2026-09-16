package gopd

import (
	"github.com/MyungSub0519/gopd/internal/content"
)

// ParsePDF snapshots a file and returns basic text and graphics grouped by page.
// It retains eager detailed analysis for Details; it is not selective parsing.
// A semantic failure can return both a partial PDF and an error. Callers must
// check err before treating the result as successful. No Close call is required.
func ParsePDF(path string) (*PDF, error) {
	detail, err := Open(path)
	return content.BasicPDF(detail), err
}
