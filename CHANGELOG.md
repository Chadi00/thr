# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed

- Updated CLI help and docs so `thr` is treated as the command name rather than an expanded acronym.
- Simplified the README quick start, uninstall notes, and release automation details.

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
