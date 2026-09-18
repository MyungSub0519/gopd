# 프로젝트 구조

현재 폴더와 각 파일의 역할을 찾기 위한 안내서입니다. 루트 `gopd`는 공개 API와 페이지 콘텐츠 해석을 담당하고, `internal`은 파일 읽기·구문 분석·공통 PDF 모델을 담당합니다. 아래 트리는 주요 소스 파일과 테스트 자료를 표시합니다.

## 구성 원칙

- 패키지는 독립적인 책임과 의존 방향이 있을 때 나눕니다. 짧은 코드마다 별도 패키지를 만들지 않습니다.
- 공개 진입점과 타입 별칭은 각각 한곳에서 찾을 수 있게 모읍니다.
- 타입과 그 타입을 다루는 코드는 함께 둡니다. 객체·스트림·사전 조회는 같은 객체 모델에 속합니다.
- 파일 읽기와 구문 분석의 내부 경계는 유지합니다. 이후의 분리는 실제 의존 관계와 변경 빈도를 보고 결정합니다.

## 현재 구성

```text
gopd/
├── go.mod                  모듈 경로와 Go 버전
├── doc.go                  라이브러리 사용 단계와 제약에 대한 패키지 문서
├── api.go                  공개 파싱·구문·값 변환 함수
├── types.go                내부 타입·상수·오류의 공개 별칭
├── basic.go                기본 결과, FontInfo, Details(), 기본 결과 변환
├── detailed_model.go       상세 페이지·텍스트·그래픽·이미지 모델
├── style_types.go          색상·선·채우기·클리핑 상태
├── semantic.go             콘텐츠 해석 세션·의미 기반 값 조회·진단
├── semantic_budget.go      해석 작업량·값·출력 스타일 예산
├── pages.go                페이지 트리·상속·콘텐츠 연결·주석
├── content.go              콘텐츠 구문 분석·명령 실행 연결·출처
├── content_state.go        명령 분기·경로·클리핑·그리기
├── content_resources.go    리소스 조회·Form/Image·ExtGState
├── graphics_state.go       공통 선·점선 상태 검증과 설정
├── text.go                 텍스트 실행·글리프·위치 계산
├── fonts.go                Font 타입과 글꼴 리소스 해석
├── cmap.go                 CMap·CodeSpace 타입과 문자 매핑
├── basic_test.go           기본 응답·페이지별 배열·JSON 테스트
├── content_test.go         콘텐츠 명령·그리기 상태 테스트
├── fonts_test.go           글꼴·CMap 테스트
├── public_api_test.go      외부 패키지 관점의 공개 API 테스트
├── integration_test.go     합성 PDF 통합 테스트
├── internal/
│   ├── document/
│   │   ├── doc.go             파일 읽기 패키지 문서
│   │   ├── read.go            ReadOptions, 제한값, 파일·ReaderAt 입력
│   │   ├── document.go        문서 상태·바이트 범위·참조 해석
│   │   ├── objects.go         간접 객체·객체 스트림 로딩
│   │   ├── object_stream.go   객체 스트림 헤더 색인·캐시
│   │   ├── content_source.go  복수 콘텐츠 스트림 연결과 출처
│   │   ├── budget.go          문서 구문 값·xref 작업량 예산
│   │   ├── xref.go            객체 위치 색인·증분 갱신
│   │   ├── filters.go         스트림 디코딩과 예측자
│   │   ├── document_test.go   입력·바이트 범위·객체·xref·퍼즈 테스트
│   │   └── filters_test.go    스트림 디코딩·예측자·디코딩 예산
│   ├── syntax/
│   │   ├── doc.go             구문 분석 패키지 문서
│   │   ├── lexer.go           Lex와 토큰 스캐너
│   │   ├── parser.go          ParseObject와 사전·배열·문자열 분석
│   │   └── syntax_test.go     토큰·객체 구문·퍼즈 테스트
│   ├── pdfmodel/
│   │   ├── doc.go             공통 모델 패키지 문서
│   │   ├── object.go          PDF 값·스트림·간접 객체·사전 조회·값 변환
│   │   ├── source.go          소스·바이트 위치·변환 출처
│   │   ├── structure.go       파일 구조·xref·진단·분석 제한
│   │   ├── token.go           토큰 종류와 범위
│   │   ├── geometry.go        점·사각형·행렬 계산
│   │   └── model_test.go      PDF 값·사전·좌표 연산 테스트
│   └── pdftest/
│       └── fixture.go         테스트용 PDF·스트림 생성
├── examples/gopd/
│   ├── main.go               CLI 인자 처리와 결과 출력
│   └── main_test.go          CLI 테스트
├── testdata/
│   ├── synthetic.pdf         저장소에 포함한 합성 통합 테스트 입력
│   ├── generate.go           표준 라이브러리 기반의 재현 가능한 PDF 생성기
│   └── README.md             생성 방법·출처·예상 결과
├── README.md               영문 프로젝트 소개
├── README.ko.md            한국어 프로젝트 소개
└── docs/
    ├── project-structure.md   이 문서: 폴더와 파일별 역할
    ├── public-api.md          외부에서 호출할 수 있는 API 목록
    ├── resource-limits.md     자원 예산 계산 범위와 해석 정책
    ├── basic-pdf.md           기본 응답 사용법
    ├── json-structure.md      상세 결과의 JSON 구조
    ├── readme/                README용 로고 이미지
    └── superpowers/           작업별 설계(specs)·구현 계획(plans) 기록
```

