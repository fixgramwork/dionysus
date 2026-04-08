# Workflow Conventions

이 디렉터리는 이 저장소에서 반복적으로 적용할 작업 규칙의 기준 문서입니다.
작업을 시작하기 전에 필요한 문서를 먼저 확인하고, 변경이 생기면 문서도 함께 갱신합니다.

## 문서 목록

- [`commit-convention.md`](commit-convention.md): 커밋 메시지 형식과 커밋 분리 기준
- [`branch-convention.md`](branch-convention.md): 브랜치 이름 형식과 작업 브랜치 분리 기준
- [`issue-convention.md`](issue-convention.md): 이슈 제목, 템플릿 선택 기준, 완료 조건 작성 규칙
- [`pull-request-convention.md`](pull-request-convention.md): PR 제목, 설명, 검증 기록, 리뷰 준비 규칙
- [`code-convention.md`](code-convention.md): Rust, NASM, linker script, Makefile 작성 규칙

## 기본 원칙

- 한 이슈, 한 PR, 한 커밋 묶음은 하나의 문제를 해결해야 한다.
- 구조나 동작을 바꾸면 문서와 검증 기록을 함께 남긴다.
- 부트 경로, 메모리 레이아웃, 툴체인 가정이 바뀌면 이유와 검증 결과를 반드시 적는다.
- 저장소의 현재 단계에서는 자동화보다 재현 가능한 수동 검증 기록이 더 중요하다.

## 권장 작업 흐름

1. 먼저 이슈를 만들거나 기존 이슈 범위를 확인한다.
2. 작은 단위로 구현하고 커밋을 나눈다.
3. PR 설명에 변경 범위, 제외 범위, 검증 결과를 명시한다.
4. 규칙이 반복적으로 필요해졌다면 이 디렉터리 문서에 반영한다.
