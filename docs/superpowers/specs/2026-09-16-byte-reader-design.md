# PDF 바이트 읽기와 구문 분석의 내부 패키지 분리

## 범위

공통 모델 분리에 이어, PDF 파일을 읽고 바이트를 분석하는 구현과 단위 테스트를 내부 패키지로 옮긴다. 기존 사용자 API, 입력 제한, 소스 바이트 추적, 오류·부분 결과 및 JSON 계약을 유지한다.

## 경계

- `internal/syntax`: lexer, direct object parser, `Lex`, `ParseObject`, 내부 계층용 `ParseObjectWithLimits`. 스캐너와 문자열 디코더는 비공개로 유지한다.
- `internal/document`: 파일 스냅샷, `Document`, `ReadOptions`, 객체 캐시, xref, 스트림 경계·필터, 바이트 범위 복사. `syntax`와 `pdfmodel`만 참조하며 루트를 가져오지 않는다.
- 루트 `gopd`: `ParseFile`, `Parse`, `Lex`, `ParseObject`를 위임하고 `Document`, `ReadOptions`를 별칭으로 노출한다. 콘텐츠 해석기는 제한을 전달할 때 `syntax.ParseObjectWithLimits`를 호출한다.

파일 읽기는 여전히 입력 전체를 한 번 복사한 스냅샷이다. 바이트 단위 위치를 추적한다는 의미이며, 파일을 1바이트씩 읽는 방식으로 변경하지 않는다. `ParseFile`은 연 파일을 닫고 `Parse`는 호출자 소유 ReaderAt을 닫지 않는다.

## 호환성과 검증

공개 Document 메서드·필드와 ReadOptions는 그대로 사용한다. 정의가 이동하는 Document/ReadOptions와 내부 구문 오류 타입의 리플렉션 패키지 경로는 변경된다. 별칭 메서드를 찾지 못하는 Go 1.25 `go doc`의 제한은 내부 구현 문서 경로로 안내한다.

syntax/document 테스트와 퍼즈 타깃은 구현 옆으로 이동한다. 공개 API 테스트, 콘텐츠 테스트, 통합 테스트는 루트에 남긴다. 파일 크기·ReaderAt 오류·소스 범위 복사와 같은 입력 경계의 부족한 검증을 보강한다. 기존 테스트 목록과 전체 테스트·빌드·vet를 확인한다.
