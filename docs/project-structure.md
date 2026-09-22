# 프로젝트 구조

현재 폴더와 각 파일의 역할을 찾기 위한 안내서입니다. 루트 `gopd`는 공개 API와 타입 별칭을 제공하고, `internal/parser`는 콘텐츠 해석과 결과 생성을, `internal/common`의 하위 패키지는 파일 읽기·구문 분석·공통 PDF 모델을 담당합니다. 아래 트리는 주요 소스 파일과 테스트 자료를 표시합니다.

## 구성 원칙

- 패키지는 독립적인 책임과 의존 방향이 있을 때 나눕니다. 짧은 코드마다 별도 패키지를 만들지 않습니다.
- 공개 진입점과 타입 별칭은 각각 한곳에서 찾을 수 있게 모읍니다.
- 타입과 그 타입을 다루는 코드는 함께 둡니다. 객체·스트림·사전 조회는 같은 객체 모델에 속합니다.
- 파일 읽기와 구문 분석의 내부 경계는 유지합니다. 이후의 분리는 실제 의존 관계와 변경 빈도를 보고 결정합니다.

## 현재 구성

```text
gopd/
├── go.mod                         모듈 경로와 Go 버전
├── doc.go                         공개 패키지 문서
├── api.go                         공개 함수와 내부 구현 연결
├── types.go                       타입·상수·오류의 공개 별칭
├── public_api_test.go             외부 공개 API 테스트와 선택 추출 예제
├── internal/
│   ├── common/
│   │   ├── pdfmodel/              PDF 값·좌표·출처·파일 구조 모델
│   │   ├── syntax/                토큰·객체·콘텐츠 명령 구문 분석
│   │   ├── document/              파일·객체·xref·스트림 읽기
│   │   └── pdftest/               테스트용 PDF·스트림 생성
│   └── parser/
│       ├── parser.go              파싱 진입점·문서 결과·해석 세션·예산
│       ├── page.go                페이지 결과·트리·상속·콘텐츠 연결
│       ├── text.go                텍스트 결과·명령·글리프·위치 계산
│       ├── graphic.go             그래픽 결과·경로·그리기·클리핑
│       ├── image.go               이미지 결과·리소스·출력
│       ├── annotation.go          주석 결과·해석·출력
│       ├── interpreter.go         공통 상태·콘텐츠 구문 분석·명령 분배·출처
│       ├── resource.go            리소스 조회·XObject 분배·Form·ExtGState
│       ├── font.go                글꼴 타입과 리소스 해석
│       ├── cmap.go                CMap·CodeSpace 타입과 문자 매핑
│       ├── style.go               스타일 타입·기본 및 선택 추출용 변환
│       ├── basic.go               PDF·Details()·기본 결과 변환
│       ├── extract.go             선택 추출 실행·옵션·문서 결과·진단
│       ├── types.go               공통 PDF 타입·상수·오류의 내부 별칭
│       └── *_test.go              구현 단위·회귀·통합·퍼즈·벤치마크
├── examples/gopd/                 CLI와 CLI 테스트
├── testdata/                      합성 PDF·생성기·출처 안내
├── README.md                      영문 프로젝트 소개
├── README.ko.md                   한국어 프로젝트 소개
└── docs/                          API·구조·제한·사용법 안내
```

`doc.go`는 각 패키지의 문서 설명입니다. 테스트 준비 함수는 사용하는 도메인의 테스트 파일에 함께 둡니다.

## 구현 파일별 역할

### 루트 공개 API와 internal/parser 콘텐츠 해석

루트의 구현 파일은 `api.go`, `types.go`, `doc.go`로 제한하고 외부 호출을 내부 구현에 연결합니다. 해석 구현과 결과 모델은 `internal/parser`에 함께 두어 콘텐츠 상태와 글꼴 정보를 공유합니다. `common`은 단일 Go 패키지가 아니라 공통 기반 패키지를 모은 폴더입니다.

