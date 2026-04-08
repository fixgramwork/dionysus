# Branch Convention

## 목적

브랜치 이름만 보고도 작업 종류와 범위를 바로 파악할 수 있어야 합니다.
이 저장소에서는 commit type과 맞춰 영어 소문자 기준으로 브랜치 이름을 작성합니다.

## 기본 형식

```text
type/issue-number-short-summary
```

이슈 번호가 아직 없으면 아래 형식을 사용합니다.

```text
type/short-summary
```

Codex가 작업 브랜치를 만들 때는 아래 형식을 기본으로 사용합니다.

```text
codex/type/issue-number-short-summary
```

## 허용 type

- `feat`
- `fix`
- `refactor`
- `build`
- `docs`
- `test`
- `ci`
- `chore`

## 작성 규칙

- 영어 소문자만 사용합니다.
- 단어 구분은 `kebab-case`를 사용합니다.
- 공백, 한글, 대문자, 특수문자는 넣지 않습니다.
- summary는 3~6단어 정도의 짧은 작업 설명으로 유지합니다.
- 브랜치 이름은 결과가 아니라 작업 주제를 표현합니다.

## 예시

- `feat/42-memory-map-parser`
- `fix/17-terminal-wrap-bug`
- `build/9-grub-toolchain-check`
- `docs/workflow-conventions`
- `codex/docs/58-issue-template-prefixes`

## 분기 기준

- 서로 다른 concern은 브랜치를 분리합니다.
- 대규모 리팩터링은 기능 변경 브랜치와 섞지 않습니다.
- 실험성 작업은 오래 유지하지 말고, 방향이 정해지면 새 브랜치로 정리합니다.

## 금지 사항

- `test`, `work`, `temp`, `final`처럼 의미 없는 이름
- 하나의 브랜치에서 여러 기능을 동시에 진행하는 이름
- 이슈와 무관한 개인 메모성 이름
