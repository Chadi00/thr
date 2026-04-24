<div align="center">

# thr

**Tiny History Recall** — a local CLI for notes you can find again with **semantic** and **keyword** search.

[![Latest release](https://img.shields.io/github/v/release/Chadi00/thr?style=flat-square&logo=github)](https://github.com/Chadi00/thr/releases)
[![Platform](https://img.shields.io/badge/platform-macOS%20%7C%20Linux-31363b?style=flat-square)](https://github.com/Chadi00/thr#platform-support)

Retrieval runs **on your machine** — no cloud API for search.

</div>

---

## Install

**macOS and Linux** — one command downloads a verified release binary and installs `thr` to a standard location:

```bash
curl -fsSL https://raw.githubusercontent.com/Chadi00/thr/master/install.sh | bash
```

The installer picks the right archive for your OS and CPU, checks SHA-256 checksums from the release, and places the binary where your PATH can find it (see [Releases](https://github.com/Chadi00/thr/releases) if you prefer a manual download).

| Variable | Purpose |
|----------|---------|
| `THR_VERSION` | `latest` (default) or an exact tag, e.g. `v0.1.1` |
| `GITHUB_TOKEN` | Optional; higher rate limits for GitHub API when resolving releases |
| `THR_USER_BIN` | Linux install dir override (default: `~/.local/bin`) |
| `THR_USE_SOURCE=1` | Build from source with Go + CGO instead of a release binary |
| `THR_INSTALL_REF` | With source install: git ref to build (default: `master`) |

---

## Quick start

After install, try:

```bash
thr add "prefers small CLIs with good docs"
thr list
thr ask "what are their CLI preferences?"
thr search "cli docs"
```

Full help: `thr --help` and `thr <command> --help`. Use `thr prefetch` to download the embedding model before the first slow `add` or `ask`.

**Scripts and agents:** add `--json` to `list`, `show`, `ask`, or `search` for stable output. Multiline input: `printf "a\nb\n" | thr add -` or `thr edit 1 -`. Shell completion: `source <(thr completion zsh)`.

---

## Commands

| Command | Description |
|---------|-------------|
| `thr add <text>` · `thr add -` | Save a memory from text or stdin |
| `thr list` | List memories (with ids) |
| `thr show <id>` | Print one memory |
| `thr ask <question>` | Semantic search (meaning, not an LLM answer) |
| `thr search <query>` | Text recall: FTS + substring + fuzzy ranking |
| `thr edit <id> <text>` · `thr edit <id> -` | Replace a memory |
| `thr forget <id>` | Delete a memory |
| `thr stats` | Database path and count |
| `thr prefetch` | Cache the embedding model |
| `thr version` | Build version (`-v` / `--version` also work) |
| `thr completion bash` (or `zsh`, `fish`) | Print a shell completion script |

**Globals:** `--db <path>` or `THR_DB` for the database. On read commands, `--json` emits stable JSON for scripts and agents.

---

## Where data lives

| | Default |
|--|--------|
| Database | `~/.thr/thr.db` |
| Embedding cache | `~/.thr/models` (`THR_MODEL_CACHE` overrides) |

Semantic vectors use [BAAI/bge-base-en-v1.5](https://huggingface.co/BAAI/bge-base-en-v1.5) (768-d) with sqlite-vec. Text recall uses SQLite FTS5 plus bounded substring and fuzzy ranking.

Use the numeric **id** from `thr list` (or from `ask` / `search`) with `show`, `edit`, and `forget`.

---

## Platform support

**First-class:** macOS **arm64** and **x86_64**, Linux **x86_64** and **arm64** (glibc-oriented distros; release builds are from Ubuntu runners). Prebuilt archives are attached to [Releases](https://github.com/Chadi00/thr/releases).

**Source builds** need Go **1.26+**, CGO, SQLite dev headers, and a loadable **ONNX Runtime** (e.g. Homebrew `onnxruntime` on macOS; Linux distro packages or a path where the embedding stack can load `libonnxruntime`).

**Not supported:** Windows, 32-bit, musl-first distros like stock Alpine (untested), and non-macOS BSD — open an issue with OS, arch, and what you tried if you need something outside this list.

---

## Contributing

Issues and pull requests are welcome. Include your OS and the exact command you ran if something breaks.

**Maintainers:** publishing is tag-driven — push a semver tag and GitHub Actions builds CGO binaries and uploads them to a GitHub Release.
