# 필요한 콘텐츠만 추출하기

`ParseFile`과 `ParseReader`는 텍스트·그래픽·이미지·주석 중 요청한 결과를 직접 생성합니다. 콘텐츠 종류가 같은 객체 로더와 명령 순회기를 공유하므로, 텍스트와 이미지를 함께 선택해도 종류별로 콘텐츠를 다시 순회하지 않습니다. 기존 `ParsePDF`·`Open`·`Read`·`BuildPDF`와 `PDF.Details()`의 동작은 유지됩니다.

## 텍스트만 읽기

```go
package main

import (
    "fmt"
    "log"

    "github.com/MyungSub0519/gopd"
)

func main() {
    result, err := gopd.ParseFile("testdata/synthetic.pdf", gopd.ParseOptions{})
    if err != nil {
        log.Fatal(err)
    }
    for _, page := range result.Pages {
        for _, text := range page.Texts {
            fmt.Printf("page=%d text=%q\n", page.Index+1, text.Unicode)
        }
    }
}
```

`ParseOptions{}`는 `ContentText`를 선택합니다. 텍스트에는 `Unicode`, 공유 `*FontInfo`, `DecodeComplete`가 포함되며 위치·스타일·글리프·출처는 생성하지 않습니다. 텍스트 요소 하나는 텍스트 표시 명령 한 번에 대응합니다. 문장·문단·표나 읽기 순서를 복원하지 않습니다.

파일 경로 대신 이미 확보한 바이트를 사용할 때는 `bytes.NewReader(data)`와 `int64(len(data))`를 `ParseReader`에 전달합니다. reader는 `io.ReaderAt`을 구현해야 합니다. `ParseFile`은 파일을 반환 전에 닫고, `ParseReader`는 전달받은 reader를 닫지 않습니다.

## 콘텐츠와 상세 수준 선택

다음 코드는 텍스트와 이미지의 위치를 함께 추출합니다.

```go
result, err := gopd.ParseFile("testdata/synthetic.pdf", gopd.ParseOptions{
    Content:   gopd.ContentText | gopd.ContentImages,
    Positions: true,
})
if err != nil {
    log.Fatal(err)
}
fmt.Println(len(result.Pages), len(result.ImageResources))
```

| `Content` 플래그 | 생성하는 결과 |
| --- | --- |
| `ContentText` | `Pages[i].Texts`: Unicode 텍스트와 기본 글꼴 정보 |
| `ContentGraphics` | `Pages[i].Graphics`: 경로 점, 그리기 명령, 윤곽선·채우기 여부 |
| `ContentImages` | `Pages[i].Images`: 이미지 사용 위치별 요소. 공통 메타데이터는 `ImageResources`에 저장 |
| `ContentAnnotations` | `Pages[i].Annotations`: 주석 종류와 사각형. appearance stream은 실행하지 않음 |
| `ContentAll` | 위 네 종류 모두 |

플래그는 `|`로 조합합니다. `Content: 0`은 텍스트 기본값이며, 알려지지 않은 비트는 입력을 읽기 전에 오류로 반환합니다.

| 옵션 | 추가되는 정보와 작업 |
| --- | --- |
| `Positions` | 텍스트의 `Position`과 이미지의 `Matrix`. 글꼴 폭과 좌표 변환을 계산 |
| `Glyphs` | 텍스트의 문자 코드·Unicode·원점·이동량을 담은 `Glyphs`. 텍스트 선택이 필요하며 `Positions`도 활성화 |
| `Styles` | 텍스트·그래픽·이미지의 `Style`. 색상·선·불투명도·혼합 방식과 클리핑 여부를 해석 |
| `Provenance` | 요소 출처, 페이지별 명령, 원본 이미지 객체와 `Document` 보관 |
| `ReadOptions` | 파일 크기, 디코딩, 구문과 의미 해석에 적용할 기존 제한 설정 |

`Positions: true`만으로 글리프 목록이 생성되지는 않습니다. 반대로 `Glyphs: true`는 위치 계산까지 요청합니다. 예를 들어 `ContentImages`만 선택하면서 `Glyphs: true`를 지정하면 옵션 오류입니다.