| 파일 | 주요 타입·함수 | 작성된 기능 |
| --- | --- | --- |
| [api.go](../api.go) | `ParseFile`, `ParseReader`, `ParsePDF`, `Open`, `Read`, `BuildPDF`, `LoadDocument`, `ReadDocument`, `Lex`, `ParseObject` 등 | 외부 호출의 진입점입니다. 콘텐츠 분석은 `internal/parser`에, 파일 읽기·구문 분석은 `internal/common`의 하위 패키지에 위임합니다. 값 변환·단위 행렬 함수도 여기에서 노출합니다. |
| [types.go](../types.go) | `Document`, `ReadOptions`, `Object`, `Span`, `Matrix` 등의 별칭 | 내부에 정의된 타입·상수·오류를 루트 API 이름으로 노출합니다. 타입을 새로 감싸거나 데이터를 복사하는 코드가 아니라, 외부 사용자가 `gopd.Document`처럼 접근하도록 연결하는 코드입니다. |
| [parser.go](../internal/parser/parser.go) | `ParsePDF`, `Open`, `Read`, `BuildPDF`, `DetailedPDF`, `ElementKind`, `ElementRef`, `semanticBuilder` | 기본·상세 파싱 진입점과 문서 전체 결과를 둡니다. 해석 세션의 캐시·출력·예산, 사전 조회와 진단 생성, 반복 리소스 작업 및 출력 크기 제한을 담당합니다. |
| [page.go](../internal/parser/page.go) | `DetailedPage`, `ExtractedPage`, `walkPages`, `interpretPage` | 페이지 결과와 페이지 트리·상속 속성 해석, 콘텐츠 연결·실행을 함께 둡니다. 상세 페이지의 `Items`는 텍스트·그래픽·이미지가 섞인 실행 순서를 보존합니다. |
| [text.go](../internal/parser/text.go) | `Text`, `DetailedText`, `ExtractedText`, `Glyph`, `TextPosition`, `executeText`, `moveText`, `showText`, `emitText` | 기본·상세·선택 추출의 텍스트 타입과 텍스트 명령 실행·결과 저장을 함께 둡니다. 문자 표시에서 텍스트·글리프·이동량을 만들고 텍스트 위치를 갱신합니다. |
| [graphic.go](../internal/parser/graphic.go) | `Graphic`, `DetailedGraphic`, `ExtractedGraphic`, `PathSegment`, `DetailedPathSegment`, `executeGraphic`, `emitGraphic` | 그래픽 타입과 경로·그리기·클리핑·출력을 담당합니다. 콘텐츠 명령과 ExtGState가 같은 선·점선 검증 함수를 사용하며, 리소스를 가짜 명령으로 다시 실행하지 않습니다. |
| [image.go](../internal/parser/image.go) | `DetailedImage`, `ImageResource`, `ExtractedImage`, `ExtractedImageResource`, `emitImage`, `emitImageResource` | 이미지 사용과 공유 리소스의 타입, Image XObject 해석 및 결과 저장을 함께 둡니다. |
| [annotation.go](../internal/parser/annotation.go) | `Annotation`, `ExtractedAnnotation`, `readAnnotations`, `emitAnnotation` | 주석 타입과 페이지 주석 해석·결과 저장을 함께 둡니다. |
| [interpreter.go](../internal/parser/interpreter.go) | `contentInterpreter`, `contentState`, `textState`, `interpretSource`, `execute`, `Operation`, `ElementSource`, `FormCall` | 콘텐츠 구문 분석, 공통 상태와 명령 분배를 담당합니다. 실행 명령 및 결과의 페이지·명령·바이트 출처 타입도 정의합니다. |
| [resource.go](../internal/parser/resource.go) | `resource`, `xobject`, `extGState` | 리소스 조회, XObject 종류별 분배, Form 실행, ExtGState의 참조 해석과 상태 적용을 담당합니다. 이미지 처리는 `image.go`로 위임하며 반복 딕셔너리 검사와 숫자 배열 확장 전에 예산을 확인합니다. |
| [font.go](../internal/parser/font.go) | `FontInfo`, `Font`, `font`, `cidWidths`, `simpleFontEncoding`, `decodeBounded` | 기본·상세 글꼴 타입과 글꼴 리소스의 종류·이름·인코딩·문자 폭·ToUnicode 정보를 함께 둡니다. 문자 코드를 해석하고 글리프 위치 계산에 필요한 폭 정보를 제공합니다. |
| [cmap.go](../internal/parser/cmap.go) | `CodeSpace`, `CMap`, `parseToUnicode`, `decodeBounded` | ToUnicode CMap을 읽어 PDF 글꼴의 문자 코드와 Unicode 문자열을 연결합니다. 코드 길이와 매핑 범위를 처리하고 디코딩 결과의 완전성 및 출력 크기 제한을 관리합니다. |
| [style.go](../internal/parser/style.go) | `Color`, `PaintStyle`, `GraphicsState`, `ClipPath`, `basicStyle`, `extractStyle` | 색상, 선 두께, 점선, 투명도, 혼합 모드와 클리핑 정보를 정의하고 기본·선택 추출용 스타일로 변환합니다. |
| [basic.go](../internal/parser/basic.go) | `PDF`, `Details`, `basicPDF` | 상세 결과를 기본 결과의 페이지별 텍스트·그래픽 배열로 변환합니다. `Details()`는 저장해 둔 상세 결과를 반환합니다. |
| [extract.go](../internal/parser/extract.go) | `ParseFile`, `ParseReader`, `ContentKind`, `ParseOptions`, `Result`, `ResultDiagnostic` | 선택 파싱의 옵션·문서 결과·진단과 실행 코드를 둡니다. 요소별 결과 타입과 출력 코드는 각 콘텐츠 파일에 있습니다. 루트 `api.go`와 `types.go`가 공개 이름을 제공합니다. |
| [doc.go](../doc.go) | `gopd` 패키지 문서 | 기본·상세·저수준 API의 사용 단계, 메모리 소유권, 동시 호출 제약, 좌표와 실행 순서의 의미를 설명합니다. |

