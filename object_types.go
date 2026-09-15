package pdf

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

func (Null) pdfValue()         {}
func (Boolean) pdfValue()      {}
func (Integer) pdfValue()      {}
func (Real) pdfValue()         {}
func (Name) pdfValue()         {}
func (PDFString) pdfValue()    {}
func (Array) pdfValue()        {}
func (Dictionary) pdfValue()   {}
func (Reference) pdfValue()    {}
func (InvalidValue) pdfValue() {}
