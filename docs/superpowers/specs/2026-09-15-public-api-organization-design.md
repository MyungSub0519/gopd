# 공개 API 역할 정리

## 목적과 승인 범위

라이브러리 사용자가 필요한 기능을 고를 수 있도록 공개 기능을 목록화하고, 그 분류대로 코드를 정리한다. 사용자는 기존 함수명과 타입의 호환성을 유지하는 방식을 선택했다.

## 공개 기능

1. 기본 파싱: `ParsePDF`, `PDF.Details`.
2. 상세 콘텐츠 분석: `Open`, `Read`, `BuildPDF`.
3. 문서 구문과 객체 접근: `ParseFile`, `Parse`, `Document.Catalog`, `Load`, `Resolve`, `ResolveObject`, `Bytes`, `RawObject`, `DecodeStream`.
4. 독립 구문 분석: `Lex`, `ParseObject`.
5. 값 조회·변환: `Dictionary.Get`, `GetAll`, `Int`, `Number`, `IsStream`, `ErrMissingKey`.
6. 좌표 계산: `IdentityMatrix`, `Matrix.Transform`, `Matrix.Mul`.

12개 공개 함수와 공개 타입의 12개 메서드, 기존 타입·필드·상수의 이름과 형태를 유지한다. 기본 결과는 현재와 같이 페이지별 `Texts`, `Graphics` 두 필드만 노출한다. 상세 파싱은 여전히 즉시 수행하고 결과를 보관한다.

## 코드 배치

- `api_basic.go`: 기본 파싱과 상세 결과 접근.
- `api_detailed.go`: 상세 파일/reader/Document 진입점.
- `api_document.go`: 저수준 문서 입력 진입점.
- `api_syntax.go`: 독립 토큰·객체 파싱 진입점.
- `basic_projection.go`: 상세 결과를 기본 결과로 변환하는 비공개 구현.
- `basic_model.go`, `detailed_model.go`: 사용 수준에 따른 반환 모델.
- `geometry.go`, `style_types.go`, `font_types.go`: 공유 좌표·스타일·글꼴 타입.
- `read_options.go`: 입력 제한 타입과 기본값 처리.
- `compat_types.go`: 현재 파싱 결과에서 사용하지 않는 `Page`, `Image`, `ImageInfo`, `ParseDiagnostic`. 정의를 유지하고 Deprecated 안내로 실제 반환 타입을 안내한다.
- 기존 lexer/parser, 객체·xref·필터·콘텐츠 해석 파일은 해당 알고리즘을 유지한다.

## 검증과 비범위

외부 패키지 테스트로 파일 기본 파싱, reader 상세 파싱, 제한 전달, 객체·스트림 접근, 구문·값·좌표 기능을 검증한다. 기존 합성·사용자 PDF 테스트와 `go vet`를 유지한다. 공개 선언의 변경은 문서화와 Deprecated 표시뿐이며 함수·타입을 제거하거나 패키지를 이동하지 않는다. 기능 추가, 새로운 출력 형식, 파싱 최적화는 범위에 포함하지 않는다.
