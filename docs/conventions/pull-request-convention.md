# Pull Request Convention

## 목적

PR은 "무엇을 바꿨는가"보다 "왜 이 범위로 바꿨고 어떻게 검증했는가"가 드러나야 합니다.
이 저장소에서는 리뷰어가 부트 경로와 빌드 영향을 빠르게 판단할 수 있도록 설명과 검증 기록을 중시합니다.

## PR 제목 규칙

PR 제목은 squash merge 시 최종 커밋 제목으로 써도 되는 수준으로 작성합니다.
가능하면 커밋 규칙과 같은 형식을 사용합니다.

```text
type(scope): summary
```

예시:

- `build(makefile): improve toolchain failure messages`
- `fix(kernel): stop writing past terminal width`
- `docs(conventions): define repository workflow`

## PR 범위 규칙

- 하나의 PR은 하나의 concern만 다룹니다.
- unrelated 정리나 포맷 수정은 분리합니다.
- 큰 구조 변경은 먼저 이슈에서 범위를 고정하고 들어옵니다.
- 초안 상태에서 논의가 필요한 PR은 Draft로 유지합니다.

## PR 설명 작성 규칙

`.github/PULL_REQUEST_TEMPLATE.md`를 기준으로 다음을 반드시 채웁니다.

- 해결하려는 문제
- 핵심 변경
- 이번 PR에서 제외한 범위
- 관련 이슈
- 검증 환경과 검증 명령
- 리뷰어가 특히 봐야 할 부분

## 검증 기록 규칙

변경 종류에 따라 아래 중 해당 항목을 남깁니다.

- `make build`
- `make iso`
- `make run` 또는 `make`
- 문서 링크 렌더링 확인
- 로그, 직렬 출력, 화면 캡처

커널, 부트, 링크, 빌드 변경에서는 "명령만" 적지 말고 결과도 적습니다.

예시:

```text
확인 환경:
- macOS 15
- qemu-system-x86_64 8.2

확인 방법:
- make build
- make iso

확인 결과:
- kernel.elf 링크 성공
- dionysus.iso 생성 성공
```

## 리뷰 준비 규칙

- 리뷰 포인트에는 의도적으로 판단을 부탁하는 지점을 적습니다.
- 리스크가 있으면 숨기지 말고 PR 본문에 적습니다.
- 아직 검증하지 못한 항목은 비워두지 말고 명시합니다.

## 머지 전 체크

- 관련 이슈가 연결되어 있다.
- 범위 밖 변경이 섞여 있지 않다.
- README 또는 관련 문서가 필요한 만큼 갱신되었다.
- 검증 근거가 남아 있다.
- 민감정보가 포함되지 않았다.
