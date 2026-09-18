# 기본 PDF API

`ParsePDF`는 상세 분석 결과를 함께 보관하는 기존 API입니다. 콘텐츠 종류와 상세 수준을 선택해 필요한 결과만 생성하려면 [선택 추출 API](selective-extraction.md)의 `Extract` 또는 `ExtractReader`를 사용합니다. 기존 `ParsePDF`와 `Details()` 동작은 유지됩니다.

## 파일 경로 하나로 파싱하기

```go
package main

import (
    "fmt"
    "log"

    "github.com/MyungSub0519/gopd"
)

func main() {
    doc, err := gopd.ParsePDF("testdata/synthetic.pdf")
    if err != nil {
        log.Fatal(err)
    }
    for page, texts := range doc.Texts {
        fmt.Printf("page=%d texts=%d graphics=%d\n",
            page+1, len(texts), len(doc.Graphics[page]))
        for _, text := range texts {
            fmt.Printf("text=%q\n", text.Unicode)
        }
    }
}
```

CLI의 [main.go](../examples/gopd/main.go)에는 같은 기능을 호출하는 함수가 있습니다.

```go
func pdfparse(path string) (*gopd.PDF, error) {
    return gopd.ParsePDF(path)
}
```

이 파일의 `run`은 `doc, err := pdfparse(path)`로 기본 객체를 받은 뒤 텍스트나 요약을 출력합니다. 라이브러리 호출 자체는 출력을 하지 않고 객체와 오류를 반환합니다. 다른 패키지에서는 공개 함수 `gopd.ParsePDF`를 사용합니다. 전체 공개 함수·메서드는 [공개 API 목록](public-api.md)을 참고하세요.

## 반환 객체

기본 `PDF`의 공개 필드는 `Texts`와 `Graphics` 두 개입니다.

```go
type PDF struct {
    Texts    [][]Text
    Graphics [][]Graphic

    details *DetailedPDF
}
```

바깥 배열은 페이지 순서, 안쪽 배열은 해당 페이지의 요소 순서입니다. 인덱스는 모두 0부터 시작합니다.

| 접근 | 의미 |
| --- | --- |
| `len(doc.Texts)` | 파싱 결과에 포함된 페이지 수 |
| `doc.Texts[0]` | 첫 번째 페이지의 텍스트 표시 구간 |
| `doc.Texts[0][0]` | 첫 번째 페이지의 첫 번째 텍스트 표시 구간 |
| `doc.Graphics[1]` | 두 번째 페이지의 벡터 경로 그리기 결과 |
| `doc.Graphics[1][0]` | 두 번째 페이지의 첫 번째 벡터 경로 그리기 결과 |

각 인덱스로 접근하기 전에 배열 길이를 확인합니다. 텍스트나 그래픽이 없는 페이지도 바깥 배열에서 빠지지 않고 빈 배열 `[]`로 남습니다. 따라서 두 필드의 바깥 배열 길이는 항상 같습니다. 페이지가 없는 문서는 두 필드 모두 `[]`입니다.

각 안쪽 배열은 해당 종류의 콘텐츠 실행 순서를 유지합니다. 텍스트·그래픽·이미지 사이의 전체 그리기 순서는 상세 페이지의 `Items`에서 확인합니다.

```text
PDF
├─ Texts[page][element]
│  └─ Page, Unicode, Font, FontSize, Matrix, RenderingMode,
│     Style, DecodeComplete, PositionComplete
└─ Graphics[page][element]
   └─ Page, Segments[], Paint, Stroke, Fill, EvenOdd, Style
```

실제 타입 정의는 [basic.go](../basic.go)에 있습니다.

