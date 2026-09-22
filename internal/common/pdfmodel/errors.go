package pdfmodel

import "errors"

// ErrLimit identifies exhausted parser resource budgets.
var ErrLimit = errors.New("PDF resource limit exceeded")
