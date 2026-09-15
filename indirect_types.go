package pdf

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

func (FileObjectOrigin) pdfObjectOrigin()   {}
func (ObjectStreamOrigin) pdfObjectOrigin() {}