| 정보 | 설명 |
| --- | --- |
| 콘텐츠의 `Page` | 바깥 배열 인덱스와 같은 페이지 번호. 개별 요소를 분리해 사용할 때도 페이지를 알 수 있음 |
| `Text.Font` | `BaseFont`, `Subtype`을 담은 공유 `*FontInfo`. 글꼴을 선택하지 않은 상태는 `nil` |
| `Text.FontSize` | 해당 텍스트에 적용한 글꼴 크기 |
| `Text.Matrix` | 표시 시작 시점의 페이지 공간 텍스트 행렬. 글꼴 크기 배율 적용 전 값 |
| `Graphic.Segments[]` | `Operator`, `Points[]`. 점은 이미 페이지 좌표계로 변환됨 |
| `Style` | 윤곽선·채우기 색상, 선 두께·끝·연결·점선, 불투명도, 혼합 방식 |
| `Style.Clipped` | 상세 상태에 클리핑 경로가 있는지 여부. 경로 자체는 `Details()`에서 확인 |
| `Style.Complete` | 현재 해석기의 미지원 효과 등 진단을 반영한 플래그 |

글꼴은 원본 리소스 기준으로 페이지 사이에서도 공유합니다. 이름이 같더라도 서로 다른 글꼴 객체는 서로 다른 포인터입니다.

좌표는 페이지 회전을 자동 반영하지 않은 PDF 사용자 좌표계입니다. 상세 페이지의 `Rotate`, `UserUnit`과 표시 배율을 고려해 화면 좌표로 변환합니다. 콘텐츠 순서는 그리기 순서이며 문단의 읽기 순서는 아닙니다.

## 상세 정보에 접근하기

```go
detail := doc.Details()
if len(detail.Texts) > 0 && len(detail.Texts[0].Source.Spans) > 0 {
    raw, err := detail.Document.Bytes(detail.Texts[0].Source.Spans[0])
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("source bytes: %x\n", raw)
}
```

상세 `Texts`와 `Graphics`는 문서 전체의 평면 배열입니다. 기본 결과는 페이지별 배열이므로 인덱스가 직접 대응하지 않습니다. 특정 페이지의 상세 텍스트는 그 페이지의 `Items`를 따라 찾습니다.

```go
detail := doc.Details()
if len(detail.Pages) > 0 {
    for _, item := range detail.Pages[0].Items {
        if item.Kind == gopd.ElementText {
            text := detail.Texts[item.Index]
            fmt.Println(text.Unicode, len(text.Glyphs))
        }
    }
}
```

| 정보 | 접근 경로 |
| --- | --- |
| 페이지 영역·회전·단위·해석 상태 | `detail.Pages[page]` |
| 종류 사이의 전체 그리기 순서 | `detail.Pages[page].Items` |
| 이미지 배치와 공유 리소스 | `detail.Images`, `detail.ImageResources` |
| 콘텐츠 해석 진단 | `detail.Diagnostics` |
| 파일 구조 진단 | `detail.Structure.Diagnostics` |

| 진입점 | 반환값과 역할 |
| --- | --- |
| `Extract(path, options)`, `ExtractReader(readerAt, size, options)` | `*Extraction`: 종류·상세 수준을 선택한 페이지별 결과 |
| `ParsePDF(path)` | `*PDF`: 페이지별 텍스트·그래픽 |
| `PDF.Details()` | `*DetailedPDF`: 같은 파싱의 상세 결과 |
| `Open(path)` | `*DetailedPDF`: 파일에서 바로 상세 분석 |
| `Read(readerAt, size)` | `*DetailedPDF`: reader에서 상세 분석 |
| `BuildPDF(document)` | `*DetailedPDF`: 저수준 문서의 콘텐츠 해석 |
| `ParseFile(path)`, `Parse(readerAt, size, options...)` | `*Document`: 저수준 구문·xref·객체 접근 |

상세 결과에는 `Fonts`, `ImageResources`, `Annotations`, `Structure`, `Document` 및 각 요소의 글리프·명령·출처·전체 그래픽 상태가 남아 있습니다. 상세 타입은 [detailed_model.go](../detailed_model.go), 기존 상세 저장 형식은 [JSON 구조 설명](json-structure.md)을 참고하세요.

이전에 상세 결과를 `*PDF`로 선언한 코드는 `*DetailedPDF`로 바꿉니다. 상세 요소를 직접 선언했다면 `DetailedPage`, `DetailedText`, `DetailedGraphic`, `DetailedImage`, `DetailedPathSegment`를 사용합니다. 기존 `Open`·`Read`·`BuildPDF`의 상세 동작은 유지합니다.

## 기본 JSON 저장

