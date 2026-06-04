# Code Convention

## 목적

이 저장소의 코드는 초기 부트 경로와 커널 진입 지점을 다루므로, 일반 애플리케이션 코드보다 가정과 부작용을 더 명확하게 드러내야 합니다.
간결함보다 예측 가능성과 검증 가능성을 우선합니다.

## 공통 원칙

- 코드 구조는 현재 저장소 규모에 맞게 단순하게 유지합니다.
- 하드웨어, ABI, 메모리 레이아웃에 대한 가정은 코드나 주석에서 명확해야 합니다.
- 바뀐 동작은 반드시 빌드 또는 부팅 경로에서 검증합니다.
- Linux host 전용 경로(`/proc`, `/sys`, `/sys/fs/cgroup`, Docker socket)는 상수로 박아두지 말고 테스트 가능한 형태로 주입 가능해야 합니다.

## Go/Svelte control-plane 규칙

- Proxmox와 비슷한 경로 이름, service 이름, `/api2/json` 응답 모양을 우선합니다.
- `/proc`, `/sys`, `/etc`, Ollama API 같은 외부 입력은 root 경로나 endpoint를 주입 가능하게 유지합니다.
- 읽기 API와 변경 API를 분리하고, sysctl/sysfs 변경은 어떤 값을 적용했는지 응답에 남깁니다.
- 네트워크 설정 API는 preview/save/apply를 분리하고, Wi-Fi PSK 같은 secret 값은 응답 JSON이나 UI 결과 테이블에 되돌려주지 않습니다.
- `cmd/dionysusd` 는 systemd에서 `api`, `proxy`, `metricsd` subcommand로 실행되는 단일 Go 제어 평면 바이너리입니다.
- 호환용 `pve/bin/dionysus-*` wrapper에는 비즈니스 로직을 두지 않고 `dionysusd` 호출만 남깁니다.
- daemon entrypoint는 명령행 인자와 systemd 환경 파일을 동시에 지원합니다.
- `dionysusd` 는 기본적으로 Linux/systemd target OS 밖에서 실행되지 않아야 하며, 로컬 UI 확인은 `--dev-allow-host` 같은 명시적 개발 override로만 허용합니다.
- SQLite, `/proc`, `/sys` 접근은 테스트에서 임시 root와 임시 DB로 검증 가능해야 합니다.
- host installer가 `apt-get` 같은 패키지 관리자를 실행할 때는 opt-in 인자 또는 환경 변수가 있어야 하며, staged rootfs 설치에서는 호스트 패키지를 변경하지 않습니다.

## Svelte operator UI 규칙

- 제품 기본 UI는 Svelte로 구현하고, 빌드 결과는 `pve/www` 에 둬서 `dionysusd proxy` 가 OS 내부에서 직접 제공할 수 있어야 합니다.
- 프런트엔드 개발 서버는 개발 편의용일 뿐이며 제품 경로가 아닙니다. 제품 경로는 systemd가 실행하는 Go `dionysusd proxy` 와 정적 Svelte assets입니다.
- 바이트, swap, service state처럼 단위가 중요한 값은 화면에서 일관되게 포맷합니다.
- 운영자 UI는 "경고", "현재 상태", "적용 액션"을 동시에 보여줘야 하며, 성공/실패 메시지를 숨기지 않습니다.
- 위험한 변경 버튼은 대응하는 `/api2/json` 작업과 결과를 화면 상태에 반영해야 합니다.
- 네트워크 변경처럼 접속성을 끊을 수 있는 작업은 preview와 explicit apply 버튼을 분리하고, 저장된 secret 여부만 표시합니다.
- KV-cache, 네트워크 같은 운영 액션은 사람이 읽을 수 있는 결과를 웹 콘솔에 남기고, preview와 실제 apply 결과를 구분해 표시합니다.

## Go 코드 규칙

