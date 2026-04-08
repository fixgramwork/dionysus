# Developer Control Plane

상태: 초안  
관련 이슈: developer control plane first design

## 목적

Dionysus의 첫 장기 목표는 부팅 가능한 커널 프로토타입을 개발자 친화적인 운영체제로 확장하는 것입니다.
이 문서는 그 출발점으로서 제어 평면의 책임 경계를 먼저 고정합니다.

이번 설계가 다루는 핵심 경험은 다음과 같습니다.

- 커널 메모리 상태를 시각적으로 확인한다.
- 메모리 영역의 타입과 내용을 안전하게 조회한다.
- 허용된 범위에서만 메모리 편집 요청을 수행한다.
- 패키지 설치, 개발 도구 설치, Local LLM 자동화, 서버 상태 확인 기능을 같은 제어 평면 위에서 확장한다.

## 설계 원칙

- 실제 메모리 소유권과 변경 권한은 Rust 커널이 가진다.
- 제어 요청은 반드시 명시적인 인터페이스를 통과한다.
- Go 서비스는 커널이 허용한 제어 인터페이스만 사용한다.
- Svelte UI는 상태 조회와 허용된 편집 요청의 사용자 경험만 담당한다.
- 자동화 기능도 같은 권한 모델과 감사 경로를 재사용한다.

## 계층 구조

```text
ASM -> Rust kernel -> control interface -> Go backend -> Svelte UI
```

이 체인은 구현 순서가 아니라 책임 전달 순서를 의미합니다.
아래 계층일수록 더 강한 권한과 더 낮은 수준의 상태를 직접 다룹니다.

## 계층별 책임

| 계층 | 주 책임 | 제공 결과 | 하지 말아야 할 일 |
| --- | --- | --- | --- |
| ASM | 부트스트랩, CPU 진입 경로, 초기 컨텍스트 준비, Rust 커널 진입 보장 | 커널이 해석 가능한 최소 부팅 상태와 하드웨어 진입점 | 정책 결정, 메모리 편집 권한 판단, 운영 기능 구현 |
| Rust kernel | 메모리 모델 소유, 권한 검증, 상태 스냅샷 생성, 제어 인터페이스 구현 | 타입이 있는 커널 상태와 검증된 제어 작업 | UI 포맷팅, 운영자 세션 관리, 외부 서비스 오케스트레이션 |
| Control interface | 커널 기능을 안정된 명령/응답 계약으로 노출 | 조회/수정 가능한 자원 목록, 오류 코드, 이벤트 계약 | 메모리 직접 소유, 비공식 우회 경로 제공 |
| Go backend | 인증, 인가, 감사 로그, rate limit, UI 친화적 API 제공 | HTTP/WebSocket API, 작업 기록, 시스템 집계 응답 | 커널 외부에서 임의 메모리 읽기/쓰기, 커널 정책 재정의 |
| Svelte UI | 메모리/서버 상태 시각화, 제한된 편집 플로우, 경고/승인 UX | 운영자 화면, diff 표시, 안전한 편집 요청 | 커널 직접 호출, 권한 우회, 정책 판단 중복 구현 |

## 제어 평면 관점에서의 확장 방향

첫 설계에서는 메모리 관찰과 제한된 수정만 다루지만, 같은 구조는 다른 운영 기능에도 그대로 적용합니다.

- 패키지 관리: 커널이 아닌 상위 제어 평면 기능으로 분리하고 Go 서비스가 작업 오케스트레이션을 담당한다.
- Local LLM 설치와 API 노출: 설치/실행 상태를 제어 평면 작업으로 모델링하고 UI는 상태와 실행 결과를 표시한다.
- 서버 상태 확인: 커널/호스트에서 수집한 메트릭을 Go가 집계하고 Svelte가 대시보드로 렌더링한다.

## 초기 아키텍처 결정

- 메모리 관찰과 수정 요청은 항상 "영역(region) 기반"으로 표현한다.
- Go와 Svelte는 절대 raw physical address만으로 메모리를 다루지 않는다.
- 첫 수직 슬라이스는 "안전하게 관찰 가능한 영역"과 "명시적으로 수정 허용된 영역"을 구분하는 데 집중한다.
- 제어 평면의 이후 기능도 동일한 자원 모델, 권한 모델, 감사 모델 위에서 쌓는다.

## 메모리 영역 데이터 모델

메모리 상태는 커널이 소유하는 `MemoryRegion` 집합으로 표현합니다.
영역 메타데이터와 영역 내용은 분리하며, UI와 백엔드는 메타데이터 없이 raw dump만 받지 않습니다.

