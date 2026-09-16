package pdfmodel

type Version struct {
	Major uint8
	Minor uint8
}

type Header struct {
	Span    Span
	Version Version // 파일 헤더의 버전. Catalog의 /Version과 별도다.
}

type FileTail struct {
	StartXRef Span
	Offset    int64
	OffsetRaw Span
	EOFMarker Span
}

type RegionKind uint8

const (
	RegionUnknown RegionKind = iota
	RegionHeader
	RegionTrivia
	RegionIndirectObject
	RegionXRef
	RegionTrailer
	RegionFileTail
)

type FileRegion struct {
	Kind RegionKind
	Span Span
}

type Severity uint8

const (
	SeverityInfo Severity = iota + 1
	SeverityWarning
	SeverityError
)

type Diagnostic struct {
	Severity Severity
	Code     string
	Message  string
	Span     Span
	Related  []Span
}

type Limits struct {
	MaxDepth        int
	MaxTokenBytes   int64
	MaxObjects      int
	MaxXRefSections int
	MaxDecodedBytes int64 // 분석 세션 전체 디코딩 출력 예산
	// MaxValues bounds cumulative direct syntax values in a Document, and
	// separately the content operands interpreted by one BuildPDF call.
	MaxValues int
	// MaxContentBytes counts decoded content bytes on every execution, including reuse.
	MaxContentBytes int64
	// MaxSemanticObjects counts page visits, stream visits, operators and annotations.
	MaxSemanticObjects int
	// MaxRegionWork bounds entries examined or moved while maintaining file regions.
	MaxRegionWork int64
}

type Structure struct {
	File        SourceID
	Header      *Header
	Tails       []FileTail
	XRefs       []XRefSection
	Objects     []IndirectObject // 지금까지 파싱한 발생 목록
	Regions     []FileRegion
	Diagnostics []Diagnostic
}

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

func (FreeEntry) pdfXRefEntry() {}

func (InUseEntry) pdfXRefEntry() {}

func (CompressedEntry) pdfXRefEntry() {}

func (UnknownXRefEntry) pdfXRefEntry() {}
