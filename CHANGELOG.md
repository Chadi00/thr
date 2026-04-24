# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.5] - 2026-04-24

### Added

- `uninstall.sh`: one-command removal of `thr`, default `~/.thr` data, and installer-added shell `PATH` lines; optional `THR_UNINSTALL_ONNX=1` and `THR_KEEP_DATA=1` (documented in the README).

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

- Release CI: `onnxruntime_go` builds without system ONNX headers; add `libsqlite3-dev` for CGO SQLite on Linux; remove redundant Homebrew `onnxruntime` from the macOS release job (Ubuntu 24.04 has no `libonnxruntime-dev` in default apt).

## [0.1.0] - 2026-04-24

### Added

- Prebuilt CGO `thr` binaries per OS/arch on GitHub Releases, `install.sh` defaulting to verified release tarballs (checksums) with a source-build fallback, and a tag-driven release workflow.
