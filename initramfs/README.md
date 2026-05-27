# Initramfs Overlay

이 디렉터리는 Linux 기반 Dionysus 부팅 실험에서 initramfs에 포함할 최소 사용자 공간 overlay를 담습니다.

현재 포함 범위:

- `/init`: 첫 사용자 공간 진입점
- `/usr/bin/dionysus-agent`: 부팅 직후 기본 상태를 수집하는 최소 agent
- `/usr/bin/dionysus-network`: 유선 NIC DHCP를 best-effort로 시도하는 최소 네트워크 훅
- `/usr/bin/dionysus-services`: initramfs와 rootfs systemd 제어 평면의 역할 경계를 출력하는 훅
- `/etc/dionysus-release`: 베이스 식별 정보

initramfs는 더 이상 관리 웹/API daemon을 직접 포함하지 않습니다.
초기 부팅 단계는 bootstrap capture, DHCP 시도, rescue shell 진입까지만 담당합니다.
Proxmox-style 관리 웹은 일반 Linux rootfs에 systemd service로 설치합니다.

- `dionysus-pvedaemon.service`: localhost API daemon
- `dionysus-pveproxy.service`: `0.0.0.0:8006` 관리 웹과 `/api2/json` API
- `dionysus-network.service`: `/etc/dionysus/network.env` 기반 영구 Wi-Fi 설정
- `dionysus-llm-swap.service`: Ollama 시작 전 dedicated swap backing store 준비

Local LLM 최적화는 initramfs 안에서는 상태 확인 중심으로 동작합니다. 일반 Linux rootfs에서는
`dionysus-llm-swap.service` 가 dedicated swap 파일을 만들고, PVE control plane이 Ollama 상태와
KV-cache/swap 권장값을 `/api2/json` 및 웹 UI로 노출합니다.
영구 Wi-Fi 설정은 initramfs가 아니라 일반 Linux rootfs의 `/etc/dionysus/network.env` 와
`dionysus-network.service` 에서 처리합니다.

## 빌드 방법

호스트에 BusyBox가 있거나, `BUSYBOX_BIN` 으로 경로를 지정해야 부팅 가능한 initramfs를 만들 수 있습니다.

예시:

```bash
BUSYBOX_BIN=/path/to/busybox make linux-initramfs
```

BusyBox 없이 overlay layout만 확인하려면:

```bash
make linux-initramfs-layout
```

macOS처럼 Docker Desktop이 꺼져 있고 BusyBox 자동 추출을 피해야 하는 환경에서는 layout 검증만 다음처럼 실행할 수 있습니다.

```bash
BUSYBOX_BIN=/does/not/exist make linux-initramfs-layout
```
