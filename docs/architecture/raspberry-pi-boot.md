# Raspberry Pi Boot Path

상태: 초안

## 목적

Raspberry Pi는 현재 x86_64 개발 경로처럼 GRUB ISO를 직접 부팅하지 않습니다.
Pi firmware가 FAT boot partition의 `config.txt`, `cmdline.txt`, kernel image, device tree, initramfs를 읽어 Linux를 시작합니다.

Dionysus의 Pi 경로는 같은 Linux 기반 제품 방향을 유지하되, 산출물만 Pi firmware 형식으로 준비합니다.

```text
Raspberry Pi firmware -> ARM64 Linux Image + DTB -> Dionysus initramfs -> rootfs systemd -> PVE control plane
```

## 기본 타깃

첫 번째 기본값은 Raspberry Pi 4 계열 64-bit입니다.

- Linux checkout: `upstream/raspberrypi-linux`
- Linux remote: `https://github.com/raspberrypi/linux.git`
- config target: `bcm2711_defconfig`
- kernel filename: `kernel8.img`
- output: `build/raspi/boot`

Raspberry Pi 5 계열은 아래처럼 명시적으로 바꿉니다.

```bash
make raspi-boot RASPI_CONFIG_TARGET=bcm2712_defconfig RASPI_KERNEL_NAME=kernel_2712.img
```

## 빌드 흐름

```bash
make raspi-fetch
make raspi-boot
```

`make raspi-boot` 는 다음을 수행합니다.

1. Dionysus initramfs를 ARM64용으로 패킹한다.
2. Raspberry Pi Linux `Image` 와 DTB를 빌드한다.
3. `build/raspi/boot` 에 Pi boot partition overlay를 만든다.

생성되는 주요 파일은 다음입니다.

- `kernel8.img` 또는 `RASPI_KERNEL_NAME`
- `dionysus-initramfs.cpio.gz`
- `config.txt`
- `cmdline.txt`
- `*.dtb`
- `overlays/*`

이 디렉터리의 파일을 Raspberry Pi FAT boot partition에 복사합니다.
boot partition이 Raspberry Pi firmware 파일을 이미 가지고 있지 않다면, `RASPI_FIRMWARE_DIR=/path/to/firmware/boot` 를 지정해 함께 복사합니다.

```bash
RASPI_FIRMWARE_DIR=/path/to/firmware/boot make raspi-boot
```

## 부팅 설정

Pi용 `config.txt` 템플릿은 `config/raspberry-pi/config.txt` 입니다.
핵심 설정은 다음입니다.

- `arm_64bit=1`
- `kernel=kernel8.img`
- `initramfs dionysus-initramfs.cpio.gz followkernel`
- `enable_uart=1`

Pi용 `cmdline.txt` 템플릿은 한 줄이어야 합니다.
현재 값은 다음입니다.

```text
console=serial0,115200 console=tty1 printk.time=1 rdinit=/init
```

## Initramfs 동작

Pi 부팅 후 initramfs의 `/init` 은 다음 순서로 실행됩니다.

1. `/proc`, `/sys`, `/dev`, cgroup v2를 마운트한다.
2. `dionysus-agent bootstrap` 으로 초기 상태를 캡처한다.
3. `dionysus-network start` 로 `eth0` DHCP를 best-effort로 시도한다.
4. `dionysus-services start` 로 initramfs와 rootfs service 경계를 출력한다.
5. 실패해도 rescue shell로 진입한다.

다른 NIC 이름을 써야 하면 kernel command line이나 init 환경에서 `DIONYSUS_NET_IFACE=<iface>` 를 넘겨야 합니다.

## 한계

이 경로는 아직 완전한 Raspberry Pi 배포판 이미지가 아닙니다.

- root filesystem partition을 만들지 않는다.
- kernel modules를 rootfs에 설치하지 않는다.
- Wi-Fi firmware와 userspace 네트워크 관리자를 포함하지 않는다.
- 보안 부팅, A/B 업데이트, 영속 설정 저장소를 제공하지 않는다.

따라서 첫 목표는 "Pi에서 ARM64 Linux + Dionysus initramfs가 시작되고 rootfs에서 PVE control plane을 올릴 수 있는가" 입니다.
운영 가능한 서버 OS로 확장하려면 rootfs, modules install, persistent state, SSH 또는 인증된 원격 API, 업데이트/롤백 전략을 별도 이슈로 나눠야 합니다.