그래픽은 위치가 경로 자체의 일부이므로 `Positions: false`여도 `Segments[].Points`를 포함합니다. 주석의 `Rect`도 항상 포함됩니다. 모든 좌표는 페이지 회전을 반영하지 않은 PDF 사용자 좌표계입니다. 화면에 표시하려면 페이지의 `Rotate`, `UserUnit`과 표시 배율을 적용합니다. 텍스트의 `Position.Matrix`는 표시 명령 시작 시점의 행렬이며 글꼴 크기 배율을 적용하기 전 값입니다.

스타일은 기본 `PaintStyle`입니다. `Clipped`는 클리핑 경로 존재 여부를 나타내며 전체 클리핑 경로나 모든 렌더링 효과를 제공하지 않습니다.

## 반환 구조와 JSON

```text
Result
├─ Content
├─ Pages[]: ExtractedPage
│  ├─ Index, MediaBox, CropBox, Rotate, UserUnit, Complete
│  ├─ Texts[]: ExtractedText
│  ├─ Graphics[]: ExtractedGraphic
│  ├─ Images[]: ExtractedImage
│  ├─ Annotations[]: ExtractedAnnotation
│  └─ Operations[]                     (Provenance)
├─ ImageResources[]: ExtractedImageResource
├─ Diagnostics[]: ResultDiagnostic
└─ Document                            (Provenance; JSON 제외)
```

페이지와 리소스 인덱스는 0부터 시작합니다. `Images[j].Resource`는 `result.ImageResources`의 인덱스입니다. 같은 이미지가 여러 번 사용되면 메타데이터는 공유하고 각 사용은 별도 `ExtractedImage`로 반환합니다. 픽셀을 디코딩하거나 이미지를 렌더링하지 않습니다. `ColorSpace`는 PDF 객체 형식을 유지하므로 복합 색 공간의 매개변수나 참조가 들어갈 수 있습니다.

`json.Marshal(result)`나 `json.NewEncoder(writer).Encode(result)`로 저장할 수 있습니다. 미선택 종류, 비어 있는 목록, 비활성 상세 필드는 `omitempty`에 따라 생략됩니다. 따라서 JSON에서 `Images`가 없다는 사실만으로 미선택인지 해당 페이지에 이미지가 없는지 구분할 수 없습니다. `result.Content & gopd.ContentImages != 0`으로 선택 여부를 확인합니다. 빈 문서의 `Pages`는 `[]`입니다.

각 종류의 요소는 그 종류 안에서 콘텐츠 실행 순서를 유지합니다. 기본 `PDF`의 `Texts[page][element]` 대신 이 API는 `result.Pages[page].Texts[element]`를 사용합니다. 문서 결과·옵션·진단은 [extract.go](../internal/parser/extract.go), 페이지 결과는 [page.go](../internal/parser/page.go)에 정의합니다. 요소별 결과 타입과 출력 코드는 [text.go](../internal/parser/text.go), [graphic.go](../internal/parser/graphic.go), [image.go](../internal/parser/image.go), [annotation.go](../internal/parser/annotation.go)에 있습니다.

## 원본 바이트와 명령 확인

`Provenance: true`는 `Document`, `Pages[i].Operations`, 텍스트·그래픽·이미지의 `Source`를 보관합니다. `Source.Operations`는 해당 페이지 명령 배열의 인덱스이고 `Source.FormPath`는 중첩 Form 호출 경로입니다. 주석의 `Source`와 진단의 `Span`도 사용할 수 있습니다. 주석만 요청하면 페이지 콘텐츠를 실행하지 않으므로 `Operations`는 생성되지 않습니다.

이미지의 원본 인코딩 바이트는 다음과 같이 읽습니다.

```go
result, err := gopd.ParseFile("testdata/synthetic.pdf", gopd.ParseOptions{
    Content:    gopd.ContentImages,
    Provenance: true,
})
if err != nil {
    log.Fatal(err)
}
for _, resource := range result.ImageResources {
    if resource.Object == nil {
        continue
    }
    stream, ok := resource.Object.Value.(gopd.Stream)
    if !ok || stream.Encoded == nil {
        continue
    }
    encoded, err := result.Document.Bytes(*stream.Encoded)
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("image=%v encoded bytes=%d\n", resource.ID, len(encoded))
}
```

