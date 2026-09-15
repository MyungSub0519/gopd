package gopd

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