### `MemoryRegion`

| 필드 | 설명 |
| --- | --- |
| `region_id` | 커널이 부여한 안정적인 식별자 |
| `label` | 운영자가 이해할 수 있는 이름 |
| `kind` | `kernel_text`, `kernel_data`, `kernel_heap`, `kernel_stack`, `boot_info`, `free`, `reserved`, `mmio`, `framebuffer`, `scratch`, `unknown` 중 하나 |
| `address_space` | `physical` 또는 `virtual` |
| `start` / `end` | 바이트 기준 시작/끝 주소 |
| `size_bytes` | 영역 크기 |
| `permissions` | 현재 CPU/커널 관점의 `read`, `write`, `execute` 조합 |
| `visibility` | `summary`, `inspectable`, `patchable`, `hidden` 중 하나 |
| `owner` | `boot`, `kernel`, `allocator`, `device`, `operator-lab` 중 하나 |
| `version` | 내용이 바뀔 때 증가하는 세대 번호 |
| `preview_limit_bytes` | 한 번에 조회 가능한 최대 바이트 수 |
| `patch_limit_bytes` | 한 번에 수정 가능한 최대 바이트 수 |

### `MemoryRead`

| 필드 | 설명 |
| --- | --- |
| `region_id` | 조회 대상 영역 |
| `offset` | 영역 시작 기준 상대 오프셋 |
| `length` | 조회 길이 |
| `format` | `hex`, `ascii`, `u32` 같은 표현 형식 |
| `version` | 읽은 시점의 영역 세대 번호 |
| `truncated` | 정책상 잘린 응답인지 여부 |
| `bytes` | 조회된 원시 데이터 |

### `MemoryPatch`

| 필드 | 설명 |
| --- | --- |
| `region_id` | 수정 대상 영역 |
| `offset` | 영역 시작 기준 상대 오프셋 |
| `before` | 기대하는 기존 바이트 값 |
| `after` | 적용하려는 새 바이트 값 |
| `expected_version` | 수정 전 확인한 세대 번호 |
| `reason` | 운영자가 남기는 변경 사유 |
| `requested_by` | 감사용 요청 주체 식별자 |

## 최소 조회 API 범위

첫 제어 평면 버전에서는 아래 조회 작업만 지원합니다.

| 작업 | 설명 | 기본 제약 |
| --- | --- | --- |
| `ListMemoryRegions` | 전체 메모리 영역 목록과 메타데이터 조회 | `visibility != hidden` 인 영역만 노출 |
| `DescribeMemoryRegion` | 단일 영역의 상세 메타데이터 조회 | 주소, 권한, 세대 번호 포함 |
| `ReadMemoryWindow` | 지정 영역의 일부 바이트 조회 | `length <= preview_limit_bytes` |
| `GetControlPlaneHealth` | 제어 평면 통신 상태 확인 | 메모리 내용은 포함하지 않음 |

Go 백엔드는 이를 다음과 같은 운영자 API로 변환합니다.

- `GET /api/v1/memory/regions`
- `GET /api/v1/memory/regions/{region_id}`
- `GET /api/v1/memory/regions/{region_id}/read?offset=0&length=128&format=hex`
- `GET /api/v1/control-plane/health`

## 최소 수정 API 범위

첫 버전에서 허용하는 수정 작업은 하나뿐입니다.

| 작업 | 설명 | 기본 제약 |
| --- | --- | --- |
| `PatchMemoryWindow` | 명시적으로 patchable 로 분류된 영역의 제한된 바이트 수정 | `length <= patch_limit_bytes`, `before` 일치, `expected_version` 일치 |

Go 백엔드는 이를 아래 API로 노출합니다.

- `POST /api/v1/memory/patches`

요청 본문은 `region_id`, `offset`, `before`, `after`, `expected_version`, `reason`을 포함해야 합니다.

## Go 백엔드와 Svelte UI 데이터 흐름

1. Svelte UI가 영역 목록을 요청한다.
2. Go 백엔드는 운영자 인증과 권한을 확인한 뒤 커널 제어 인터페이스로 `ListMemoryRegions`를 전달한다.
3. 사용자가 특정 영역을 선택하면 UI가 read 요청을 보내고, Go는 이를 `ReadMemoryWindow`로 변환한다.
4. 커널은 영역 정책을 검증한 뒤 메타데이터와 함께 읽기 결과를 반환한다.
5. UI는 hex/ascii 뷰, 영역 타입, 권한, 수정 가능 여부를 한 화면에 보여준다.
6. 사용자가 수정 요청을 보내면 Go 백엔드는 권한, 요청 크기, 감사 필드를 확인한 뒤 `PatchMemoryWindow`를 전달한다.
7. 커널은 `before` 값과 `expected_version`을 검증하고 성공 시 새 `version`과 변경 결과를 반환한다.
8. UI는 성공 결과와 diff를 표시하고, 같은 영역을 다시 읽어 최신 상태를 동기화한다.