- package 바깥으로 공개할 필요가 없는 식별자는 소문자로 유지합니다.
- Linux host 전용 접근은 `Config` 를 통해 주입 가능한 root path와 endpoint를 사용합니다.
- HTTP handler는 Proxmox-style wrapper `{ "data": ... }` 와 token auth 실패 응답 모양을 유지합니다.
- 변경 API는 dry-run 결과와 실제 적용 결과를 모두 테스트 가능해야 합니다.
- 표준 라이브러리로 충분하면 외부 Go module을 추가하지 않습니다.

## Rust 코드 규칙

- 함수, 변수, 모듈은 `snake_case`, 타입과 trait은 `UpperCamelCase`, 상수와 정적 값은 `UPPER_SNAKE_CASE`를 사용합니다.
- crate 바깥으로 공개할 필요가 없는 항목은 `pub`을 붙이지 않습니다. 공개 API는 최소 범위로 유지합니다.
- 하드웨어 레지스터, 포트, ABI 값에는 `u8`, `u16`, `u32`, `usize` 같은 고정 폭 또는 명시적 크기 타입을 우선합니다.
- `unsafe`는 필요한 가장 작은 범위로 제한하고, 왜 안전한지 코드나 짧은 주석으로 근거를 남깁니다.
- 한 함수는 한 가지 책임만 가지도록 유지하고, 초기화 순서나 부트 단계처럼 순서가 중요한 로직은 이름과 구조에서 드러나게 만듭니다.
- 직접적인 숫자 리터럴은 의미 있는 상수나 새 타입으로 끌어올립니다. 단, x86 포트 번호처럼 널리 알려진 값은 근처에 맥락을 둡니다.
- low-level Rust 코드를 추가할 때는 패닉, 할당, 표준 라이브러리 의존 여부를 실행 환경과 맞춰 명시합니다.

## Rust 포맷 규칙

- 기본 포맷은 `rustfmt` 출력을 따릅니다.
- 들여쓰기는 공백 4칸을 사용하고, 체이닝이 길어지면 줄바꿈으로 의미 단위를 드러냅니다.
- 조건문과 `match`의 각 분기는 축약형보다 명시적인 블록을 우선해 부작용과 반환값을 읽기 쉽게 유지합니다.
- 긴 타입 시그니처나 trait bound는 한 줄에 욱여넣지 말고 줄바꿈으로 책임 경계를 보이게 합니다.
- 로그나 출력 문자열은 사람이 바로 해석할 수 있게 쓰고, 부트 단계 이름이나 실패 지점을 포함해 디버깅 정보를 남깁니다.

## 주석 규칙

주석은 아래 상황에서만 추가합니다.

- 부트 프로토콜이나 calling convention처럼 코드만으로 드러나지 않는 계약이 있다.
- 메모리 주소나 정렬 제약처럼 실수하기 쉬운 전제가 있다.
- 의도가 구현보다 더 중요하다.

아래와 같은 주석은 피합니다.

- 코드 한 줄을 그대로 번역한 설명
- 이미 함수명으로 충분히 드러나는 설명

## Makefile 규칙

- 타깃 이름은 짧고 예측 가능하게 유지합니다.
- 외부 도구 경로는 지금처럼 변수로 override 가능해야 합니다.
- 실패 메시지는 다음 행동이 드러나게 작성합니다.
- `.PHONY` 대상은 명시적으로 관리합니다.

## 파일 분리 기준

- boot path, terminal, serial, memory, scheduler처럼 책임이 분리되기 시작하면 파일을 나눕니다.
- 지금 단계에서는 파일 수를 늘리기보다 책임 경계를 먼저 명확히 합니다.
- 파일 분리 시 public entry point와 internal helper를 구분합니다.

## 검증 기준

아래 변경은 최소 검증 기준을 만족해야 합니다.

- `makefile`, `config/grub-linux.cfg`, initramfs overlay, PVE control-plane boot path

권장 검증:

- Linux 기반 경로:
  - 빌드만 바뀐 경우: `make build`
  - ISO 패키징까지 바뀐 경우: `make iso`
  - 부트 흐름이나 출력이 바뀐 경우: `make` 또는 `make run`

검증이 불가능하면, 막힌 이유와 아직 확인하지 못한 위험을 PR에 적습니다.
