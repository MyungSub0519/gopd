package gopd

import "io"

type SourceID uint32

// Source == 0은 위치 없음. 오프셋은 항상 바이트 단위다.
type Position struct {
	Source SourceID
	Offset int64
}

// [Start, End): Start를 포함하고 End를 제외한다.
type Span struct {
	Source SourceID
	Start  int64
	End    int64
}

type Source struct {
	ID     SourceID
	Reader io.ReaderAt
	Size   int64
	Origin *Derivation // nil이면 원본 파일, 아니면 변환으로 얻은 소스
}

type Derivation struct {
	Input Span
	Steps []Transform
}

type TransformKind uint8

const (
	TransformFilter TransformKind = iota + 1
	TransformDecrypt
)

type Transform struct {
	Kind   TransformKind
	Name   Name
	Params *Object // 없으면 nil; PDF의 명시적 null과 구별
}