인코딩된 스트림이 독립적인 PNG/JPEG 파일이라는 보장은 없습니다. 필터와 이미지 메타데이터를 함께 해석해야 합니다. `Document`는 JSON에서 항상 제외되므로 JSON만 저장하면 원본 바이트까지 저장되는 것은 아닙니다. `Provenance`를 끄면 결과에 `Document`, 전체 폰트/CMap 또는 상세 결과를 숨겨 보관하지 않습니다. `Result`에는 `Details()` 메서드가 없습니다.

## 실제로 생략하는 작업

- 텍스트를 선택하지 않으면 글꼴 Unicode 해석을 생략합니다. 문자열만 요청하면 문자 폭, 글리프 좌표, 폰트 descriptor와 embedded font stream을 읽지 않습니다.
- 스타일을 요청하지 않으면 불필요한 색상·선 스타일 해석과 스타일 결과 생성을 생략합니다. 좌표 변환은 위치·그래픽 경로·스타일 해석에 필요한 경우에 수행합니다.
- 이미지를 선택하지 않아도 `Do`의 대상이 Form인지 확인합니다. Form 안에 필요한 텍스트나 그래픽이 있을 수 있기 때문입니다. 재사용 Form은 호출 당시 상태에 따라 매번 실행합니다.
- `ContentAnnotations`만 요청하면 페이지의 `Contents`와 `Resources`를 읽지 않습니다. 페이지 트리와 페이지 영역, 주석은 계속 읽습니다.
- 실행하는 콘텐츠는 순차 구문 커서로 읽습니다. 전체 콘텐츠의 토큰 배열을 먼저 만들지 않으며 `Provenance`를 끄면 명령 목록도 보관하지 않습니다.

결과 크기, 불필요한 할당과 파싱 후 보관량을 줄이는 방식입니다. 호출 중에는 원본 전체 스냅샷과 읽은 객체, 디코딩한 스트림 캐시가 여전히 메모리에 존재합니다. 파일을 일정 크기만큼씩 처리하는 스트리밍 I/O나 전체 프로세스 메모리 상한을 보장하지 않습니다. 제한별 적용 범위는 [리소스 제한](resource-limits.md)을 참고하세요.

## 오류와 완전성

옵션 오류와 입력 구조를 읽는 단계의 오류는 `nil` 결과와 오류를 반환합니다. 페이지 콘텐츠나 리소스 해석 도중 실패하면 부분 `Result`와 오류가 함께 반환될 수 있습니다. `result != nil`이나 `page.Complete`만으로 성공을 판단하지 말고 항상 `err`부터 확인합니다.

`DecodeComplete`는 문자 해석, `Position.Complete`는 요청한 위치 계산, `page.Complete`와 `Diagnostics`는 요청한 해석의 지원 범위를 나타냅니다. `Diagnostics[i].Page == -1`은 문서 전체 진단입니다. 지원하지 않는 효과가 오류 대신 진단으로 반환될 수 있습니다. 암호화 콘텐츠와 인라인 이미지도 여전히 지원이 제한됩니다.

선택하지 않은 리소스의 유효성이나 건너뛴 연산의 의미는 검증하지 않을 수 있습니다. 선택 추출의 성공은 PDF 전체 유효성 검사를 통과했다는 뜻이 아닙니다. 반면 순회하는 콘텐츠의 구문, 명령·피연산자 수, 필요한 참조와 디코딩에는 제한이 계속 적용됩니다. 같은 Form을 반복 실행한 작업도 누적 계산하며, 한도 초과는 `errors.Is(err, gopd.ErrLimit)`로 확인합니다.

반환값은 읽기 전용으로 취급합니다. 결과에 `Close()`를 호출할 필요는 없습니다. `Provenance`로 보관한 `Document`의 지연 메서드는 동시 호출을 지원하지 않습니다.

합성 문서의 반복 벤치마크와 측정 범위는 [할당량·JSON 측정 결과](selective-extraction-benchmarks.md)를 참고하세요.
