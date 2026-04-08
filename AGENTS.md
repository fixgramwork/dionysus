# Agent Workflow

Before starting work in this repository, review the convention index:

- [`docs/conventions/README.md`](docs/conventions/README.md)

Use the detailed documents as the source of truth for:

- commit messages
- branch names
- issue titles and issue scope
- PR titles, descriptions, and verification notes
- code style for Rust, NASM, linker scripts, and Makefiles

Repository-specific rules:

- Keep one change set focused on one concern.
- For boot, kernel, linker, GRUB, or build changes, always record the exact verification command and observed result.
- Prefer updating the related convention document when the workflow changes instead of leaving the rule implicit in a PR discussion.
