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
Bootloader/Firmware -> Linux kernel -> kernel interfaces -> Dionysus agent -> Perl API2 daemon -> pveproxy -> ExtJS-style UI
```

- Linux becomes the long-term kernel and hardware base.
- Dionysus agent translates Linux interfaces into a product-specific resource model.
- Perl exposes Proxmox-style `/api2/json` control-plane APIs and audit-friendly workflows.
- `pveproxy` serves the management web on port `8006` with an ExtJS-style operator interface.
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
| Application | Dionysus agent, Perl API2 daemon, pveproxy-style UI, and memory inspection primitives |
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
The management web is installed into the normal Linux root filesystem as systemd services.

For a normal Linux root filesystem with systemd, stage Dionysus as an OS-managed service:

```bash
make pve-control-plane-install
```

That target installs the Perl API2 daemon, `pveproxy`-style web service, workload profiles, environment defaults, and systemd units into `build/pve-control-plane-rootfs`. The service starts on `multi-user.target` and serves the management web from inside the OS on port `8006`.
It also stages `dionysus-llm-swap.service`, which creates and enables a dedicated swap file before `ollama.service`, `dionysus-pvedaemon.service`, and `dionysus-pveproxy.service`.

## Linux Base Workflow

If the product direction is "Ubuntu-style development on top of Linux", treat this repository as a distro/product layer around upstream Linux instead of a custom kernel tree.

Current repository convention:

- upstream Linux checkout: `upstream/linux`
- tracked Dionysus kernel deltas: config fragments, optional patches, initramfs, agent, PVE API daemon, and UI
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

- `pve/bin/dionysus-pvedaemon`: Perl API daemon using Proxmox-style `/api2/json` paths
- `pve/bin/dionysus-pveproxy`: web proxy serving the operator UI on port `8006`
- `pve/lib/Dionysus/PVE/API.pm`: node status, service status, Ollama, swap, and optimization API
- `pve/www/index.html`: ExtJS-style management interface without a frontend build step
- `profiles/*.yaml`: sample Web, LLM, and DB policy profiles
- `packaging/systemd/dionysus-pvedaemon.service`: systemd unit for the API daemon
- `packaging/systemd/dionysus-pveproxy.service`: systemd unit for the management web proxy
- `packaging/systemd/dionysus-llm-swap.service`: systemd unit for provisioning Local LLM swap before Ollama starts
- `scripts/install-pve-control-plane.sh`: rootfs staging helper for installing the Perl stack, profiles, and service files

Verification commands for the application slice:

```bash
# Proxmox-style control plane
make pve-check
make pve-control-plane-install
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
