package gopd

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
