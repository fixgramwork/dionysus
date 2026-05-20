# Linux-Based Control Plane

상태: 초안  
관련 이슈: rebase platform direction on upstream Linux

## 목적

Dionysus의 장기 목표는 개발자 친화적인 운영 경험을 제공하는 것입니다.
이 목표를 위해 커스텀 커널을 계속 키우는 대신, 리누스 토발즈의 upstream Linux를 기반으로 삼고
Dionysus는 그 위에 제어 평면과 운영 경험을 쌓습니다.

이 방향 전환의 핵심은 다음과 같습니다.

- 커널 자체를 새로 만드는 비용보다 Linux가 이미 제공하는 하드웨어 지원과 안정성을 활용한다.
- Dionysus 고유 가치는 커널 재구현이 아니라 안전한 관찰, 제한된 제어, 개발자 UX에 둔다.
- upstream Linux를 최대한 그대로 유지하고, Dionysus-specific 로직은 가능한 한 사용자 공간과 얇은 확장 계층에 둔다.

## 방향 전환 선언

이 저장소는 더 이상 독립 ASM/C 부트 프로토타입을 제품 경로로 유지하지 않으며,
부팅 기준은 upstream Linux와 Dionysus initramfs 조합으로 고정합니다.

앞으로의 기준은 다음과 같습니다.

- 커널 베이스: upstream Linux
- 제품 레이어: Dionysus agent, control API, operator UI
- 커널 확장 원칙: 필요할 때만 최소한의 module, eBPF, config fragment, patch 사용

## 계층 구조

```text
Bootloader/Firmware -> Linux kernel -> kernel interfaces -> Dionysus agent -> Perl API2 daemon -> pveproxy -> ExtJS-style UI
```

여기서 `kernel interfaces` 는 다음과 같은 Linux 표준 경로를 의미합니다.

- `/proc`
- `/sys`
- `netlink`
- `ioctl`
- `debugfs` 또는 `tracefs`
- 필요 시 Dionysus 전용 character device
- 필요 시 eBPF program/map

## 계층별 책임

| 계층 | 주 책임 | 제공 결과 | 하지 말아야 할 일 |
| --- | --- | --- | --- |
| Bootloader/Firmware | Linux 커널과 initramfs를 시작할 수 있는 부팅 컨텍스트 제공 | 재현 가능한 부팅 경로 | Dionysus 정책 구현 |
| Linux kernel | 하드웨어 추상화, 프로세스/메모리/파일시스템/네트워크 핵심 기능 제공 | 검증된 운영체제 기반 | Dionysus UI, 운영자 워크플로 구현 |
| Kernel interfaces | 커널 상태와 제어 기능을 표준 계약으로 노출 | 안정된 조회/제어 엔트리포인트 | 제품별 UX 정책 강제 |
| Dionysus agent | Linux 인터페이스 수집, 정책 적용, 안전한 자원 모델 구성 | Dionysus resource model, 감사 가능한 요청 처리 | Linux 핵심 기능 재구현 |
| Perl API2 daemon | 인증, 인가, audit log, session, API 집계 | Proxmox-style `/api2/json` 운영자 API | 커널 우회 직접 제어 |
| pveproxy + ExtJS-style UI | 상태 시각화, diff, 승인 플로우, 제한된 조작 UX | 운영자 화면과 안전한 상호작용 | 정책 원천 결정 |

## OS 내부 관리 웹 실행 모델

Proxmox와 같은 관리 경험을 목표로 할 때 Dionysus 관리 웹은 개발 머신의 Node 서버가 아니라
관리 대상 Linux OS 내부에서 실행되는 서비스여야 합니다.

현재 제품 기준은 Proxmox와 비슷한 `pvedaemon`/`pveproxy` 구조입니다.

- 일반 Linux rootfs 경로: `dionysus-pvedaemon.service` 가 localhost API daemon을 시작하고, `dionysus-pveproxy.service` 가 `0.0.0.0:8006` 에서 웹 UI와 `/api2/json` API를 제공합니다.
- Local LLM 경로: `dionysus-llm-swap.service` 가 `ollama.service` 보다 먼저 dedicated swap backing store를 준비합니다.

