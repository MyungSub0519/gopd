package model

import (
	"errors"
	"fmt"
	"math"
	"strconv"
)

// ErrMissingKey reports that a dictionary key is absent.
//
// Use errors.Is to test for it. An absent key is routine in PDF, where most
// entries are optional and have defaults, so callers need to tell it apart
// from a genuine lookup failure such as a duplicate key.
var ErrMissingKey = errors.New("PDF dictionary key is missing")

// Get returns the value stored under key.
//
// A duplicate key is an error rather than a silent choice. The specification
// does not define which occurrence wins, so picking one would discard data the
// file actually contains; GetAll exposes every occurrence for callers that
// want to inspect the damage instead.
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

// GetAll returns every value stored under key, in the order written. It is the
// way to inspect a damaged dictionary that Get rejects for having duplicates.
func (d Dictionary) GetAll(key Name) []Object {
	var values []Object
	for _, entry := range d.Entries {
		if entry.Key == key {
			values = append(values, entry.Value)
		}
	}
	return values
}

// Int converts a direct PDF Integer to int64.
//
// A Real is rejected even when it holds a whole number, because the places
// that call for an integer — object numbers, /Length, array indices — are
// defined to require one. Indirect references are not followed; resolve them
// with Document.ResolveObject first.
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

// Number converts a direct Integer or Real to a finite float64.
//
// Infinities and NaN are rejected: they cannot appear in valid PDF syntax, and
// letting one through would silently poison every later coordinate computation
// it takes part in. Exact decimal precision is not preserved, and indirect
// references are not followed.
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

// IsStream reports whether object directly holds a Stream. It does not follow
// references; use Document.ResolveObject first when the object may be indirect.
func IsStream(object Object) bool { _, ok := object.Value.(Stream); return ok }