`parser.go`는 문서 해석 세션을, `page.go`는 페이지와 콘텐츠 선택을 관리합니다. `interpreter.go`는 구문 분석과 명령 분배를 연결하고, `text.go`·`graphic.go`·`image.go`·`annotation.go`는 각 콘텐츠의 타입·해석·결과 저장을 함께 둡니다. `resource.go`는 리소스를 통한 실행을 담당합니다. 파일은 나누되 같은 상태와 예산을 공유하는 하나의 `internal/parser` Go 패키지를 유지합니다. `font.go`는 글꼴 리소스 전체를, `cmap.go`는 문자 코드 매핑을 담당합니다.

### internal/common/document: PDF 파일과 객체 읽기

PDF 파일의 물리적 구조를 다루는 패키지입니다. xref는 객체 번호로 파일 위치 또는 객체 스트림 위치를 찾는 색인입니다.

| 파일 | 주요 타입·함수 | 작성된 기능 |
| --- | --- | --- |
| [read.go](../internal/common/document/read.go) | `ReadOptions`, `normalizeOptions`, `Load`, `Read` | 입력·분석 제한값을 설정하고 파일 또는 `io.ReaderAt`에서 전체 바이트 스냅샷을 읽습니다. `Document`를 초기화한 뒤 헤더와 xref 분석을 시작합니다. |
| [document.go](../internal/common/document/document.go) | `Document`, `Bytes`, `RawObject`, `Catalog`, `Resolve`, `ResolveObject`, `parseObject` | 원본·디코딩 소스와 객체 캐시 등 문서 상태를 보관합니다. 바이트 범위 조회, 문서 최상위 카탈로그 조회, 간접 참조 해석과 참조 순환 검사를 제공합니다. 트레일러와 객체를 읽을 때 누적 구문 값 예산을 적용합니다. |
| [objects.go](../internal/common/document/objects.go) | `Load`, `parseIndirect`, `loadCompressed`, `objectStreamIndex` | 객체 번호에 해당하는 간접 객체를 필요할 때 읽고 캐시합니다. `obj`·`endobj` 경계, 스트림 길이, 여러 객체를 담는 객체 스트림의 헤더 색인과 캐시를 처리합니다. |
| [xref.go](../internal/common/document/xref.go) | `readHeaderAndXRefs`, `readXRefChain`, `readXRefTable`, `readXRefStream` | 헤더와 파일 끝 정보를 읽고, 표 또는 스트림 형태의 xref를 분석합니다. 증분 저장 이력을 따라가 최신 객체 위치를 적용하며 삭제된 객체와 참조 순환도 처리합니다. |
| [filters.go](../internal/common/document/filters.go) | `DecodeStream`, `decodeFilter`, `applyPredictor` | 스트림에 적용된 압축·인코딩 필터와 예측자(차분으로 저장한 값을 복원하는 처리)를 해제합니다. 디코딩 크기 제한을 적용하고, 결과를 별도 소스와 출처 정보로 등록·캐시합니다. |
| [doc.go](../internal/common/document/doc.go) | `document` 패키지 문서 | 바이트 스냅샷, 범위 조회, 객체 조회, xref와 스트림 처리라는 패키지 책임을 설명합니다. |

### internal/common/syntax: 바이트를 PDF 문법으로 읽기

