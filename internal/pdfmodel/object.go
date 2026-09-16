package pdfmodel

import (
	"errors"
	"fmt"
	"math"
	"strconv"
)

type Value interface {
	pdfValue()
}

type Object struct {
	Span  Span
	Value Value
}

type Null struct{}

type Boolean bool

// PDF 숫자 문법을 검증한 십진 표기. 자동으로 float64로 바꾸지 않는다.
type Integer string

type Real string

// '/'를 제외하고 #xx escape를 해제한 바이트열. UTF-8을 보장하지 않는다.
type Name string

type StringForm uint8

const (
	StringLiteral StringForm = iota + 1
	StringHex
)

type PDFString struct {
	Form  StringForm
	Bytes []byte // 구문 escape/hex 해제 후, 암호 해제 및 문자 디코딩 전
}

type Array struct {
	Items []Object
}

type DictionaryEntry struct {
	Key     Name
	KeySpan Span
	Value   Object
}

type Dictionary struct {
	Entries []DictionaryEntry
}

type ObjectID struct {
	Number     uint32
	Generation uint16
}

type Reference struct {
	ID ObjectID
}

// 손상된 구문을 표현하는 분석기 전용 값. PDF의 null과 구별한다.
type InvalidValue struct {
	Reason string
}

func (Null) pdfValue() {}

func (Boolean) pdfValue() {}

func (Integer) pdfValue() {}

func (Real) pdfValue() {}

func (Name) pdfValue() {}

func (PDFString) pdfValue() {}

func (Array) pdfValue() {}

func (Dictionary) pdfValue() {}

func (Reference) pdfValue() {}

func (InvalidValue) pdfValue() {}

type StreamBoundary uint8

const (
	StreamUnresolved StreamBoundary = iota
	StreamFromLength
	StreamRecovered
)

type Stream struct {
	Dictionary     Dictionary
	DictionarySpan Span
	StartKeyword   Span
	DataStart      Position
	Encoded        *Span // 경계를 모르면 nil
	EndKeyword     *Span // 확인하지 못했으면 nil
	Boundary       StreamBoundary
}

func (Stream) pdfValue() {}

type ObjectOrigin interface {
	pdfObjectOrigin()
}

type FileObjectOrigin struct {
	Whole  Span  // n g obj부터 확인한 endobj까지
	Header Span  // n g obj
	EndObj *Span // 손상으로 확인하지 못하면 nil
}

type ObjectStreamOrigin struct {
	Container     ObjectID
	ContainerSpan Span   // 원본 파일의 정확한 컨테이너 발생 위치
	Index         uint32 // object stream 내부 순번, generation이 아님
	HeaderPair    Span   // 디코딩 소스 안의 객체 번호/상대 오프셋 쌍
}

type IndirectObject struct {
	ID     ObjectID
	Body   Object
	Origin ObjectOrigin
}

func (FileObjectOrigin) pdfObjectOrigin() {}

func (ObjectStreamOrigin) pdfObjectOrigin() {}

// ErrMissingKey identifies absent dictionary keys. Use errors.Is to distinguish
// a missing key from a duplicate key or another lookup failure.
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

// Int converts a direct PDF Integer into int64, rejecting other types and
// out-of-range values. Resolve indirect references before calling Int.
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

// Number converts a direct Integer or Real into a finite float64. It does not
// resolve indirect references or preserve exact decimal precision.
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

// IsStream reports whether object directly contains a Stream. It does not
// follow references; resolve them before calling IsStream when necessary.
func IsStream(object Object) bool { _, ok := object.Value.(Stream); return ok }
