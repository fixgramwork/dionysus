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
| Codebase | Minimal bootable kernel scaffold |
| Focus | Bootable ISO pipeline and early kernel iteration |
| Next milestone | Expand the kernel beyond the GRUB handoff |

## Overview

Dionysus is currently being prepared as a focused project under `fixgramwork`.
At this stage, the repository serves as the public-facing source of truth for:

- project intent
- initial scope
- contributor expectations
- release readiness

This README is intentionally structured like a production open-source repository so the project can scale without rewriting its documentation from scratch.

## Principles

- Keep the first version narrow and shippable.
- Prefer obvious setup over clever tooling.
- Add quality gates early: linting, testing, and CI.
- Document decisions as the repository grows.

## Planned Deliverables

| Track | What will be added |
| --- | --- |
| Application | Initial source scaffold |
| Developer Experience | Install, run, and test commands |
| Quality | Formatting, linting, and CI checks |
| Documentation | Usage examples and architecture notes |
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

Current local workflow:

```bash
git clone https://github.com/fixgramwork/dionysus.git
cd dionysus

# install dependencies
# i686-elf-gcc, i686-elf-ld, nasm, grub-mkrescue, xorriso, qemu-system-x86_64

# build the kernel ELF only
make build

# package a bootable ISO
make iso

# build, package, and boot with QEMU
make
```

The default Make target is `run`, so `make` builds the kernel, emits `build/dionysus.iso`, and boots it with QEMU.

## Roadmap

- [ ] Define the core problem and first user flow
- [x] Commit the initial project scaffold
- [x] Add development and test instructions
- [ ] Introduce CI and code quality checks
- [ ] Publish the first tagged release

## Contributing

Early contributions are welcome, but alignment matters more than volume at this stage.

1. Open an issue first for feature proposals or structural changes.
2. Keep pull requests focused on one concern.
3. Update documentation when behavior or setup changes.

## License

A license has not been added yet.

Add a `LICENSE` file before accepting external code contributions or publishing packages.