주어진 바이트 구간을 토큰과 PDF 값으로 바꾸는 패키지입니다. 파일을 열거나 간접 참조가 가리키는 객체를 로딩하는 일은 `document`가 담당합니다.

| 파일 | 주요 타입·함수 | 작성된 기능 |
| --- | --- | --- |
| [lexer.go](../internal/common/syntax/lexer.go) | `Lex`, `syntaxScanner` | 숫자, 이름, 문자열, 구분자, 공백, 주석 등의 토큰을 구분하고 소스의 바이트 위치를 보존합니다. 토큰 크기와 개수 제한도 검사합니다. |
| [parser.go](../internal/common/syntax/parser.go) | `ParseObject`, `ParseObjectWithLimits`, `objectParser` | 토큰을 숫자·문자열·배열·사전·간접 참조 등의 `Object`로 조립합니다. 이름 및 문자열의 이스케이프와 16진 표현을 해석하고, 중첩 깊이 등 제한을 검사합니다. |
| [doc.go](../internal/common/syntax/doc.go) | `syntax` 패키지 문서 | 독립된 바이트 범위의 토큰화와 객체 구문 분석이라는 책임을 설명합니다. |

예를 들어 `<< /Type /Page /Contents 12 0 R >>`를 사전과 참조 값으로 만드는 곳은 `syntax`, `12 0 R`이 가리키는 객체를 찾는 곳은 `document`, 그 내용에서 텍스트와 그래픽을 만드는 곳은 `internal/parser`입니다.

### internal/common/pdfmodel: 공통 PDF 타입과 연산

파일 읽기와 콘텐츠 해석이 함께 사용하는 PDF 표현입니다. 다른 프로젝트 내부 패키지에 의존하지 않습니다.

| 파일 | 주요 타입·함수 | 작성된 기능 |
| --- | --- | --- |
| [object.go](../internal/common/pdfmodel/object.go) | `Object`, `Value`, `Dictionary`, `Reference`, `Stream`, `IndirectObject`, `Int`, `Number` | PDF의 기본 값, 간접 객체와 스트림을 정의합니다. 사전 키 조회·중복 키 처리·숫자 변환·스트림 여부 확인도 함께 둡니다. |
| [source.go](../internal/common/pdfmodel/source.go) | `SourceID`, `Position`, `Span`, `Source`, `Derivation`, `Transform` | 원본 또는 디코딩된 데이터의 위치와 바이트 범위, 변환 출처를 표현합니다. `Span`의 범위는 `[Start, End)`입니다. |
| [structure.go](../internal/common/pdfmodel/structure.go) | `Structure`, `Header`, `FileTail`, `XRefSection`, `Diagnostic`, `Limits` 등 | 파일 헤더·끝부분·영역, xref 항목과 구간, 진단 및 분석 제한을 정의합니다. 실제 xref를 읽는 코드는 `document/xref.go`에 있습니다. |
| [token.go](../internal/common/pdfmodel/token.go) | `TokenKind`, `Token` | 구문 분석기가 반환하는 토큰의 종류와 바이트 범위를 정의합니다. |
| [geometry.go](../internal/common/pdfmodel/geometry.go) | `Point`, `Rect`, `Matrix`, `IdentityMatrix`, `Transform`, `Mul` | 점·사각형·좌표 변환 행렬과 행렬 합성·점 변환 연산을 제공합니다. |
| [doc.go](../internal/common/pdfmodel/doc.go) | `pdfmodel` 패키지 문서 | 공통 PDF 값, 소스 위치, 좌표와 구조 모델의 범위를 설명합니다. |

### 테스트 지원·CLI·설정

| 파일 | 작성된 기능 |
| --- | --- |
| [internal/common/pdftest/fixture.go](../internal/common/pdftest/fixture.go) | `File`과 `Stream`으로 테스트에 필요한 작은 PDF와 스트림을 만듭니다. 파서 구현에 의존하지 않아 입력 생성과 파싱 검증을 분리합니다. |
| [examples/gopd/main.go](../examples/gopd/main.go) | CLI 인자를 검사하고 `ParsePDF`를 호출합니다. 기본 통계, `-text` 텍스트, `-json` 통계를 출력하고 종료 코드를 결정합니다. `-json`은 전체 기본 응답이 아니라 개수 중심 요약 JSON입니다. |
| [go.mod](../go.mod) | 모듈 경로 `github.com/MyungSub0519/gopd`와 Go 버전 `1.25.0`을 선언합니다. |