이 흐름은 이후 패키지 관리, Local LLM 자동화, 서버 상태 확인 기능에도 동일하게 적용합니다.
차이는 "메모리 영역" 대신 "작업(job)", "서비스(service)", "시스템 상태(system status)" 같은 자원을 다룬다는 점뿐입니다.

## 권한 모델과 보호 장치

첫 설계에서는 세 가지 역할을 둡니다.

| 역할 | 허용 작업 |
| --- | --- |
| `observe` | 영역 목록 조회, 영역 상세 조회, 제한된 읽기 |
| `patch-lab` | `observe` 권한 + patchable 영역의 제한된 수정 |
| `operate` | 향후 패키지/LLM/서비스 작업 실행, 첫 메모리 슬라이스에는 직접 사용하지 않음 |

핵심 보호 장치는 다음과 같습니다.

- 커널만이 영역의 `visibility` 와 `patch_limit_bytes` 를 결정한다.
- 모든 수정 요청은 `region_id + offset` 기준으로만 수행하며 절대 주소 직접 쓰기를 허용하지 않는다.
- 수정 전 `before` 값과 `expected_version` 이 동시에 일치해야 한다.
- 첫 버전의 patchable 영역은 `scratch` 또는 `operator-lab` 소유 영역으로 제한한다.
- Go 백엔드는 인증, 인가, 감사 로그, 요청 rate limit 을 담당한다.
- UI는 "수정 가능" 배지와 확인 절차를 명시하되, 실제 정책은 커널과 백엔드에서 강제한다.

## 금지된 작업

다음 작업은 첫 설계에서 명시적으로 금지합니다.

- `kernel_text`, `kernel_stack`, `boot_info`, `mmio`, `framebuffer`, `unknown` 영역 수정
- 영역 경계를 넘는 읽기/쓰기
- `region_id` 없이 physical address만으로 읽기/쓰기
- 대용량 전체 메모리 덤프
- 커널이 소유하지 않은 보호 페이지 테이블, 인터럽트 디스크립터, 스케줄러 상태 수정
- Go 서비스가 커널 검증 없이 캐시된 메모리 값을 진실로 취급하는 것
- Svelte UI가 커널/Go 승인 없이 로컬 상태만 바꿔 "적용된 것처럼" 표시하는 것

## 첫 번째 수직 슬라이스

첫 구현 이슈는 다음 범위를 가집니다.

- Rust 커널이 정적 `MemoryRegion` 목록을 노출한다.
- 그 목록에는 최소 하나의 `inspectable` 영역과 하나의 `patchable` scratch 영역이 포함된다.
- 제어 인터페이스는 `ListMemoryRegions`, `ReadMemoryWindow`, `PatchMemoryWindow` 를 지원한다.
- Go 백엔드는 위 작업을 HTTP API로 중계하고 감사 필드를 기록한다.
- Svelte UI는 영역 목록, 상세 메타데이터, hex preview, scratch patch 폼을 제공한다.

이 수직 슬라이스의 목표는 "임의 메모리 편집기"가 아니라 "커널 소유 정책 아래에서 안전한 관찰과 제한된 수정이 끝까지 관통되는지"를 증명하는 것입니다.

## 후속 이슈 분리 기준

후속 작업은 아래 기준으로 쪼갭니다.

- 새로운 자원 타입을 도입하면 별도 이슈로 분리한다.
- 새로운 권한 클래스나 위험한 수정 권한이 생기면 별도 이슈로 분리한다.
- Rust 커널, Go 백엔드, Svelte UI를 동시에 크게 바꾸는 작업은 vertical slice 단위로 자른다.
- 구현과 정책 정의를 한 이슈에 섞지 않는다.

다음 이슈 후보는 아래처럼 정리할 수 있습니다.

- `[proposal] expose kernel-owned memory region registry`
- `[proposal] define transport for control plane memory operations`
- `[proposal] add Go gateway for memory inspect and patch requests`
- `[proposal] build Svelte memory inspector vertical slice`
- `[proposal] model package management jobs on the control plane`
- `[proposal] model Local LLM lifecycle automation on the control plane`
- `[proposal] add unified system status resources for the dashboard`