`doc.go`는 각 패키지의 문서 설명입니다. 테스트 준비 함수는 사용하는 도메인의 테스트 파일에 함께 둡니다.

## 구현 파일별 역할

### 루트: 공개 API와 페이지 콘텐츠 해석

이 파일들은 모두 같은 `gopd` 패키지입니다. 파일별로 책임을 나누되, 콘텐츠 해석에 필요한 상태와 글꼴 정보를 같은 패키지 안에서 공유합니다.

| 파일 | 주요 타입·함수 | 작성된 기능 |
| --- | --- | --- |
| [api.go](../api.go) | `ParsePDF`, `Open`, `Read`, `BuildPDF`, `ParseFile`, `Parse`, `Lex`, `ParseObject` 등 | 외부 호출의 진입점입니다. 입력을 읽는 내부 패키지에 연결하고, `BuildPDF`에서 문서 카탈로그와 페이지 트리를 찾아 상세 콘텐츠 분석을 시작합니다. 값 변환·단위 행렬 함수도 여기에서 노출합니다. |
| [types.go](../types.go) | `Document`, `ReadOptions`, `Object`, `Span`, `Matrix` 등의 별칭 | 내부에 정의된 타입·상수·오류를 루트 API 이름으로 노출합니다. 타입을 새로 감싸거나 데이터를 복사하는 코드가 아니라, 외부 사용자가 `gopd.Document`처럼 접근하도록 연결하는 코드입니다. |
| [basic.go](../basic.go) | `PDF`, `Text`, `Graphic`, `PathSegment`, `FontInfo`, `Details`, `basicPDF` | 기본 반환 구조체와 상세 결과를 기본 결과로 바꾸는 기능입니다. 텍스트와 경로 그래픽을 페이지별 이중 배열에 담고, 기본 응답에 필요한 글꼴·스타일 정보만 옮깁니다. `Details()`는 저장해 둔 상세 결과를 반환합니다. |
| [detailed_model.go](../detailed_model.go) | `DetailedPDF`, `DetailedPage`, `DetailedText`, `DetailedGraphic`, `DetailedImage` 등 | 해석 결과를 저장하는 상세 모델입니다. 페이지, 글리프, 이미지 리소스, 주석, 실행 명령, 원본 바이트 출처와 진단 정보를 표현합니다. 페이지의 `Items`는 텍스트·그래픽·이미지가 섞인 실행 순서를 보존합니다. |
| [style_types.go](../style_types.go) | `Color`, `PaintStyle`, `GraphicsState`, `ClipPath` | 색상, 선 두께, 점선, 투명도, 혼합 모드와 클리핑 정보를 정의합니다. 기본 응답용 스타일과 상세 해석 중 사용하는 그래픽 상태를 구분합니다. |
| [semantic.go](../semantic.go), [semantic_budget.go](../semantic_budget.go) | `semanticBuilder`, `get`, `diag`, `chargeSemanticWork`, `chargeValues` | 해석 세션의 캐시·출력·예산을 보관합니다. 의미 기반 사전 조회와 진단 생성, 반복 리소스 작업 및 출력 크기 제한을 담당합니다. |
| [pages.go](../pages.go) | `walkPages`, `interpretPage`, `readAnnotations` | 페이지 트리와 상속된 속성을 읽습니다. 페이지 콘텐츠 연결·실행과 주석 해석은 별도 함수로 구분합니다. |
| [content.go](../content.go) | `contentInterpreter`, `contentSource`, `interpretSource` | 콘텐츠 스트림을 명령과 피연산자로 나누어 실행기로 전달하며, 결과의 페이지·명령·바이트 출처를 연결합니다. |
| [content_state.go](../content_state.go), [graphics_state.go](../graphics_state.go) | `contentState`, `textState`, `execute`, `pathOperation`, `paint`, `setLineParameter`, `setDash` | 명령 실행과 경로·클리핑·그래픽 출력을 담당합니다. 콘텐츠 명령과 ExtGState가 같은 선·점선 검증 함수를 사용하며, 리소스를 가짜 명령으로 다시 실행하지 않습니다. |
| [content_resources.go](../content_resources.go) | `resource`, `xobject`, `extGState` | 리소스 조회, Form/Image XObject 실행, ExtGState의 참조 해석과 상태 적용을 담당합니다. 반복 딕셔너리 검사와 숫자 배열 확장 전에 예산을 확인합니다. |
| [text.go](../text.go) | `moveText`, `showText` | 문자 표시에서 텍스트·글리프·이동량을 만들고 텍스트 위치를 갱신합니다. |
| [fonts.go](../fonts.go) | `Font`, `font`, `cidWidths`, `simpleFontEncoding`, `decodeBounded` | 글꼴 리소스의 종류·이름·인코딩·문자 폭·ToUnicode 정보를 읽습니다. 콘텐츠에 들어 있는 문자 코드를 해석하고, 글리프 위치 계산에 필요한 폭 정보를 제공합니다. |
| [cmap.go](../cmap.go) | `CodeSpace`, `CMap`, `parseToUnicode`, `decodeBounded` | ToUnicode CMap을 읽어 PDF 글꼴의 문자 코드와 Unicode 문자열을 연결합니다. 코드 길이와 매핑 범위를 처리하고 디코딩 결과의 완전성 및 출력 크기 제한을 관리합니다. |
| [doc.go](../doc.go) | `gopd` 패키지 문서 | 기본·상세·저수준 API의 사용 단계, 메모리 소유권, 동시 호출 제약, 좌표와 실행 순서의 의미를 설명합니다. |

