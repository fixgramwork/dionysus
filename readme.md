<div align="center">
  <h1>Dionysus</h1>
  <p><strong>Bootable prototype, setup expectations, and release intent in one place.</strong></p>
  <p>Early-stage repository with a minimal bootable kernel scaffold and an ISO-first local workflow.</p>
  <p>
    <img src="https://img.shields.io/badge/status-prototype-1f6feb?style=for-the-badge" alt="Status: Prototype" />
    <img src="https://img.shields.io/badge/docs-readme_ready-0a7ea4?style=for-the-badge" alt="Docs: README Ready" />
    <img src="https://img.shields.io/badge/contributions-welcome-2da44e?style=for-the-badge" alt="Contributions: Welcome" />
  </p>
  <p>
    <a href="#at-a-glance">At a Glance</a> •
    <a href="#project-status">Status</a> •
    <a href="#roadmap">Roadmap</a> •
    <a href="#contributing">Contributing</a>
  </p>
</div>

## At a Glance

| Area | Current State |
| --- | --- |
| Repository | `fixgramwork/dionysus` |
| Stage | Prototype |
| Codebase | Linux-based boot path, Raspberry Pi boot overlay, initramfs, and Proxmox-style control plane |
| Focus | Rebase the long-term platform direction on upstream Linux |
| Next milestone | Boot upstream Linux with initramfs and start the first Dionysus agent slice |

## Overview

Dionysus is currently being prepared as a focused project under `fixgramwork`.
At this stage, the repository serves as the public-facing source of truth for:

- project intent
- initial scope
- contributor expectations
- release readiness

The long-term product direction is a developer-friendly Linux-based operating environment with a Proxmox-style control plane for memory inspection, controlled mutation of Dionysus-owned lab resources, package automation, Local LLM lifecycle management, and server status visibility.
The current Local LLM direction uses Ollama as the first target service and optimizes model serving around KV-cache pressure, model file cache retention, and a Dionysus-managed swap backing store.

This README is intentionally structured like a production open-source repository so the project can scale without rewriting its documentation from scratch.

## Principles

- Keep the first version narrow and shippable.
- Prefer obvious setup over clever tooling.
- Add quality gates early: linting, testing, and CI.
- Document decisions as the repository grows.

## Architecture Direction

The first system design pass fixes the control-plane boundary as:

```text
Bootloader/Firmware -> Linux kernel -> kernel interfaces -> Dionysus agent -> dionysusd API daemon -> pveproxy-compatible UI
```

- Linux becomes the long-term kernel and hardware base.
- Dionysus agent translates Linux interfaces into a product-specific resource model.
- Go `dionysusd` exposes Proxmox-style `/api2/json` control-plane APIs and audit-friendly workflows.
- `dionysusd proxy` serves the Svelte management web on port `8006` from inside the target OS.
- Future package, Local LLM, and system status features extend the same control-plane model instead of introducing side channels.

See [`docs/architecture/linux-base-control-plane.md`](docs/architecture/linux-base-control-plane.md) for the current architecture draft.
See [`docs/architecture/raspberry-pi-boot.md`](docs/architecture/raspberry-pi-boot.md) for the Raspberry Pi ARM64 boot path.

## Boot Direction

The repository boot direction is now Linux-only:

- Linux kernel at the base
- Dionysus initramfs and userspace on top
- Dionysus PVE API daemon, pveproxy, and management UI as the product layer

## Planned Deliverables

| Track | What will be added |
| --- | --- |
| Application | Dionysus agent, Go API2 daemon, Svelte pveproxy-style UI, and memory inspection primitives |
| Developer Experience | Linux initramfs, PVE control-plane install/run/test automation |
| Quality | Formatting, linting, and CI checks |
| Documentation | Usage examples, architecture notes, and control-plane contracts |
| Release | Versioning and deployment guidance |

## Project Status

This repository now contains a minimal bootable prototype.

Before `v0.1.0`, the project should include:

- runnable source code
- reproducible local setup
- a basic test flow
- usage examples
- a license file

## Quick Start

Primary Linux-based workflow:

```bash
git clone https://github.com/fixgramwork/dionysus.git
cd dionysus

# clone upstream Linux
make linux-fetch LINUX_REF=master

# prepare the initramfs layout
make linux-initramfs-layout

# build a bootable initramfs when BusyBox is available
BUSYBOX_BIN=/path/to/busybox make linux-initramfs

# on a Linux build host, build/package/boot the OS
make
```

The default Make target is `run`, so `make` now follows the Linux-based OS path and resolves to `make linux-run`.
On non-Linux hosts, `make build`, `make iso`, and `make run` automatically fall back to a Docker-based Linux builder when Docker Desktop is running.
The early initramfs path now performs bootstrap capture, best-effort networking, and rescue shell entry only.
The management web is installed into the normal Ubuntu/Debian Linux root filesystem as systemd services.

For a normal Linux root filesystem with systemd, stage Dionysus as an OS-managed service:

```bash
make pve-control-plane-install
```