`encoding/json`의 `json.NewEncoder(writer).Encode(doc)` 또는 `json.MarshalIndent(doc, "", "  ")`로 기본 객체를 저장할 수 있습니다. JSON 루트의 키는 `Texts`와 `Graphics` 두 개입니다.

예를 들어 콘텐츠가 없는 3페이지 문서는 다음과 같습니다.

```json
{
  "Texts": [[], [], []],
  "Graphics": [[], [], []]
}
```

요소가 있으면 해당 페이지의 안쪽 배열에 Text 또는 Graphic 객체가 들어갑니다. 저장소의 [합성 테스트 PDF](../testdata/synthetic.pdf)는 텍스트·그래픽·이미지를 포함하는 두 페이지 문서입니다. 생성 방법과 예상 결과는 [테스트 데이터 안내](../testdata/README.md)를 참고하세요.

원본 객체·바이트 범위·명령·글리프·CMap·스트림·상세 진단은 자동으로 직렬화하지 않습니다. 내부 상세 결과 포인터는 비공개 필드입니다. 글꼴의 기본 정보는 각 사용 위치에 중첩해 저장되므로 Go에서 공유하는 포인터 관계가 JSON에 자동 보존되는 것은 아닙니다.

`go run ./examples/gopd -json file.pdf`는 기존대로 개수 요약을 출력합니다. 기본 객체 전체 JSON을 출력하는 CLI 옵션을 추가한 것은 아닙니다.

## 기존 기본 API에서 이전하기

| 이전 접근 | 현재 접근 |
| --- | --- |
| `doc.Pages` | `doc.Details().Pages` |
| `doc.Texts[i]` | 페이지별 `doc.Texts[page][i]`. 문서 전체 평면 배열은 `doc.Details().Texts` |
| `doc.Graphics[i]` | 페이지별 `doc.Graphics[page][i]`. 문서 전체 평면 배열은 `doc.Details().Graphics` |
| `doc.Images` | `doc.Details().Images`와 `ImageResources` |
| `doc.Diagnostics` | `doc.Details().Diagnostics`와 `Structure.Diagnostics` |
| `len(doc.Texts)`로 전체 텍스트 수 계산 | 각 페이지의 길이를 합산하거나 `len(doc.Details().Texts)` 사용 |

## 오류와 현재 처리 범위

- 존재하지 않는 파일이나 구문 오류는 `error`로 반환합니다.
- 콘텐츠 해석 도중 실패하면 부분 `PDF`와 오류가 함께 반환될 수 있습니다. 결과가 `nil`이 아니어도 반드시 오류를 확인합니다. 부분 결과의 바깥 배열은 오류 전까지 상세 결과에 추가된 페이지에 대응합니다.
- 미지원 효과는 오류 없이 상세 진단으로 남을 수도 있습니다. `Details().Diagnostics`와 `Details().Structure.Diagnostics`에서 원인과 바이트 출처를 확인합니다.
- 문자 해석은 `DecodeComplete`, 위치 계산 지원 여부는 `PositionComplete`로 확인합니다. 기본 결과에 글리프별 좌표 전체가 포함된다는 의미는 아닙니다.
- 기본 스타일은 모든 렌더링 정보를 담지 않습니다. 전체 클리핑 경로·좌표 변환 상태 등은 상세 정보에 남아 있습니다.
- 현재 `ParsePDF`는 상세 분석을 먼저 수행하고 기본 결과를 생성하며 상세 결과도 보관합니다. 필요한 결과만 직접 생성하려면 `Extract`를 사용합니다. 선택 추출도 호출 중 입력과 디코딩 캐시를 보관하므로 전체 프로세스 메모리 상한을 보장하지 않습니다.
- `Details()`는 이미 보관한 결과를 반환하므로 파일을 다시 읽지 않습니다. 반환 객체를 닫을 필요는 없습니다.
- 결과는 읽기 전용으로 취급합니다. 기본 슬라이스 데이터는 상세 슬라이스와 분리하지만 같은 기본 리소스를 가리키는 포인터는 공유합니다. 상세 `Document`의 지연 메서드는 동시 호출을 지원하지 않습니다.
