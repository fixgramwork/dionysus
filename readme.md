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
| Focus | Bootable prototype with a documented developer control plane |
| Next milestone | Implement the first memory inspection vertical slice from Rust to Svelte |

## Overview

Dionysus is currently being prepared as a focused project under `fixgramwork`.
At this stage, the repository serves as the public-facing source of truth for:

- project intent
- initial scope
- contributor expectations
- release readiness

The long-term product direction is a developer-friendly operating system with a control plane for memory inspection, controlled mutation, package automation, Local LLM lifecycle management, and server status visibility.

This README is intentionally structured like a production open-source repository so the project can scale without rewriting its documentation from scratch.

## Principles

- Keep the first version narrow and shippable.
- Prefer obvious setup over clever tooling.
- Add quality gates early: linting, testing, and CI.
- Document decisions as the repository grows.

## Architecture Direction

The first system design pass fixes the control-plane boundary as:

```text
ASM -> Rust kernel -> control interface -> Go backend -> Svelte UI
```

- ASM remains responsible for the boot path and low-level entry guarantees.
- Rust owns memory regions, policy, and mutation authority.
- Go exposes authenticated control-plane APIs and audit-friendly workflows.
- Svelte focuses on visualization and guarded operator interactions.
- Future package, Local LLM, and system status features will extend the same control-plane model instead of introducing side channels.

See [`docs/architecture/developer-control-plane.md`](docs/architecture/developer-control-plane.md) for the current architecture draft.

## Planned Deliverables

| Track | What will be added |
| --- | --- |
| Application | Rust kernel control interface and memory inspection primitives |
| Developer Experience | Go backend, Svelte UI, and install/run/test automation |
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

- [x] Document the developer control plane architecture
- [x] Commit the initial project scaffold
- [x] Add development and test instructions
- [ ] Implement the first memory inspection vertical slice
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