각 테스트 파일의 담당 범위는 아래 **테스트** 절에 정리했습니다. [공개 API 목록](public-api.md)은 함수와 타입을, [기본 응답](basic-pdf.md)과 [JSON 구조](json-structure.md)는 반환값을 설명합니다. `docs/superpowers`는 작업 당시의 설계·계획 기록이므로 현재 파일 배치는 이 문서를 기준으로 확인합니다.

## 코드를 읽는 순서

1. [api.go](../api.go) → [internal/parser/extract.go](../internal/parser/extract.go): `ParseFile`·`ParseReader` 선택 파싱 진입점과 옵션·결과 생성 흐름. 기존 전체 상세 흐름은 [internal/parser/parser.go](../internal/parser/parser.go)의 `ParsePDF → Open → LoadDocument → BuildPDF`에서 이어집니다.
2. [internal/common/document/read.go](../internal/common/document/read.go): 크기 제한을 검사하고 파일을 메모리 스냅샷으로 읽는 부분. `Load`는 연 파일을 닫고 `Read`는 호출자가 전달한 ReaderAt을 닫지 않습니다.
3. [page.go](../internal/parser/page.go) → [interpreter.go](../internal/parser/interpreter.go): 페이지 선택, 콘텐츠 구문 분석, 명령 분배 순서. 콘텐츠별 구현은 [text.go](../internal/parser/text.go), [graphic.go](../internal/parser/graphic.go), [image.go](../internal/parser/image.go), [annotation.go](../internal/parser/annotation.go), 리소스 실행은 [resource.go](../internal/parser/resource.go)에서 이어 읽습니다.
4. [basic.go](../internal/parser/basic.go): 상세 콘텐츠를 페이지별 `Texts`·`Graphics`로 정리하는 부분.

`ParseFile`·`ParseReader`의 결과는 선택한 콘텐츠를 담은 `Result`, `LoadDocument`·`ReadDocument`의 결과는 객체와 바이트를 조회하는 `Document`입니다. `BuildPDF`는 `DetailedPDF`, `ParsePDF`는 페이지별 배열을 제공하는 `PDF`를 반환합니다. 현재 `ParsePDF`도 상세 분석을 수행하고 그 결과를 보관하며 `Details()`를 호출할 때 다시 파싱하지 않습니다.

바이트 단위 분석은 위치를 바이트로 추적한다는 의미입니다. 파일을 매번 1바이트씩 읽는 구현은 아닙니다. `Document.Bytes(span)`은 원본 또는 디코딩 소스의 `[Start, End)`를 복사해서 반환합니다.

## 패키지 경계

| 패키지 | 책임 | 프로젝트 내부 의존성 |
| --- | --- | --- |
| 루트 `gopd` | 공개 API 위임·타입·상수·오류 별칭 | `parser`, `common/document`, `common/syntax`, `common/pdfmodel` |
| `internal/parser` | 콘텐츠·폰트 해석·선택 추출·반환 모델 | `common/document`, `common/syntax`, `common/pdfmodel` |
| `internal/common/document` | 파일·객체·xref·스트림 읽기 | `syntax`, `pdfmodel` |
| `internal/common/syntax` | 독립 바이트 범위의 구문 해석 | `pdfmodel` |
| `internal/common/pdfmodel` | 공통 PDF 표현과 값·좌표 연산 | 없음 |
| `internal/common/pdftest` | 테스트용 PDF·스트림 생성 | 없음; 테스트에서만 사용 |

`Source → Transform → Object → Span`처럼 바이트 출처와 객체는 서로 연결되므로 같은 모델 패키지에 둡니다. `Value`, `ObjectOrigin`, `XRefEntry`의 비공개 메서드와 구현 타입도 함께 유지합니다.

`parser`는 공통 기반에 의존하며 `common`의 하위 패키지는 `parser`와 루트 `gopd`에 의존하지 않습니다. 텍스트·이미지·그래픽은 명령 순서와 상태를 공유하므로 같은 `parser` 패키지 안에서 파일별로 책임을 나눕니다.

## 공개 API와 호환성

외부 사용자는 계속 `github.com/MyungSub0519/gopd`를 가져와 `gopd.ParsePDF` 등을 호출합니다. 공개 함수는 [api.go](../api.go), 내부 타입의 공개 별칭은 [types.go](../types.go)에 있습니다. 공개 필드·메서드·상수·JSON 형식과 기존 해석 동작은 유지합니다. 작업량·출력·진단 예산의 계산 범위는 [자원 제한](resource-limits.md)을 확인하세요. 누락된 사전 키는 `errors.Is(err, gopd.ErrMissingKey)`로 확인합니다.

