![GoPD](docs/readme/GoPD-logo-withtext.png)

**Go 네이티브로 작성된 파워풀한 PDF 파서**

[English](README.md) | 한국어

## 제작 배경

Go 생태계에서는 MuPDF처럼 강력한 오픈소스 PDF 파서를 찾기 어려웠습니다. 다른 언어로 작성된 DLL을 불러와 사용하는 프로젝트가 많았고, 저는 이런 방식이 아름답지 않다고 생각했습니다.

Go 네이티브로 작성된 프로젝트 중에는 상업적으로 사용되는 프로젝트도 있었습니다. 하지만 저는 그 점이 마음에 들지 않았습니다.

이 프로젝트는 MuPDF를 참고했습니다.

## 필요한 콘텐츠만 추출하기

`ParseFile`은 요청한 종류의 결과만 생성합니다. 빈 옵션은 Unicode 텍스트와 기본 글꼴 정보를 페이지별로 반환합니다.

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
            fmt.Println(page.Index+1, text.Unicode)
        }
    }
}
```

`ContentText`, `ContentGraphics`, `ContentImages`, `ContentAnnotations`를 `|`로 조합하거나 `ContentAll`을 선택합니다. `Positions`, `Styles`, `Glyphs`, `Provenance`로 상세 정보를 선택합니다. `Glyphs`는 텍스트 선택이 필요하며 위치 계산도 활성화합니다. `ParseReader`는 `io.ReaderAt`과 입력 크기를 받습니다.

선택한 종류는 콘텐츠 순회기를 공유합니다. 미선택 결과는 생성하지 않으며 `Provenance`를 켰을 때만 결과가 원본 스냅샷을 보관합니다. 호출 중에는 입력과 디코딩 캐시가 메모리에 남습니다. 스트리밍 파일 읽기나 건너뛴 콘텐츠의 전체 유효성 검사를 보장하지 않습니다.

기존 `ParsePDF`와 상세 API도 유지됩니다. `ParsePDF`는 `Details()`에 제공할 상세 결과를 계속 보관합니다. 옵션과 사용 예시는 [선택 추출 안내](docs/selective-extraction.md), [기본 API 안내](docs/basic-pdf.md)를 참고하세요.

## 지원 기능

GoPD는 PDF에 담긴 콘텐츠와 그 콘텐츠를 구성하는 내부 구조까지 정밀하게 해석하는 것을 목표로 합니다.

- **텍스트** — 문자열뿐 아니라 글자 단위의 문자 코드, Unicode 매핑, 글꼴, 크기, 위치와 변환 정보를 분석합니다.
- **벡터 그래픽** — 선과 베지어 곡선으로 구성된 경로, 윤곽선과 채우기, 색상, 선 두께, 점선과 클리핑 정보를 분석합니다.
- **이미지** — 이미지의 원본 스트림, 해상도, 색 공간, 마스크 여부와 페이지에 배치되는 위치·크기·회전을 분석합니다.
- **콘텐츠 실행 구조** — 텍스트·그래픽·이미지의 그리기 순서와 상태 변화, 중첩된 Form XObject의 호출 관계를 추적합니다.
- **파일 내부 구조** — PDF 객체와 간접 참조, 압축된 객체 스트림, xref 테이블·스트림과 증분 갱신 연결 구조를 분석합니다.
- **원본과의 연결** — 해석 결과에서 해당 요소를 만든 명령과 객체, 원본 또는 디코딩된 데이터의 바이트 범위까지 추적할 수 있도록 합니다.

현재는 이 목표의 일부를 구현한 단계이며, 복잡한 색 공간·투명도 효과, 인라인 이미지, 암호화된 콘텐츠 등의 해석에는 제한이 있습니다.

현재 처리 범위와 제한 설정은 [리소스 제한 문서](docs/resource-limits.md)를 참고하세요.
[기여 안내](CONTRIBUTING.md)와 [공개 전 점검 항목](docs/release-checklist.md)도 제공합니다.

## 프로젝트 구조

루트 패키지는 공개 API와 타입 별칭을 제공합니다. 콘텐츠 해석·선택 추출·결과 모델은 `internal/parser`에, 공통 PDF 모델·구문 분석·문서 읽기·테스트 입력 생성기는 `internal/common`의 하위 패키지에 있습니다. 파일별 역할과 의존 방향은 [프로젝트 구조 안내](docs/project-structure.md)를 참고하세요.