초기 부팅 경로는 관리 웹을 띄우지 않습니다.
initramfs는 `/proc`, `/sys`, 네트워크, 부트 상태를 확인하고 rescue shell을 제공하는 bootstrap 계층입니다.
관리 웹/API는 rootfs의 systemd service에서만 실행합니다.

- initramfs 경로: `/init` 이 `dionysus-agent bootstrap`, `dionysus-network start`, `dionysus-services banner` 를 실행합니다.
- rootfs 경로: `dionysus-pvedaemon.service`, `dionysus-pveproxy.service`, `dionysus-llm-swap.service` 를 systemd가 관리합니다.

이 구조에서 호스트 OS가 살아 있으면 관리 웹도 살아 있고, 호스트 OS 자체가 종료되면 관리 웹도 함께 종료됩니다.
VM이나 컨테이너 같은 게스트의 상태는 호스트 OS 내부의 Dionysus 서비스가 관찰하고 제어합니다.

systemd rootfs의 기본 관리 화면은 Perl 기반 `dionysus-pveproxy` 가 제공합니다.

## Local LLM 최적화 방향

첫 Local LLM 대상은 Ollama입니다. Dionysus는 Ollama를 직접 대체하지 않고, OS 레벨에서 모델 serving에 필요한
메모리 조건을 조정합니다.

초기 최적화 단위는 다음과 같습니다.

- Ollama API(`/api/tags`)와 `/proc` 프로세스 스캔으로 실행 상태, 모델 수, RSS, swap 사용량을 관찰한다.
- `dionysus-llm-swap.service` 로 dedicated swap 파일을 먼저 켜고, Ollama가 RAM 부족 시 사용할 backing store를 확보한다.
- Proxmox-style `/api2/json/nodes/localhost/ollama/optimize` API로 `vm.swappiness`, `vm.page-cluster`, `vm.vfs_cache_pressure`, `vm.watermark_scale_factor`, THP 정책을 조정한다.
- UI/API는 "무조건 빠른 swap"처럼 표현하지 않고, KV-cache overflow와 모델 page cache 보존을 위한 운영 프로필로 설명한다.

KV-cache는 latency-sensitive anonymous memory이므로 swap은 성능 향상 장치가 아니라 RAM 한계를 넘을 때의
bounded overflow 장치입니다. 따라서 Dionysus는 swap 사용량, readahead 설정, swappiness, Ollama process swap 사용량을
계속 보여주고, swap 포화나 과도한 stall 위험을 경고해야 합니다.

## 설계 원칙

- upstream first: 가능한 한 Linux mainline을 그대로 사용한다.
- fork last: Dionysus 기능 때문에 장기 커널 포크를 만들지 않는다.
- user space first: 가능하면 agent와 백엔드에서 해결한다.
- narrow kernel surface: 정말 필요한 경우에만 kernel module, eBPF, config fragment를 추가한다.
- reproducible base: 어떤 Linux commit과 config를 기준으로 했는지 항상 기록한다.

## Dionysus 자원 모델

Dionysus는 Linux 내부 구조를 그대로 UI에 노출하지 않고, 운영자가 다루기 쉬운 자원으로 재모델링합니다.

첫 단계 자원은 아래처럼 제한합니다.

| 자원 | Linux 원천 | Dionysus 역할 |
| --- | --- | --- |
| `system_memory_map` | `/proc/iomem`, `/proc/meminfo` | 메모리 지형도와 사용량 표시 |
| `kernel_health` | `/proc`, `/sys`, `dmesg` 기반 정보 | 제어 평면 상태 확인 |
| `lab_buffer` | Dionysus 전용 module/device 또는 initramfs lab region | 안전한 읽기/쓰기 실험 대상 |
| `service_status` | `systemd`, `/proc`, socket 상태 | 이후 운영 기능 확장 기반 |

