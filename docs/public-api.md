# 라이브러리 공개 기능과 API

외부에서 사용할 기능과 그 기능에 대응하는 API 목록입니다. 현재 공개 함수는 **12개**, 공개 타입의 메서드는 **12개**, 공개 타입은 **84개**입니다. 타입에는 기본 결과·상세 결과·PDF 구문과 기존 호환성용 선언이 모두 포함됩니다. 이번 정리에서는 이름·시그니처·필드·상수와 반환 동작을 유지합니다.

모듈 경로는 `github.com/MyungSub0519/gopd`, 패키지 이름은 `gopd`입니다.

```go
import "github.com/MyungSub0519/gopd"
```

아래 시그니처는 패키지 내부 선언과 동일하게 `gopd.` 접두어를 생략했습니다. 외부에서는 `gopd.ParsePDF`, `gopd.ReadOptions`, `gopd.ObjectID`처럼 사용합니다.

## 외부에 제공할 기능 목록

| 구분 | 제공할 기능 | API | 주요 결과 |
| --- | --- | --- | --- |
| 기본 파싱 | 파일에서 페이지별 텍스트·벡터 그래픽 추출 | `ParsePDF` | `PDF` |
| 상세 결과 접근 | 같은 파싱의 상세 결과 확인 | `PDF.Details` | `DetailedPDF` |
| 상세 콘텐츠 분석 | 파일·ReaderAt·저수준 문서에서 상세 콘텐츠 해석 | `Open`, `Read`, `BuildPDF` | `DetailedPDF` |
| 저수준 문서 입력 | 입력 크기·구문 깊이 등의 제한을 적용하여 문서 열기 | `ParseFile`, `Parse` | `Document` |
| 객체 탐색 | Catalog·간접 객체 조회 및 참조 해석 | `Document.Catalog`, `Load`, `Resolve`, `ResolveObject` | `Object`, `IndirectObject` |
| 원본·스트림 접근 | 구문 바이트 읽기, 스트림 디코딩, 출처 추적 | `Document.Bytes`, `RawObject`, `DecodeStream` | 바이트, `Source` |
| 독립 구문 분석 | 별도 바이트 범위의 토큰·PDF 객체 분석 | `Lex`, `ParseObject` | `Token`, `Object` |
| 값 조회·변환 | 사전 조회, 숫자 변환, 스트림 여부 확인 | `Dictionary.Get`, `GetAll`, `Int`, `Number`, `IsStream` | 조회 값과 오류 |
| 좌표 계산 | 항등 행렬, 점 변환, 변환 합성 | `IdentityMatrix`, `Matrix.Transform`, `Mul` | `Matrix`, `Point` |

일반 사용자는 `ParsePDF`부터 시작합니다. 글리프·이미지·원문 출처 등이 필요하면 `Details()`를 사용합니다. 입력 제한을 직접 설정하거나 객체를 탐색하는 도구는 `ParseFile` 또는 `Parse`부터 시작합니다. 위 기능은 모두 기존 단일 `gopd` 패키지에서 제공합니다.

이미지·주석은 현재 상세 결과에 포함됩니다. 픽셀 렌더링, OCR, 편집·저장, 암호화 해제, 사람의 읽기 순서 복원, 선택적 페이지·콘텐츠 파싱은 현재 공개 기능에 포함하지 않습니다. 지원하지 않는 효과와 입력은 오류 또는 진단으로 확인합니다.

## 기본 진입점

