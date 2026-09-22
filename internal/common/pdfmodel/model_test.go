package pdfmodel

import (
	"errors"
	"math"
	"reflect"
	"strconv"
	"testing"
)

func TestDictionaryGetRejectsDuplicateKeys(t *testing.T) {
	t.Parallel()
	dictionary := Dictionary{Entries: []DictionaryEntry{
		{Key: "Width", Value: Object{Value: Integer("10")}},
		{Key: "Height", Value: Object{Value: Integer("20")}},
		{Key: "Width", Value: Object{Value: Integer("30")}},
	}}
	object, err := dictionary.Get("Width")
	if err == nil || errors.Is(err, ErrMissingKey) || object != (Object{}) {
		t.Fatalf("duplicate lookup = %+v, %v; want no object and a non-missing error", object, err)
	}
	if object, err := dictionary.Get("Absent"); !errors.Is(err, ErrMissingKey) || object != (Object{}) {
		t.Fatalf("missing lookup = %+v, %v; want no object and ErrMissingKey", object, err)
	}
	if _, err := (Dictionary{}).Get("Absent"); !errors.Is(err, ErrMissingKey) {
		t.Fatalf("empty dictionary lookup error = %v; want ErrMissingKey", err)
	}
}

func TestDictionaryGetAllPreservesDuplicateOrder(t *testing.T) {
	t.Parallel()
	first := Object{Span: Span{Source: 1, Start: 10, End: 12}, Value: Integer("10")}
	second := Object{Span: Span{Source: 1, Start: 30, End: 32}, Value: Integer("30")}
	dictionary := Dictionary{Entries: []DictionaryEntry{
		{Key: "Width", Value: first},
		{Key: "Height", Value: Object{Value: Integer("20")}},
		{Key: "Width", Value: second},
	}}
	if got := dictionary.GetAll("Width"); !reflect.DeepEqual(got, []Object{first, second}) {
		t.Fatalf("GetAll(Width) = %+v; want duplicate values in their original order", got)
	}
	if got := dictionary.GetAll("Absent"); got != nil {
		t.Fatalf("GetAll(Absent) = %+v; want nil", got)
	}
}

func TestIntValidation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		value   Value
		want    int64
		wantErr error
	}{
		{name: "minimum", value: Integer("-9223372036854775808"), want: math.MinInt64},
		{name: "maximum", value: Integer("9223372036854775807"), want: math.MaxInt64},
		{name: "overflow", value: Integer("9223372036854775808"), wantErr: strconv.ErrRange},
		{name: "underflow", value: Integer("-9223372036854775809"), wantErr: strconv.ErrRange},
		{name: "malformed", value: Integer("1.5"), wantErr: strconv.ErrSyntax},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := Int(Object{Value: test.value})
			if got != test.want || !errors.Is(err, test.wantErr) {
				t.Fatalf("Int(%v) = %d, %v; want %d, %v", test.value, got, err, test.want, test.wantErr)
			}
		})
	}
	if got, err := Int(Object{Value: Real("1")}); got != 0 || err == nil {
		t.Fatalf("Int(Real) = %d, %v; want zero and an error", got, err)
	}
}

func TestNumberValidation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		value   Value
		want    float64
		wantErr bool
	}{
		{name: "integer", value: Integer("-42"), want: -42},
		{name: "real", value: Real(".25"), want: 0.25},
		{name: "positive infinity", value: Real("+Inf"), wantErr: true},
		{name: "negative infinity", value: Real("-Inf"), wantErr: true},
		{name: "NaN", value: Real("NaN"), wantErr: true},
		{name: "overflow", value: Real("1e309"), wantErr: true},
		{name: "malformed", value: Real("not a number"), wantErr: true},
		{name: "wrong type", value: Name("42"), wantErr: true},
		{name: "reference", value: Reference{ID: ObjectID{Number: 42}}, wantErr: true},
		{name: "missing value", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := Number(Object{Value: test.value})
			if got != test.want || (err != nil) != test.wantErr {
				t.Fatalf("Number(%v) = %g, %v; want %g, error=%t", test.value, got, err, test.want, test.wantErr)
			}
		})
	}
}

func TestIsStreamDoesNotResolveReferences(t *testing.T) {
	t.Parallel()
	if IsStream(Object{Value: Reference{ID: ObjectID{Number: 1}}}) {
		t.Fatal("a reference is not a direct stream")
	}
	if IsStream(Object{}) {
		t.Fatal("an absent value is not a stream")
	}
}

func TestMatrixTransformIncludesOffDiagonalTerms(t *testing.T) {
	t.Parallel()
	matrix := Matrix{2, 3, 5, 7, 11, 13}
	point := Point{X: -2, Y: 4}
	if got := matrix.Transform(point); got != (Point{X: 27, Y: 35}) {
		t.Fatalf("Transform(%+v) = %+v; want {X:27 Y:35}", point, got)
	}
}

func TestMatrixMulComposesGeneralTransforms(t *testing.T) {
	t.Parallel()
	outer := Matrix{2, 3, 5, 7, 11, 13}
	inner := Matrix{17, 19, 23, 29, 31, 37}
	composed := outer.Mul(inner)
	if want := (Matrix{129, 184, 191, 272, 258, 365}); composed != want {
		t.Fatalf("Mul = %v; want %v", composed, want)
	}
	point := Point{X: -2, Y: 4}
	if got := composed.Transform(point); got != outer.Transform(inner.Transform(point)) {
		t.Fatalf("composed Transform = %+v; want inner transformation applied first", got)
	}
	if outer.Mul(IdentityMatrix()) != outer || IdentityMatrix().Mul(outer) != outer {
		t.Fatal("identity multiplication changed the transformation")
	}
}
