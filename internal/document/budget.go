package document

import (
	"github.com/MyungSub0519/gopd/internal/pdfmodel"
	"github.com/MyungSub0519/gopd/internal/syntax"
)

// parseObject shares the direct-value budget across trailers and loaded objects.
// Failed attempts are charged too; retries must not reset the work allowance.
func (d *Document) parseObject(data []byte, source pdfmodel.SourceID, offset int64) (pdfmodel.Object, int, error) {
	object, consumed, values, err := syntax.ParseObjectWithValueBudget(data, source, offset, d.Options.Limits, d.Options.Limits.MaxValues-d.parsedValues)
	d.parsedValues += values
	return object, consumed, err
}
