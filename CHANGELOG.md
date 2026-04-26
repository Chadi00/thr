# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.14] - 2026-04-26

### Changed

- The pinned BGE semantic model is now bundled into the `thr` binary and prepared into the local cache without a runtime Hugging Face download.

## [0.1.13] - 2026-04-25

### Changed

- `thr ask` now filters weak semantic matches with a default `--max-distance 0.80`, with `--max-distance 4` available for closest-results behavior.

## [0.1.12] - 2026-04-25

### Changed

- Updated CLI help and docs so `thr` is treated as the command name rather than an expanded acronym.
- Simplified the README quick start, uninstall wording, and release automation details.

### Fixed

- Disabled Cobra's built-in `completion` command so `thr completion` is no longer exposed.

## [0.1.11] - 2026-04-25

### Fixed

- `uninstall.sh` now asks separately before deleting saved memories or the cached embedding model, preserving both when it cannot prompt.

## [0.1.10] - 2026-04-25

### Fixed

- Release automation now waits for successful CI on the exact pushed commit before tagging, building, smoke testing, and publishing a release.
- Installer PATH snippets now keep `$PATH` escaped in shell rc files so future shells expand it correctly.

## [0.1.9] - 2026-04-25

### Added

- `thr index` rebuilds missing or stale semantic embeddings for the active local model.
- Embedding metadata tracks model id, revision, manifest digest, dimension, and index time so stale semantic indexes can be detected.
- `thr stats` reports active model identity, verification status, and semantic index health.
- Release archives are now checked through signed `checksums.txt` metadata, with installer and smoke-test coverage for signature verification.

### Changed

- Semantic search now uses a pinned, SHA-256-verified `Qdrant/bge-base-en-v1.5-onnx-Q` model cache and re-downloads unverified model files.
- Local database and model-cache paths are hardened with private filesystem permissions.
- `thr ask` now stops with a clear `thr index` prompt when embeddings are missing or stale instead of searching an outdated semantic index.
- `thr add` and `thr edit` validate memory text size before initializing storage or the embedding model, with `--max-bytes` available for overrides.
- Plain-text output sanitizes control and format characters while preserving structured JSON output for scripts.
- README install, prefetch, and contribution guidance was tightened around the primary macOS Homebrew path.

## [0.1.8] - 2026-04-24

### Fixed

- macOS release smoke tests now treat an already-configured Homebrew `PATH` as a valid install outcome, so tag-driven releases succeed on GitHub's default runners.

## [0.1.7] - 2026-04-24

### Changed

- Simplified the README install and uninstall docs to the default macOS Homebrew commands so the published instructions stay focused on the primary supported path.

## [0.1.5] - 2026-04-24

### Added

- `uninstall.sh`: one-command removal of `thr`, default `~/.thr` data, and installer-added shell `PATH` lines.

## [0.1.4] - 2026-04-24

### Fixed

- macOS installs from GitHub release binaries now run the same ONNX Runtime Homebrew setup as source builds (`brew install onnxruntime` when needed), so `thr prefetch` and embedding commands work after a one-line install without a separate manual dependency step.

## [0.1.3] - 2026-04-24

### Removed

- Shell `completion` subcommand (bash, zsh, fish script generation) and the `--db` flag shell-completion registration. The tool is focused on non-interactive use (agents and scripts), not tab completion in a terminal.

## [0.1.2] - 2026-04-24

### Fixed

- `thr search` now includes subsequence (abbreviation-style) matches against recent memories, not only exact substrings, so behavior matches the CLI description of fuzzy recall (e.g. `rst` can match `rust`).

## [0.1.1] - 2026-04-24

### Fixed

- Release CI: `onnxruntime_go` builds without system ONNX headers, and the macOS release job no longer installs a redundant Homebrew `onnxruntime` build dependency.

## [0.1.0] - 2026-04-24

### Added

- Prebuilt CGO `thr` binaries per OS/arch on GitHub Releases, `install.sh` defaulting to verified release tarballs (checksums) with a source-build fallback, and a tag-driven release workflow.
