# Linux Base Convention

## 목적

이 문서는 Dionysus가 upstream Linux를 제품 베이스로 사용할 때 반복적으로 필요한 기록과 변경 규칙을 정의합니다.
Linux 베이스를 바꾸는 작업은 boot path, kernel config, initramfs, 제어 평면 동작에 연쇄 영향을 주므로
"무엇을 기준으로 삼았는지"를 항상 재현 가능하게 남겨야 합니다.

## 기본 원칙

- Dionysus의 기본 전략은 upstream Linux reuse 이다.
- 가능하면 사용자 공간, initramfs, service, agent에서 해결한다.
- kernel module, eBPF, config fragment, patch는 필요한 최소 범위로 제한한다.
- Linux 베이스 관련 변경은 항상 "어떤 upstream 기준" 위에서 검증했는지 남긴다.

## 반드시 기록할 항목

Linux 베이스 관련 변경에는 아래 항목을 이슈, PR, 또는 관련 문서에 반드시 남깁니다.

- 사용한 upstream remote
- 기준 브랜치 또는 tag
- 정확한 commit SHA
- 적용한 config fragment 또는 `.config` 생성 방식
- 추가한 patch/module/eBPF artifact
- initramfs 구성 방식
- OS 내부 service 또는 systemd unit 구성 방식
- 실제 검증 명령
- 관찰한 부팅 또는 실행 결과
- 비 Linux host였다면 사용한 container builder 경로

## 변경 우선순위

Linux 기반 기능을 추가할 때는 아래 순서를 우선합니다.

1. 표준 Linux 인터페이스로 해결 가능한지 확인한다.
2. initramfs 또는 사용자 공간 agent에서 해결 가능한지 확인한다.
3. 그래도 부족하면 out-of-tree module 또는 eBPF를 검토한다.
4. 마지막 수단으로만 kernel patch를 사용한다.

## 피해야 할 작업

- 이유 없이 upstream Linux 전체를 포크하는 것
- commit SHA 없이 "latest Linux" 같은 표현만 남기는 것
- 커널 설정 변경 후 부팅 검증 기록 없이 제출하는 것
- 사용자 공간에서 해결 가능한 기능을 곧바로 kernel patch로 구현하는 것
- Dionysus 제품 기능과 무관한 임시 실험 patch를 설명 없이 남기는 것

## 검증 기록 규칙

다음 변경은 반드시 정확한 검증 명령과 관찰 결과를 기록합니다.

- Linux kernel commit 변경
- kernel config 변경
- initramfs 구성 변경
- OS 내부 service, systemd unit, 또는 부팅 시 자동 실행 경로 변경
- 부트 파라미터 변경
- Dionysus module/eBPF 추가 또는 수정

권장 기록 형식:

- `Command:` 실행한 명령 전체
- `Result:` 핵심 관찰 결과
- `Base:` upstream branch/tag 와 commit SHA

예시:

- `Command: make linux-boot`
- `Result: Linux 6.x kernel reached initramfs and started dinosus-agent successfully`
- `Base: torvalds/linux master @ <commit-sha>`

initramfs overlay 또는 early userspace 변경만 있는 경우에도 최소한 아래 중 하나를 남깁니다.

- `Command: make linux-initramfs-layout`
- `Command: BUSYBOX_BIN=/path/to/busybox make linux-initramfs`
- `Command: make pve-control-plane-install`

Linux 부팅 경로나 GRUB 구성이 바뀌면 아래 명령을 우선 기준으로 기록합니다.

- `Command: make linux-kernel`
- `Command: make linux-iso`
- `Command: make linux-run`

비 Linux host에서는 container fallback 사용 여부도 같이 적습니다.

- `Builder: docker image dionysus/linux-builder:bookworm-amd64`