`pages.go`는 어떤 페이지와 콘텐츠를 해석할지 관리하고, `content.go`는 구문 분석과 실행을 연결합니다. `content_state.go`·`text.go`는 명령이 상태와 결과를 바꾸는 규칙, `content_resources.go`는 리소스를 통한 실행을 담당합니다. 서로 같은 상태를 공유하는 구현이므로 새 패키지나 인터페이스는 추가하지 않았습니다. `fonts.go`는 글꼴 리소스 전체를 다루고, `cmap.go`는 그중 문자 코드 매핑을 담당합니다.

### internal/document: PDF 파일과 객체 읽기

PDF 파일의 물리적 구조를 다루는 패키지입니다. xref는 객체 번호로 파일 위치 또는 객체 스트림 위치를 찾는 색인입니다.

| 파일 | 주요 타입·함수 | 작성된 기능 |
| --- | --- | --- |
| [read.go](../internal/document/read.go) | `ReadOptions`, `normalizeOptions`, `ParseFile`, `Parse` | 입력·분석 제한값을 설정하고 파일 또는 `io.ReaderAt`에서 전체 바이트 스냅샷을 읽습니다. `Document`를 초기화한 뒤 헤더와 xref 분석을 시작합니다. |
| [document.go](../internal/document/document.go) | `Document`, `Bytes`, `RawObject`, `Catalog`, `Resolve`, `ResolveObject` | 원본·디코딩 소스와 객체 캐시 등 문서 상태를 보관합니다. 바이트 범위 조회, 문서 최상위 카탈로그 조회, 간접 참조 해석과 참조 순환 검사를 제공합니다. |
| [objects.go](../internal/document/objects.go) | `Load`, `parseIndirect`, `loadCompressed` | 객체 번호에 해당하는 간접 객체를 필요할 때 읽고 캐시합니다. `obj`·`endobj` 경계, 스트림 길이, 여러 객체를 담는 객체 스트림을 처리합니다. |
| [xref.go](../internal/document/xref.go) | `readHeaderAndXRefs`, `readXRefChain`, `readXRefTable`, `readXRefStream` | 헤더와 파일 끝 정보를 읽고, 표 또는 스트림 형태의 xref를 분석합니다. 증분 저장 이력을 따라가 최신 객체 위치를 적용하며 삭제된 객체와 참조 순환도 처리합니다. |
| [filters.go](../internal/document/filters.go) | `DecodeStream`, `decodeFilter`, `applyPredictor` | 스트림에 적용된 압축·인코딩 필터와 예측자(차분으로 저장한 값을 복원하는 처리)를 해제합니다. 디코딩 크기 제한을 적용하고, 결과를 별도 소스와 출처 정보로 등록·캐시합니다. |
| [doc.go](../internal/document/doc.go) | `document` 패키지 문서 | 바이트 스냅샷, 범위 조회, 객체 조회, xref와 스트림 처리라는 패키지 책임을 설명합니다. |

