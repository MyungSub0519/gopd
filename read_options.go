package gopd

import (
	"errors"
)

// ReadOptions bounds input, recursive parsing, xrefs and decoded stream data.
// Zero fields use defaults. Negative values are invalid.
type ReadOptions struct {
	MaxFileBytes int64
	Limits       Limits
}

func normalizeOptions(options []ReadOptions) (ReadOptions, error) {
	if len(options) > 1 {
		return ReadOptions{}, errors.New("at most one ReadOptions value is accepted")
	}
	var o ReadOptions
	if len(options) == 1 {
		o = options[0]
	}
	if o.MaxFileBytes < 0 || o.Limits.MaxDepth < 0 || o.Limits.MaxTokenBytes < 0 || o.Limits.MaxObjects < 0 || o.Limits.MaxXRefSections < 0 || o.Limits.MaxDecodedBytes < 0 {
		return o, errors.New("negative PDF read limit")
	}
	if o.MaxFileBytes == 0 {
		o.MaxFileBytes = 256 << 20
	}
	if o.Limits.MaxDepth == 0 {
		o.Limits.MaxDepth = 256
	}
	if o.Limits.MaxTokenBytes == 0 {
		o.Limits.MaxTokenBytes = 16 << 20
	}
	if o.Limits.MaxObjects == 0 {
		o.Limits.MaxObjects = 1_000_000
	}
	if o.Limits.MaxXRefSections == 0 {
		o.Limits.MaxXRefSections = 256
	}
	if o.Limits.MaxDecodedBytes == 0 {
		o.Limits.MaxDecodedBytes = 256 << 20
	}
	return o, nil
}
