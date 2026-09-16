package model

type XRefEntry interface {
	pdfXRefEntry()
}

type FreeEntry struct {
	NextFree   uint32
	Generation uint16
}

type InUseEntry struct {
	Offset     int64
	Generation uint16
}

type CompressedEntry struct {
	StreamNumber uint32
	Index        uint32
}

type UnknownXRefEntry struct {
	Type uint64 // 미지원 엔트리 종류; 나머지 원문은 XRefRecord.Span에 보존
}

type XRefRecord struct {
	Number uint32
	Span   Span // xref stream이면 디코딩 Source 안의 범위
	Entry  XRefEntry
}

type XRefRange struct {
	First   uint32
	Count   uint32
	Header  *Span // 텍스트 xref subsection 헤더. xref stream이면 nil
	Records []XRefRecord
}

type XRefForm uint8

const (
	XRefTable XRefForm = iota + 1
	XRefStream
)

type SectionID uint32

type XRefSection struct {
	ID       SectionID
	Form     XRefForm
	Offset   int64 // 원본 파일에서 xref 또는 xref stream 객체의 시작
	Span     Span
	Ranges   []XRefRange
	Trailer  Object // table trailer 또는 xref stream의 사전
	Prev     *int64 // 검증한 /Prev. 원래 값은 Trailer에 남는다.
	XRefStm  *int64 // 검증한 /XRefStm. hybrid 참조 관계를 보존한다.
	StreamID *ObjectID
}

func (FreeEntry) pdfXRefEntry()        {}
func (InUseEntry) pdfXRefEntry()       {}
func (CompressedEntry) pdfXRefEntry()  {}
func (UnknownXRefEntry) pdfXRefEntry() {}