### internal/syntax: 바이트를 PDF 문법으로 읽기

주어진 바이트 구간을 토큰과 PDF 값으로 바꾸는 패키지입니다. 파일을 열거나 간접 참조가 가리키는 객체를 로딩하는 일은 `document`가 담당합니다.

| 파일 | 주요 타입·함수 | 작성된 기능 |
| --- | --- | --- |
| [lexer.go](../internal/syntax/lexer.go) | `Lex`, `syntaxScanner` | 숫자, 이름, 문자열, 구분자, 공백, 주석 등의 토큰을 구분하고 소스의 바이트 위치를 보존합니다. 토큰 크기와 개수 제한도 검사합니다. |
| [parser.go](../internal/syntax/parser.go) | `ParseObject`, `ParseObjectWithLimits`, `objectParser` | 토큰을 숫자·문자열·배열·사전·간접 참조 등의 `Object`로 조립합니다. 이름 및 문자열의 이스케이프와 16진 표현을 해석하고, 중첩 깊이 등 제한을 검사합니다. |
| [doc.go](../internal/syntax/doc.go) | `syntax` 패키지 문서 | 독립된 바이트 범위의 토큰화와 객체 구문 분석이라는 책임을 설명합니다. |

예를 들어 `<< /Type /Page /Contents 12 0 R >>`를 사전과 참조 값으로 만드는 곳은 `syntax`, `12 0 R`이 가리키는 객체를 찾는 곳은 `document`, 그 내용에서 텍스트와 그래픽을 만드는 곳은 루트 콘텐츠 해석 코드입니다.

