package gopd

import (
	"errors"
	"fmt"
	"math"
	"strconv"
)

var ErrMissingKey = errors.New("PDF dictionary key is missing")

// Get rejects duplicate keys rather than silently discarding original entries.
func (d Dictionary) Get(key Name) (Object, error) {
	var value Object
	found := false
	for _, entry := range d.Entries {
		if entry.Key == key {
			if found {
				return Object{}, fmt.Errorf("duplicate PDF dictionary key /%s", key)
			}
			value, found = entry.Value, true
		}
	}
	if !found {
		return Object{}, fmt.Errorf("/%s: %w", key, ErrMissingKey)
	}
	return value, nil
}

// GetAll retains the stored order, including duplicate keys in damaged files.
func (d Dictionary) GetAll(key Name) []Object {
	var values []Object
	for _, entry := range d.Entries {
		if entry.Key == key {
			values = append(values, entry.Value)
		}
	}
	return values
}

func Int(object Object) (int64, error) {
	n, ok := object.Value.(Integer)
	if !ok {
		return 0, fmt.Errorf("expected PDF integer at %+v, got %T", object.Span, object.Value)
	}
	v, err := strconv.ParseInt(string(n), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("PDF integer %q: %w", n, err)
	}
	return v, nil
}

func Number(object Object) (float64, error) {
	var text string
	switch n := object.Value.(type) {
	case Integer:
		text = string(n)
	case Real:
		text = string(n)
	default:
		return 0, fmt.Errorf("expected PDF number, got %T", object.Value)
	}
	n, err := strconv.ParseFloat(text, 64)
	if err != nil || math.IsInf(n, 0) || math.IsNaN(n) {
		return 0, fmt.Errorf("PDF number %q is outside finite float64 range", text)
	}
	return n, nil
}