파일 경로 하나로 기본 결과가 필요하면 `gopd.ParsePDF`를 호출합니다.

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
    }
    fmt.Println("content diagnostics:", len(doc.Details().Diagnostics))
}
```

반환된 `*PDF`의 공개 필드는 `Texts [][]Text`, `Graphics [][]Graphic` 두 개입니다. 바깥 배열은 페이지 순서이며 요소가 없는 페이지는 빈 배열 `[]`로 유지합니다. 페이지 정보·이미지·진단을 포함한 상세 결과는 `doc.Details()`로 접근합니다. 반환 필드와 JSON 사용법은 [기본 PDF API](basic-pdf.md)를 참고하세요.

CLI의 [main.go](../examples/gopd/main.go)에 있는 `pdfparse()`는 비공개 보조 함수이며 내부에서 `gopd.ParsePDF()`를 호출합니다. 외부 라이브러리 사용자는 `gopd.ParsePDF()`를 사용합니다.

## 1. PDF 파싱 함수 — 6개

| 시그니처 | 용도 |
| --- | --- |
| `func ParsePDF(path string) (*PDF, error)` | 파일 경로로 텍스트와 그래픽을 페이지별로 모은 기본 구조체 반환. 일반 사용자의 진입점 |
| `func Open(path string) (*DetailedPDF, error)` | 파일 경로로 페이지 콘텐츠·리소스·명령·출처를 포함한 상세 결과 반환 |
| `func Read(r io.ReaderAt, size int64) (*DetailedPDF, error)` | ReaderAt과 전체 바이트 크기로 상세 분석 |
| `func ParseFile(path string, options ...ReadOptions) (*Document, error)` | 파일의 저수준 구문·xref 분석과 객체 접근 준비 |
| `func Parse(r io.ReaderAt, size int64, options ...ReadOptions) (*Document, error)` | ReaderAt 입력의 저수준 분석과 객체 접근 준비 |
| `func BuildPDF(d *Document) (*DetailedPDF, error)` | 저수준 Document의 페이지 트리와 콘텐츠를 해석해 상세 결과 생성 |

공개 진입점은 [api.go](../api.go)에 모여 있습니다. 파일·ReaderAt 읽기는 [internal/document/read.go](../internal/document/read.go), 콘텐츠 해석은 [content.go](../content.go), 기본 결과 변환은 [basic.go](../basic.go)에 있습니다.

`Parse`와 `ParseFile`의 `options`는 생략하거나 하나 전달할 수 있습니다. `ReadOptions.MaxFileBytes`와 `ReadOptions.Limits`로 입력 크기·구문 깊이·토큰 크기·객체 수·xref 섹션 수·디코딩 데이터 제한을 설정합니다. 각 필드의 0은 기본값을 사용하고 음수는 오류입니다.

`ParsePDF`, `Open`, `Read`에는 옵션 인자가 없습니다. 제한을 조정하며 콘텐츠를 해석하려면 `ParseFile` 또는 `Parse`로 문서를 얻고, 오류를 확인한 뒤 `BuildPDF`를 호출합니다.

`MaxValues`, `MaxContentBytes`, `MaxSemanticObjects`, `MaxRegionWork`는 각각 누적 직접 값, 반복 실행 콘텐츠 바이트, 의미 분석 방문 횟수, 파일 구간 관리 작업량을 제한합니다. 기본값과 계산 범위는 [리소스 제한](resource-limits.md)을 참고하세요. 리소스 제한 오류는 `errors.Is(err, gopd.ErrLimit)`로 구분합니다.

## 2. 구문 분석 함수 — 2개

| 시그니처 | 용도 |
| --- | --- |
| `func Lex(data []byte, source SourceID, offset int64) ([]Token, error)` | 바이트를 PDF 토큰으로 분리하고 공백·주석·바이트 위치 보존 |
| `func ParseObject(data []byte, source SourceID, offset int64) (object Object, consumed int, err error)` | 객체 하나를 파싱하고 입력에서 소비한 바이트 수 반환 |

`source`는 입력이 속한 소스의 식별자이고, `offset`은 그 소스에서 `data[0]`의 바이트 위치입니다. 이 함수들은 전달받은 바이트를 분석하며 Document에 소스를 등록하지 않습니다.

진입점: [api.go](../api.go). 실제 스캐너와 객체 parser는 [internal/syntax/lexer.go](../internal/syntax/lexer.go), [internal/syntax/parser.go](../internal/syntax/parser.go)에 있습니다.

## 3. 값 확인·변환 함수 — 3개

| 시그니처 | 용도 |
| --- | --- |
| `func Int(object Object) (int64, error)` | PDF Integer를 int64로 변환. 타입 불일치·범위 초과는 오류 |
| `func Number(object Object) (float64, error)` | PDF Integer 또는 Real을 유한한 float64로 변환 |
| `func IsStream(object Object) bool` | 객체의 값이 Stream인지 확인 |

이 함수들은 간접 참조를 자동으로 해석하지 않습니다. 필요한 경우 먼저 `Document.ResolveObject`를 호출합니다.

공개 함수: [api.go](../api.go). 사전 메서드와 변환 구현: [internal/pdfmodel/object.go](../internal/pdfmodel/object.go).

## 4. 좌표 변환 함수 — 1개

| 시그니처 | 용도 |
| --- | --- |
| `func IdentityMatrix() Matrix` | 좌표를 변경하지 않는 항등 행렬 `[1 0 0 1 0 0]` 생성 |

공개 함수는 [api.go](../api.go), 타입 별칭은 [types.go](../types.go), 계산 구현은 [internal/pdfmodel/geometry.go](../internal/pdfmodel/geometry.go)에 있습니다.

## 5. PDF 메서드 — 1개

| 시그니처 | 용도 |
| --- | --- |
| `func (p *PDF) Details() *DetailedPDF` | 같은 파싱에서 생성해 보관한 상세 결과 반환. 수신자가 nil이면 nil 반환 |

`Details()`는 파일을 다시 읽지 않습니다. 기본 결과의 Texts·Graphics는 페이지별 이중 배열이며 상세 결과는 문서 전체의 평면 배열입니다. 특정 페이지의 상세 요소는 `detail.Pages[page].Items`의 Kind와 Index로 찾습니다. 기본 배열의 안쪽 인덱스를 상세 배열의 전역 인덱스로 사용할 수는 없습니다.

구현: [basic.go](../basic.go).

## 6. Document 메서드 — 7개

`Document`는 `ParseFile`·`Parse`의 반환값 또는 `doc.Details().Document`로 접근합니다.

| 시그니처 | 용도 |
| --- | --- |
| `func (d *Document) Catalog() (Object, error)` | trailer의 Root 참조를 해석해 문서 Catalog 반환 |
| `func (d *Document) Load(id ObjectID) (*IndirectObject, error)` | xref를 따라 객체 번호·세대에 해당하는 간접 객체 로딩 |
| `func (d *Document) Resolve(ref Reference) (Object, error)` | 간접 참조 한 단계에 해당하는 객체 본문 반환 |
| `func (d *Document) ResolveObject(object Object) (Object, error)` | 참조 체인을 따라 최종 객체 반환. 순환·깊이 제한 초과는 오류 |
| `func (d *Document) DecodeStream(stream Stream) (Source, error)` | 지원하는 필터를 적용해 스트림을 디코딩하고 결과 소스 반환 |
| `func (d *Document) Bytes(span Span) ([]byte, error)` | 지정 소스의 바이트 범위를 읽어 반환 |
| `func (d *Document) RawObject(object Object) ([]byte, error)` | 객체의 Span에 해당하는 구문 바이트 반환 |

`Span`은 `Source`와 바이트 범위 `[Start, End)`로 구성됩니다. `Bytes`와 `RawObject`는 원본 파일 소스에서는 파일 바이트를, 디코딩 소스에서는 디코딩된 바이트를 반환합니다. 디코딩 소스의 `Origin.Input`으로 변환 전 범위를 추적할 수 있으며, 압축 전후 바이트가 일대일로 대응한다는 의미는 아닙니다.

여러 `/Contents` 스트림을 연결한 소스는 `Origin.Inputs`에 입력 범위를 순서대로 기록합니다. 해당 소스의 Span은 연결된 바이트에 대한 위치입니다. 단일 필터 변환의 `Origin.Input`과 구분해서 추적합니다.

`Document`의 공개 별칭은 [types.go](../types.go)에 있습니다. 메서드 구현은 [internal/document/document.go](../internal/document/document.go), [objects.go](../internal/document/objects.go), [filters.go](../internal/document/filters.go)에 있습니다.

## 7. Dictionary 메서드 — 2개

| 시그니처 | 용도 |
| --- | --- |
| `func (d Dictionary) Get(key Name) (Object, error)` | 키 하나의 값 조회. 키 누락 또는 중복은 오류 |
| `func (d Dictionary) GetAll(key Name) []Object` | 같은 키의 모든 값을 저장 순서대로 반환. 일치하는 키가 없으면 nil 슬라이스 반환 |

`Name` 키에는 앞의 `/`를 붙이지 않습니다. 예를 들어 PDF의 `/Pages`는 `dictionary.Get("Pages")`로 조회합니다. `Get`과 `GetAll`은 저장된 값을 반환하며 간접 참조를 자동으로 해석하지 않습니다.

키 누락은 공개 오류 변수 `gopd.ErrMissingKey`를 감싼 오류로 반환하므로 `errors.Is(err, gopd.ErrMissingKey)`로 확인할 수 있습니다. 중복 키 오류와는 구분됩니다.

공개 함수: [api.go](../api.go). 사전 메서드와 변환 구현: [internal/pdfmodel/object.go](../internal/pdfmodel/object.go).

## 8. Matrix 메서드 — 2개

| 시그니처 | 용도 |
| --- | --- |
| `func (m Matrix) Transform(p Point) Point` | 점에 좌표 변환 적용 |
| `func (m Matrix) Mul(n Matrix) Matrix` | 두 변환을 합성한 행렬 반환 |

`Matrix`는 `[a b c d e f]`이며 점 `(x, y)`를 `(a*x+c*y+e, b*x+d*y+f)`로 변환합니다.

`m.Mul(n).Transform(p)`는 `m.Transform(n.Transform(p))`와 같습니다. 즉 `n`을 먼저 적용하고 `m`을 적용합니다. 페이지의 `Rotate`와 `UserUnit`을 자동으로 적용하는 함수는 아닙니다.

공개 함수는 [api.go](../api.go), 타입 별칭은 [types.go](../types.go), 계산 구현은 [internal/pdfmodel/geometry.go](../internal/pdfmodel/geometry.go)에 있습니다.

## 공개 타입의 역할

반환값에 연결된 타입도 외부 API의 일부입니다. 다음 분류는 기존 선언을 모두 유지하면서 사용 목적을 구분한 것입니다.

| 역할 | 공개 타입 | 코드 |
| --- | --- | --- |
| 기본 결과 | `PDF`, `Text`, `Graphic`, `PathSegment` | [basic.go](../basic.go) |
| 상세 콘텐츠 | `DetailedPDF`, `DetailedPage`, `DetailedText`, `DetailedGraphic`, `DetailedImage`, `DetailedPathSegment`, `Glyph`, `ImageResource`, `Annotation` | [detailed_model.go](../detailed_model.go) |
| 실행 순서·출처 | `ElementKind`, `ElementRef`, `FormCall`, `ElementSource`, `Operation` | [detailed_model.go](../detailed_model.go) |
| 글꼴 | `FontInfo`, `Font`, `CodeSpace`, `CMap` | [basic.go](../basic.go), [fonts.go](../fonts.go), [cmap.go](../cmap.go) |
| 스타일 | `Color`, `PaintStyle`, `GraphicsState`, `ClipPath` | [style_types.go](../style_types.go) |
| 좌표 | `Point`, `Rect`, `Matrix` | [types.go](../types.go), [geometry.go](../internal/pdfmodel/geometry.go) |
| 문서·입력 제한 | `Document`, `ReadOptions`, `Limits` | [document.go](../internal/document/document.go), [read.go](../internal/document/read.go), [structure.go](../internal/pdfmodel/structure.go) |
| PDF 값 | `Value`, `Object`, `Null`, `Boolean`, `Integer`, `Real`, `Name`, `StringForm`, `PDFString`, `Array`, `DictionaryEntry`, `Dictionary`, `ObjectID`, `Reference`, `InvalidValue` | [object.go](../internal/pdfmodel/object.go) |
| 간접 객체 | `ObjectOrigin`, `FileObjectOrigin`, `ObjectStreamOrigin`, `IndirectObject` | [object.go](../internal/pdfmodel/object.go) |
| 스트림 | `StreamBoundary`, `Stream` | [object.go](../internal/pdfmodel/object.go) |
| 바이트 출처 | `SourceID`, `Position`, `Span`, `Source`, `Derivation`, `TransformKind`, `Transform` | [source.go](../internal/pdfmodel/source.go) |
| xref | `XRefEntry`, `FreeEntry`, `InUseEntry`, `CompressedEntry`, `UnknownXRefEntry`, `XRefRecord`, `XRefRange`, `XRefForm`, `SectionID`, `XRefSection` | [structure.go](../internal/pdfmodel/structure.go) |
| 파일 구조·진단 | `Version`, `Header`, `FileTail`, `RegionKind`, `FileRegion`, `Severity`, `Diagnostic`, `Structure` | [structure.go](../internal/pdfmodel/structure.go) |
| 토큰 | `TokenKind`, `Token` | [token.go](../internal/pdfmodel/token.go) |

`ElementKind`, `TokenKind`, `Severity` 등의 기존 상수와 `ErrMissingKey`도 유지합니다. 모든 타입이 완전한 PDF 규격 지원을 의미하지는 않습니다. 예를 들어 복구·복호화 등을 표현하는 타입 값이 정의되어 있어도 해당 처리가 구현되지 않은 경우가 있습니다.

## 코드 구성 기준

```text
api.go                공개 파싱·구문·값 변환 함수
types.go              내부 타입·상수·오류의 공개 별칭
basic.go              기본 결과·FontInfo·Details()·기본 결과 변환
detailed_model.go     상세 반환 타입
style_types.go        스타일·그래픽 상태
semantic*.go          콘텐츠 해석 세션·공통 조회·진단·자원 예산
pages.go              페이지 트리·상속·콘텐츠 연결·주석
content*.go           콘텐츠 구문·명령 실행·리소스·Form/Image·ExtGState
graphics_state.go     공통 선·점선 상태 검증과 설정
text.go               텍스트 실행·글리프·위치 계산
fonts.go, cmap.go     글꼴·CMap 타입과 해석
internal/document/   파일·바이트 범위·객체·xref·스트림 읽기
internal/syntax/     토큰·객체 구문 해석
internal/pdfmodel/   공통 모델·사전 조회·값 변환·좌표 계산
internal/pdftest/    테스트용 PDF·스트림 생성기
examples/gopd/      CLI 인수 처리와 표시
```

공개 함수는 `api.go`, 별칭은 `types.go`에서 찾습니다. 기본 응답과 변환은 `basic.go`에서 함께 읽을 수 있습니다. 파일 읽기·구문 분석 구현은 내부 패키지에 두며, CLI는 라이브러리를 호출합니다. 패키지 경계, 타입 이동의 호환성 범위, 테스트 배치는 [프로젝트 구조](project-structure.md)를 참고하세요.

## 결과 수명과 오류 처리

- 파일 입력은 메모리 스냅샷으로 보관합니다. `ParseFile`과 `Open`은 자신이 연 파일을 닫으며 `ParsePDF`도 이 경로를 사용합니다. 반환 결과에 `Close()`를 호출할 필요는 없습니다.
- `Parse`와 `Read`는 호출자가 전달한 ReaderAt을 닫지 않습니다.
- `ParsePDF`는 상세 분석까지 수행한 뒤 기본 결과를 생성하고 상세 결과도 유지합니다. 선택적 파싱이나 메모리 절약 모드는 아닙니다.
- 파싱·콘텐츠 해석 실패 시 부분 결과와 오류가 함께 반환될 수 있으므로, 결과가 nil이 아니어도 반드시 오류를 확인합니다.
- 미지원 효과는 오류 없이 진단으로 남을 수도 있습니다. `doc.Details().Diagnostics`, `doc.Details().Structure.Diagnostics`, 상세 페이지의 `Complete`와 기본 텍스트의 `DecodeComplete`, `PositionComplete`로 해석 상태를 확인합니다.
- 반환 구조체·슬라이스·공유 리소스는 읽기 전용으로 취급합니다. 같은 Document의 지연 파싱·디코딩 메서드는 동시 호출을 지원하지 않습니다.

## 목록 확인과 관련 문서

저장소 루트에서 현재 코드의 공개 선언을 확인합니다.

```powershell
go doc -all .
```

공개 함수나 메서드를 추가·변경할 때 이 문서의 목록과 개수도 함께 갱신합니다. [외부 패키지 테스트](../public_api_test.go)는 `gopd_test` 패키지에서 공개 API만 사용해 기본·상세·저수준 진입점을 검증합니다.

- [기본 PDF API와 JSON 저장](basic-pdf.md)
- [상세 JSON 구조](json-structure.md)
- [프로젝트 README](../README.md)