### internal/pdfmodel: 공통 PDF 타입과 연산

파일 읽기와 콘텐츠 해석이 함께 사용하는 PDF 표현입니다. 다른 프로젝트 내부 패키지에 의존하지 않습니다.

| 파일 | 주요 타입·함수 | 작성된 기능 |
| --- | --- | --- |
| [object.go](../internal/pdfmodel/object.go) | `Object`, `Value`, `Dictionary`, `Reference`, `Stream`, `IndirectObject`, `Int`, `Number` | PDF의 기본 값, 간접 객체와 스트림을 정의합니다. 사전 키 조회·중복 키 처리·숫자 변환·스트림 여부 확인도 함께 둡니다. |
| [source.go](../internal/pdfmodel/source.go) | `SourceID`, `Position`, `Span`, `Source`, `Derivation`, `Transform` | 원본 또는 디코딩된 데이터의 위치와 바이트 범위, 변환 출처를 표현합니다. `Span`의 범위는 `[Start, End)`입니다. |
| [structure.go](../internal/pdfmodel/structure.go) | `Structure`, `Header`, `FileTail`, `XRefSection`, `Diagnostic`, `Limits` 등 | 파일 헤더·끝부분·영역, xref 항목과 구간, 진단 및 분석 제한을 정의합니다. 실제 xref를 읽는 코드는 `document/xref.go`에 있습니다. |
| [token.go](../internal/pdfmodel/token.go) | `TokenKind`, `Token` | 구문 분석기가 반환하는 토큰의 종류와 바이트 범위를 정의합니다. |
| [geometry.go](../internal/pdfmodel/geometry.go) | `Point`, `Rect`, `Matrix`, `IdentityMatrix`, `Transform`, `Mul` | 점·사각형·좌표 변환 행렬과 행렬 합성·점 변환 연산을 제공합니다. |
| [doc.go](../internal/pdfmodel/doc.go) | `pdfmodel` 패키지 문서 | 공통 PDF 값, 소스 위치, 좌표와 구조 모델의 범위를 설명합니다. |

### 테스트 지원·CLI·설정

| 파일 | 작성된 기능 |
| --- | --- |
| [internal/pdftest/fixture.go](../internal/pdftest/fixture.go) | `File`과 `Stream`으로 테스트에 필요한 작은 PDF와 스트림을 만듭니다. 파서 구현에 의존하지 않아 입력 생성과 파싱 검증을 분리합니다. |
| [examples/gopd/main.go](../examples/gopd/main.go) | CLI 인자를 검사하고 `ParsePDF`를 호출합니다. 기본 통계, `-text` 텍스트, `-json` 통계를 출력하고 종료 코드를 결정합니다. `-json`은 전체 기본 응답이 아니라 개수 중심 요약 JSON입니다. |
| [go.mod](../go.mod) | 모듈 경로 `github.com/MyungSub0519/gopd`와 Go 버전 `1.25.0`을 선언합니다. |

각 테스트 파일의 담당 범위는 아래 **테스트** 절에 정리했습니다. [공개 API 목록](public-api.md)은 함수와 타입을, [기본 응답](basic-pdf.md)과 [JSON 구조](json-structure.md)는 반환값을 설명합니다. `docs/superpowers`는 작업 당시의 설계·계획 기록이므로 현재 파일 배치는 이 문서를 기준으로 확인합니다.

## 코드를 읽는 순서

1. [api.go](../api.go): `ParsePDF → Open → ParseFile → BuildPDF` 호출 흐름.
2. [internal/document/read.go](../internal/document/read.go): 크기 제한을 검사하고 파일을 메모리 스냅샷으로 읽는 부분. `ParseFile`은 연 파일을 닫고 `Parse`는 호출자가 전달한 ReaderAt을 닫지 않습니다.
3. [pages.go](../pages.go) → [content.go](../content.go) → [content_state.go](../content_state.go): 페이지 선택, 콘텐츠 구문 분석, 명령 실행 순서. 텍스트는 [text.go](../text.go), 리소스는 [content_resources.go](../content_resources.go), 예산은 [semantic_budget.go](../semantic_budget.go)에서 이어 읽습니다.
4. [basic.go](../basic.go): 상세 콘텐츠를 페이지별 `Texts`·`Graphics`로 정리하는 부분.