중요한 점은 "임의의 Linux 커널 메모리 편집기"를 목표로 하지 않는다는 것입니다.
Dionysus가 수정 가능한 대상은 반드시 Dionysus가 소유하거나 명시적으로 노출한 자원이어야 합니다.

## 메모리 관찰과 수정 정책

기존 커스텀 커널 초안은 `MemoryRegion` 기반 읽기/쓰기 모델을 정의했지만,
Linux 기반에서는 그 모델을 다음처럼 조정합니다.

- 관찰은 Linux가 이미 노출하는 메모리/상태 인터페이스를 우선 사용한다.
- 수정은 Dionysus 전용 `lab_buffer` 같은 명시적 실험 자원에만 허용한다.
- 일반 `kernel_text`, `kernel_data`, page table, scheduler state 같은 영역은 수정 대상에서 제외한다.
- 절대 주소 기반 raw write API는 제품 레벨에서 제공하지 않는다.

즉, Dionysus의 "patch"는 Linux 자체를 해킹하는 기능이 아니라,
Linux 위에서 안전하게 통제된 실험 자원을 변경하는 기능이어야 합니다.

## 첫 번째 수직 슬라이스

Linux 베이스 전환 이후 첫 구현 범위는 다음으로 제한합니다.

1. upstream Linux 커널과 initramfs로 부팅한다.
2. initramfs에서 Dionysus agent를 시작한다.
3. agent가 `system_memory_map` 과 `kernel_health` 를 수집한다.
4. agent가 Dionysus 전용 `lab_buffer` 를 읽고 쓸 수 있는 제한된 API를 제공한다.
5. Perl API2 daemon과 pveproxy UI는 이 자원만 노출한다.

이 수직 슬라이스로 증명하려는 것은 다음입니다.

- Dionysus가 Linux 위에서 정상 부팅하고 시작될 수 있는가
- Linux 표준 인터페이스를 Dionysus 자원 모델로 안정적으로 변환할 수 있는가
- 안전한 실험 자원에 대해서만 제한된 patch 플로우를 끝까지 연결할 수 있는가

## 구현 우선순위

### 1. 베이스 시스템 고정

- 어떤 upstream Linux 브랜치/commit을 기준으로 삼을지 결정
- 최소 부팅 가능한 kernel config fragment 정의
- initramfs 빌드 방식 정의

### 2. Dionysus agent 도입

- 부팅 후 가장 먼저 실행되는 사용자 공간 agent 작성
- `/proc`, `/sys`, `netlink` 기반 상태 수집기 작성
- health endpoint와 memory-map endpoint 추가
- 첫 진입점은 initramfs의 `/init` 와 `dionysus-agent bootstrap` 경로로 시작한다

### 3. 실험용 제어 자원 추가

- Dionysus 소유 `lab_buffer` 설계
- user space만으로 충분한지 검토
- 부족하면 최소 kernel module 또는 eBPF 사용

### 4. 제어 평면 확장

- Perl API2 daemon에서 인증, audit, rate limit 추가
- pveproxy UI에서 읽기 전용 상태와 제한된 patch UX 구현

## 금지된 방향

다음 방향은 Linux 베이스 전략과 맞지 않으므로 피합니다.

- 기존 부트 프로토타입 위에 스케줄러, 메모리 관리자, 드라이버를 직접 계속 확장하는 것
- Dionysus 기능을 위해 Linux 전체를 장기 포크하는 것
- 표준 Linux 인터페이스로 해결 가능한 문제를 무조건 kernel patch로 푸는 것
- 운영자에게 arbitrary kernel memory write 기능을 제공하는 것

## 현재 저장소에 대한 해석

초기 freestanding 부트 프로토타입은 저장소에서 제거되었고,
현재 부팅 경로는 `makefile`, `config/grub-linux.cfg`, `initramfs/overlay`, `upstream/linux` 를 기준으로 유지합니다.
즉, 저장소에서 실제 제품 부팅 경로로 취급하는 것은 Linux 베이스 하나뿐입니다.

즉, 앞으로의 제품 방향은 다음과 같습니다.

- 제품 베이스: upstream Linux
- 제품 가치: Dionysus control plane