That target installs the Go `dionysusd` API/proxy/metrics daemon, Svelte assets, workload profiles, persistent network defaults, and systemd units into `build/pve-control-plane-rootfs`.
For a real Ubuntu/Debian Ollama server, install directly on the host with:

```bash
sudo sh scripts/install-pve-control-plane.sh --host
```

The host install creates `/etc/dionysus/pve.token` as the JWT signing key plus `/etc/dionysus/pve.users.json` for web console accounts. Legacy `/etc/dionysus/pve.user` and `/etc/dionysus/pve.password` files are still written for initial fallback compatibility. It enables `dionysus-llm-swap.service`, `dionysus-metricsd.service`, `dionysus-pvedaemon.service`, and `dionysus-pveproxy.service`, then serves the management web from inside the OS on port `8006`.
The installer checks required packages and prints `apt-get` commands when dependencies are missing.
To let the host install download and install missing packages through `apt-get`, run:

```bash
sudo sh scripts/install-pve-control-plane.sh --host --install-deps
```

The `--install-deps` path runs `apt-get update` and `apt-get install -y --no-install-recommends ...` only during direct host install.
The management UI first signs in through `/api2/json/access/ticket`, stores the returned JWT in the browser, and then connects directly to the host OS through `dionysusd`: `/proc`, `/sys`, and `/etc/os-release` populate the node OS panel, while the same status response reports the active web listener, static web root, metrics database, network config path, and JWT-auth state.
`dionysus-metricsd` records Ollama RAM samples every 30 seconds in `/var/lib/dionysus/metrics/ollama.sqlite3` and keeps 7 days by default.
`dionysus-llm-swap.service` creates and enables a dedicated swap file before `ollama.service`, `dionysus-pvedaemon.service`, and `dionysus-pveproxy.service`.
The web UI includes a KV-cache optimization panel that compares current kernel values against the built-in Ollama KV-cache profile, previews changes, applies them manually, and records operator-visible output in the web console.
The web UI also includes a Users page for adding console accounts, rotating passwords, and deleting non-current users without editing files by hand.
`dionysusd` fails closed outside a Linux/systemd target OS by default. Local UI-only development must be explicit with `--dev-allow-host` or `DIONYSUS_DEV_ALLOW_HOST=1`.

For a QEMU OS image where `apt` works inside the guest, build the Debian ARM64 rootfs:

```bash
make debian-rootfs
make debian-qemu-run
```

The Debian path writes an ext4 disk image to `build/debian-arm64/dionysus-debian-arm64.ext4`,
copies the Debian kernel/initrd to `build/debian-arm64/vmlinuz` and `build/debian-arm64/initrd.img`,
and installs Dionysus as systemd services inside the rootfs. The guest exposes the management UI at
`http://127.0.0.1:18106` through QEMU port forwarding. The root password is `dionysus` by default
and can be changed with `DEBIAN_ROOT_PASSWORD=...`. QEMU also forwards SSH on port `10022`, so the
default local login is `ssh root@127.0.0.1 -p 10022`.
The web console login defaults to `root` / `dionysus` and can be changed at image build or install time with
`DIONYSUS_PVE_USERNAME=...` and `DIONYSUS_PVE_PASSWORD=...`.

The Debian rootfs build uses Docker automatically on non-Linux hosts because it needs `debootstrap`
and `mkfs.ext4`. It installs `apt`, `systemd`, `linux-image-arm64`, networking tools, `sqlite3`,
the Go/Svelte Dionysus control plane, and Ollama by default. Set `DIONYSUS_DEBIAN_INCLUDE_OLLAMA=0`
to skip the large Ollama payload. The web console defaults to a 300-second command timeout through
`DIONYSUS_CONSOLE_TIMEOUT_SECONDS`, which leaves enough room for QEMU-hosted `apt` update/install
operations. The image also sets apt to prefer IPv4 and skip translation indexes to keep QEMU package
metadata refreshes predictable.

Persistent LAN and Wi-Fi settings are managed by the web UI on port `8006` or by editing `/etc/dionysus/network.env`.
The UI exposes an explicit preview, save, and save-and-apply flow for `dionysus-network.service`.
LAN and Wi-Fi are opt-in so an existing server network stack is not changed until an operator enables a mode.
When Wi-Fi is enabled, `dionysus-network.service` renders a private `wpa_supplicant` config, brings the selected interface up, and applies DHCP or static IPv4 from the same file before the management web starts.

## Linux Base Workflow

If the product direction is "Ubuntu-style development on top of Linux", treat this repository as a distro/product layer around upstream Linux instead of a custom kernel tree.

Current repository convention:

- upstream Linux checkout: `upstream/linux`
- tracked Dionysus deltas: config fragments, optional patches, initramfs, agent, Go PVE API daemon, and Svelte UI
- current Linux fragment: [`config/linux/x86_64-dionysus.fragment`](config/linux/x86_64-dionysus.fragment)

Bootstrap commands:

```bash
# fetch the kernel source that sits under Dionysus
make linux-fetch LINUX_REF=master

# inspect the pinned upstream commit
make linux-status

# prepare Dionysus early userspace
make linux-initramfs-layout
BUSYBOX_BIN=/path/to/busybox make linux-initramfs

# stage the Proxmox-style control plane in a Linux root filesystem
make pve-control-plane-install

# build and boot the full Linux-based OS path
make build
make iso
make run
```

If the host is macOS, the Linux kernel build and BusyBox provisioning are routed through the container builder image defined in `scripts/docker/linux-builder.Dockerfile`.

For this model, prefer:

- keeping Linux as an upstream checkout
- recording the exact upstream commit SHA you build against
- storing only Dionysus-specific config, patch, initramfs, and userspace changes in this repository
- treating `initramfs/overlay/init` and `initramfs/overlay/usr/bin/dionysus-agent` as the first Linux-based execution path
- treating `config/grub-linux.cfg` as the first boot path for `bzImage + initramfs`

## Raspberry Pi Workflow

Raspberry Pi does not use the x86_64 GRUB ISO path. Build a Pi boot partition overlay instead:

```bash
# fetch Raspberry Pi Linux for the ARM64 target
make raspi-fetch

# build ARM64 DionysusD, initramfs, kernel Image, DTBs, and boot files
make raspi-boot
```

The output is written to `build/raspi/boot`. Copy those files to a Raspberry Pi FAT boot partition that already contains Raspberry Pi firmware files, or pass `RASPI_FIRMWARE_DIR=/path/to/firmware/boot` to include firmware files in the staged output.

The default Pi target is Raspberry Pi 4-class ARM64:

```bash
make raspi-boot RASPI_CONFIG_TARGET=bcm2711_defconfig RASPI_KERNEL_NAME=kernel8.img
```

For Raspberry Pi 5-class boards:

```bash
make raspi-boot RASPI_CONFIG_TARGET=bcm2712_defconfig RASPI_KERNEL_NAME=kernel_2712.img
```

## Dionysus PVE Slice

The primary Linux-native application slice now follows a Proxmox-style stack:

- `cmd/dionysusd`: Go daemon for API, web proxy, metrics sampling, JWT login auth, user management, and Ollama RAM control
- `pve/bin/dionysus-pvedaemon`: compatibility wrapper for `dionysusd api`
- `pve/bin/dionysus-pveproxy`: compatibility wrapper for `dionysusd proxy`
- `pve/bin/dionysus-metricsd`: compatibility wrapper for `dionysusd metricsd`
- `frontend/svelte`: Svelte source for the operator UI
- `pve/www`: built Svelte management interface served by `dionysusd proxy`
- `profiles/*.yaml`: sample Web, LLM, and DB policy profiles
- `packaging/systemd/dionysus-network.service`: systemd unit for persistent LAN/Wi-Fi setup from `/etc/dionysus/network.env`
- `packaging/systemd/dionysus-metricsd.service`: systemd unit for 30-second Ollama RAM sampling
- `packaging/systemd/dionysus-pvedaemon.service`: systemd unit for the API daemon
- `packaging/systemd/dionysus-pveproxy.service`: systemd unit for the management web proxy
- `packaging/systemd/dionysus-llm-swap.service`: systemd unit for provisioning Local LLM swap before Ollama starts
- `scripts/install-pve-control-plane.sh`: rootfs staging helper for installing the Go daemon, Svelte assets, profiles, and service files

Verification commands for the application slice:

```bash
# Proxmox-style control plane
make pve-check
make pve-control-plane-install
sudo sh scripts/install-pve-control-plane.sh --host
```

## Roadmap

- [x] Document the developer control plane architecture
- [x] Document the Linux-based control plane direction
- [x] Commit the initial project scaffold
- [x] Add development and test instructions
- [x] Boot upstream Linux with initramfs and start a Dionysus agent
- [ ] Implement the first Linux-backed memory inspection vertical slice
- [ ] Model package, Local LLM, and system status jobs on the control plane
- [ ] Introduce CI and code quality checks
- [ ] Publish the first tagged release

## Contributing

Early contributions are welcome, but alignment matters more than volume at this stage.

1. Read the workflow conventions before starting:
   - [`docs/conventions/README.md`](docs/conventions/README.md)
   - [`docs/conventions/commit-convention.md`](docs/conventions/commit-convention.md)
   - [`docs/conventions/branch-convention.md`](docs/conventions/branch-convention.md)
   - [`docs/conventions/issue-convention.md`](docs/conventions/issue-convention.md)
   - [`docs/conventions/pull-request-convention.md`](docs/conventions/pull-request-convention.md)
   - [`docs/conventions/code-convention.md`](docs/conventions/code-convention.md)
2. Open an issue first for feature proposals or structural changes.
3. Keep pull requests focused on one concern and record validation results.
4. Update documentation when behavior, workflow, or setup changes.

## License

A license has not been added yet.

Add a `LICENSE` file before accepting external code contributions or publishing packages.