`ParseFile`의 결과는 객체와 바이트를 조회하는 `Document`, `BuildPDF`의 결과는 페이지 내용을 해석한 `DetailedPDF`, `ParsePDF`의 결과는 페이지별 배열을 제공하는 `PDF`입니다. 현재 `ParsePDF`도 상세 분석을 수행하고 그 결과를 보관합니다. `Details()`를 호출할 때 다시 파싱하지 않습니다.

바이트 단위 분석은 위치를 바이트로 추적한다는 의미입니다. 파일을 매번 1바이트씩 읽는 구현은 아닙니다. `Document.Bytes(span)`은 원본 또는 디코딩 소스의 `[Start, End)`를 복사해서 반환합니다.

## 패키지 경계

| 패키지 | 책임 | 프로젝트 내부 의존성 |
| --- | --- | --- |
| 루트 `gopd` | 공개 API·반환 모델·콘텐츠·폰트 해석 | `document`, `syntax`, `pdfmodel` |
| `internal/document` | 파일·객체·xref·스트림 읽기 | `syntax`, `pdfmodel` |
| `internal/syntax` | 독립 바이트 범위의 구문 해석 | `pdfmodel` |
| `internal/pdfmodel` | 공통 PDF 표현과 값·좌표 연산 | 없음 |
| `internal/pdftest` | 테스트용 PDF·스트림 생성 | 없음; 테스트에서만 사용 |

`Source → Transform → Object → Span`처럼 바이트 출처와 객체는 서로 연결되므로 같은 모델 패키지에 둡니다. `Value`, `ObjectOrigin`, `XRefEntry`의 비공개 메서드와 구현 타입도 함께 유지합니다.

별도 `internal/geometry`는 좌표 모델에 필요한 작은 연산만 제공하고 루트 별칭에서만 사용했으므로 `pdfmodel/geometry.go`에 합쳤습니다. 콘텐츠·폰트 해석은 현재 루트에서 협력하며, 독립적인 사용처나 의존 경계가 필요해질 때 패키지 분리를 검토합니다.

## 공개 API와 호환성

외부 사용자는 계속 `github.com/MyungSub0519/gopd`를 가져와 `gopd.ParsePDF` 등을 호출합니다. 공개 함수는 [api.go](../api.go), 내부 타입의 공개 별칭은 [types.go](../types.go)에 있습니다. 공개 필드·메서드·상수·JSON 형식은 유지합니다. 값 해석과 제한 경계 오류는 수정하며, 반복 리소스 작업·스타일 출력·진단에도 예산을 적용하므로 이전에 성공한 입력이 `ErrLimit`을 반환할 수 있습니다. 계산 범위는 [자원 제한](resource-limits.md)을 확인하세요. 누락된 사전 키는 `errors.Is(err, gopd.ErrMissingKey)`로 확인합니다.

이전 내부 이동으로 `Document`·공통 모델의 실제 정의 경로가 변경되었으며, 이번 통합에서 `Point`·`Rect`·`Matrix`의 정의도 `internal/geometry`에서 `internal/pdfmodel`로 이동했습니다. 이 차이는 `reflect.Type.PkgPath()`와 `%T`에 반영됩니다. 외부 코드는 내부 패키지 경로를 직접 가져오지 않습니다.

