package structure

import (
	"errors"

	"github.com/MyungSub0519/gopd/internal/model"
)

// ReadOptions bounds a single read: how large an input is accepted, and how
// much work parsing it may perform.
//
// A zero field means "use the default", so the zero ReadOptions is the usual
// configuration rather than a degenerate one. A negative field is rejected, on
// the grounds that it is far more likely to be a bug than a request for
// unlimited work.
type ReadOptions struct {
	// MaxFileBytes caps the input snapshot; the default is 256 MiB.
	MaxFileBytes int64

	// Limits caps the work done over that input; see model.Limits.
	Limits model.Limits
}

// normalizeOptions validates the caller's options and fills in defaults.
//
// It takes a slice because the public entry points accept options variadically,
// which is how an optional argument is expressed without a second function;
// more than one value is a mistake rather than a merge.
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
