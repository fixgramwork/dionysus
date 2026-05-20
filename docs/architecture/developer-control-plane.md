# Developer Control Plane

상태: superseded  
관련 이슈: developer control plane first design

이 문서는 초기 자체 커널 제어 평면 초안이 있었음을 기록하기 위한 얇은 포인터입니다.
현재 제품 기준은 upstream Linux 기반의 Proxmox-style control plane입니다.

최신 기준은 [`linux-base-control-plane.md`](linux-base-control-plane.md) 를 따릅니다.

현재 유효한 제어 평면 계층은 다음과 같습니다.

```text
Bootloader/Firmware -> Linux kernel -> kernel interfaces -> Dionysus agent -> Perl API2 daemon -> pveproxy -> ExtJS-style UI
```

이전 자체 커널/별도 웹 개발 서버 경로는 제품 경로에서 제외되었습니다.