Go 1.25의 `go doc`은 별칭의 메서드를 따라가지 못할 수 있습니다. 저장소에서는 `go doc ./internal/document Document.Bytes`, `go doc ./internal/pdfmodel Dictionary.Get`, `go doc ./internal/pdfmodel Matrix.Mul`로 구현 문서를 볼 수 있습니다. 외부 사용자용 설명은 [공개 API 목록](public-api.md)에 있습니다.

## 테스트

같은 기능의 단위 테스트와 회귀 테스트를 함께 둡니다. 구현 파일마다 테스트 파일을 하나씩 만들지는 않습니다. 아래는 주요 테스트의 담당 범위입니다.

| 위치 | 파일 | 검증 범위 |
| --- | --- | --- |
| 루트 | [basic_test.go](../basic_test.go) | 기본 응답·페이지별 배열·JSON |
| 루트 | [content_test.go](../content_test.go) | 콘텐츠 실행·변환·그래픽 상태·공통 PDF 준비 함수 |
| 루트 | [content_resources_test.go](../content_resources_test.go) | 반복 리소스·출력 스타일·진단 예산과 ExtGState의 참조·null 처리 |
| 루트 | [content_hardening_test.go](../content_hardening_test.go), [content_fuzz_test.go](../content_fuzz_test.go) | 콘텐츠 경계·제한·malformed 입력·퍼즈 |
| 루트 | [fonts_test.go](../fonts_test.go) | 글꼴 인코딩·CMap·Unicode 매핑 |
| 루트 | [font_semantics_test.go](../font_semantics_test.go), [font_limits_test.go](../font_limits_test.go) | Differences 참조·정확한 표준 글꼴 이름·글꼴 및 CMap 예산 |
| 루트 | [public_api_test.go](../public_api_test.go) | 외부 사용자 관점의 공개 API |
| 루트 | [integration_test.go](../integration_test.go) | 합성 PDF의 기본·상세 결과 |
| `internal/document` | [document_test.go](../internal/document/document_test.go) | 입력·바이트 범위·객체·xref·문서 퍼즈 |
| `internal/document` | [filters_test.go](../internal/document/filters_test.go) | 필터·예측자·디코딩 제한 |
| `internal/document` | [review_boundaries_test.go](../internal/document/review_boundaries_test.go) | 참조 깊이 경계·직접 null 옵션·소진된 예산의 빈 스트림 |
| `internal/document` | [filters_fuzz_test.go](../internal/document/filters_fuzz_test.go) | 필터 및 TIFF/PNG 예측자 퍼즈 |
| `internal/pdfmodel` | [model_test.go](../internal/pdfmodel/model_test.go) | 사전·값 변환·행렬 연산 |
| `internal/syntax` | [syntax_test.go](../internal/syntax/syntax_test.go) | 토큰·객체 구문과 퍼즈 |
| `examples/gopd` | [main_test.go](../examples/gopd/main_test.go) | CLI 호출과 출력 |

테스트는 구현과 같은 패키지에 둡니다. 공개 API 검증은 루트 `public_api_test.go`의 `gopd_test` 패키지에서 수행합니다. 루트 `integration_test.go`와 CLI 테스트는 저장소에 포함된 `testdata/synthetic.pdf`를 사용하며, 파일이 없으면 실패합니다. 이 PDF는 실제 인물·주소·문서 메타데이터 없이 만든 두 페이지 합성 문서입니다. 생성 방법과 예상 콘텐츠는 [테스트 데이터 안내](../testdata/README.md)를 참고하세요.

합성 입력은 `pdftest.File(objects, trailerSuffix)`와 `pdftest.Stream(dict, content)`로 생성합니다. `trailerSuffix`는 원문 그대로 붙이므로 추가 항목 앞 공백도 호출자가 넣습니다. PDF 생성기는 파서 구현을 호출하지 않습니다.

```sh
go test ./...                 # 전체 테스트와 퍼즈 시드
go test ./internal/...        # 파일 읽기·구문·공통 모델
go test . -run '^TestPublic'  # 공개 API
go build ./...
go vet ./...
```