공통 타입의 정의는 `internal/common`의 하위 패키지로, 기본·상세·선택 추출 결과와 글꼴·스타일 타입의 정의는 `internal/parser`로 이동했습니다. 루트의 타입 별칭은 기존 사용법과 메서드를 유지하지만 실제 타입의 정의 패키지는 달라집니다. 이 차이는 `reflect.Type.PkgPath()`와 `%T`에 반영됩니다. 외부 코드는 내부 패키지 경로를 직접 가져오지 않습니다.

Go 1.25의 `go doc`은 별칭의 메서드를 따라가지 못할 수 있습니다. 저장소에서는 `go doc ./internal/common/document Document.Bytes`, `go doc ./internal/common/pdfmodel Dictionary.Get`, `go doc ./internal/common/pdfmodel Matrix.Mul`로 구현 문서를 볼 수 있습니다. 외부 사용자용 설명은 [공개 API 목록](public-api.md)에 있습니다.

## 테스트

테스트 파일은 저장소 전체 10개이며, 같은 기능의 정상 동작·오류·제한값·회귀 테스트를 한 파일에 모읍니다. 퍼즈·벤치마크·예제도 해당 기능의 테스트 파일에 함께 둡니다. `limits`, `review`, `hardening` 같은 검증 관점이나 작성 계기로 파일을 추가하지 않으며, 구현 파일마다 테스트 파일을 하나씩 만들지도 않습니다.

| 위치 | 파일 | 검증 범위 |
| --- | --- | --- |
| `internal/parser` | [basic_test.go](../internal/parser/basic_test.go) | 기본 응답·페이지별 배열·JSON |
| `internal/parser` | [parser_test.go](../internal/parser/parser_test.go) | 콘텐츠 실행·그래픽 상태·리소스·예산·합성 PDF 통합·퍼즈 |
| `internal/parser` | [font_test.go](../internal/parser/font_test.go) | 글꼴·CMap·인코딩·Unicode·문자 폭·선택 추출·제한값·퍼즈 |
| `internal/parser` | [extract_test.go](../internal/parser/extract_test.go) | 선택 추출·상태·출처·오류·퍼즈·벤치마크 |
| 루트 | [public_api_test.go](../public_api_test.go) | 외부 사용자 관점의 공개 API와 실행 가능한 추출 예제 |
| `internal/common/document` | [document_test.go](../internal/common/document/document_test.go) | 입력·바이트 범위·객체·xref·콘텐츠 연결·제한값·문서 퍼즈·영역 처리 벤치마크 |
| `internal/common/document` | [filters_test.go](../internal/common/document/filters_test.go) | 필터·예측자·스트림 디코딩·캐시·제한값·퍼즈 |
| `internal/common/pdfmodel` | [model_test.go](../internal/common/pdfmodel/model_test.go) | 사전·값 변환·행렬 연산 |
| `internal/common/syntax` | [syntax_test.go](../internal/common/syntax/syntax_test.go) | 토큰·객체 구문과 퍼즈 |
| `examples/gopd` | [main_test.go](../examples/gopd/main_test.go) | CLI 호출과 출력 |

테스트는 구현과 같은 패키지에 둡니다. 공개 API 검증은 루트 `public_api_test.go`의 `gopd_test` 패키지에서 수행합니다. `internal/parser/parser_test.go`의 통합 테스트와 CLI 테스트는 저장소에 포함된 `testdata/synthetic.pdf`를 사용하며, 파일이 없으면 실패합니다. 이 PDF는 실제 인물·주소·문서 메타데이터 없이 만든 두 페이지 합성 문서입니다. 생성 방법과 예상 콘텐츠는 [테스트 데이터 안내](../testdata/README.md)를 참고하세요.

합성 입력은 `pdftest.File(objects, trailerSuffix)`와 `pdftest.Stream(dict, content)`로 생성합니다. `trailerSuffix`는 원문 그대로 붙이므로 추가 항목 앞 공백도 호출자가 넣습니다. PDF 생성기는 파서 구현을 호출하지 않습니다.

```sh
go test ./...                 # 전체 테스트와 퍼즈 시드
go test ./internal/common/... # 파일 읽기·구문·공통 모델
go test ./internal/parser    # 콘텐츠 해석·통합·퍼즈 시드
go test . -run '^TestPublic'  # 공개 API
go build ./...
go vet ./...
```
