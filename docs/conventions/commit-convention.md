# Commit Convention

## 목적

커밋 메시지는 변경 이유와 범위를 빠르게 파악할 수 있어야 합니다.
이 저장소에서는 `Conventional Commits` 형식을 기본으로 하되, 현재 코드베이스에 맞는 scope와 검증 기준을 함께 사용합니다.

## 기본 형식

```text
type(scope): summary
```

`type`, `scope`, `summary`는 모두 영어 기준으로 작성합니다.

예시:

```text
feat(kernel): print multiboot validation result
fix(boot): preserve stack alignment before kernel entry
build(makefile): split toolchain checks by target
docs(conventions): define issue and PR workflow
```

## 허용 type

- `feat`: 새로운 기능이나 눈에 띄는 동작 추가
- `fix`: 버그, 회귀, 잘못된 동작 수정
- `refactor`: 동작 변경 없이 구조 개선
- `build`: Makefile, toolchain, linker, GRUB, ISO, QEMU 관련 변경
- `docs`: README, 규칙 문서, 주석, 운영 문서 변경
- `test`: 테스트, 검증 스크립트, 확인 절차 보강
- `ci`: GitHub Actions나 자동화 파이프라인 변경
- `chore`: 릴리스 준비, 이름 정리, 비기능성 유지보수

필요 이상의 type 확장은 피합니다.

## 권장 scope

- `boot`
- `kernel`
- `serial`
- `terminal`
- `linker`
- `grub`
- `makefile`
- `docs`
- `github`

scope는 선택 사항이지만, 변경 범위가 명확해지는 경우에는 반드시 적습니다.

## summary 규칙

- 명령형 현재형으로 작성합니다.
- 72자 이내를 권장합니다.
- 마침표를 붙이지 않습니다.
- 구현 방법보다 결과를 설명합니다.

좋은 예:

- `fix(kernel): guard against invalid multiboot magic`
- `docs(readme): clarify required cross-compiler tools`

피해야 할 예:

- `fix stuff`
- `updated files`
- `kernel changes for boot issue`

## 본문 작성 기준

아래 조건 중 하나에 해당하면 커밋 본문을 추가합니다.

- 부트 순서가 바뀐다.
- 메모리 주소, ABI, calling convention, linker layout이 바뀐다.
- 빌드나 실행에 필요한 외부 도구 가정이 바뀐다.
- 왜 이렇게 구현했는지 코드만으로 드러나지 않는다.

권장 형식:

```text
type(scope): summary

Why:
- ...

What:
- ...

Validation:
- make build
- make iso
```

## 커밋 분리 기준

- 리팩터링과 동작 변경은 가능한 한 분리합니다.
- 문서 변경은 해당 코드 변경과 함께 가도 되지만, unrelated 문서 수정은 섞지 않습니다.
- 포맷 변경만 있는 경우 별도 커밋으로 분리합니다.
- 한 커밋은 되돌릴 수 있을 만큼 독립적이어야 합니다.

## 금지 사항

- 여러 기능을 하나의 커밋에 섞지 않습니다.
- 검증되지 않은 대규모 rename과 동작 변경을 한 번에 넣지 않습니다.
- `WIP`, `tmp`, `misc`, `final` 같은 의미 없는 메시지를 쓰지 않습니다.
