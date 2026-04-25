<div align="center">

# thr

A local CLI for notes you can find again with **semantic** and **keyword** search.

[![Latest release](https://img.shields.io/github/v/release/Chadi00/thr?style=flat-square&logo=github)](https://github.com/Chadi00/thr/releases)
[![Platform](https://img.shields.io/badge/platform-macOS-31363b?style=flat-square)](https://github.com/Chadi00/thr#platform-support)

Retrieval runs **on your machine** — no cloud API for search.

</div>

---

## Install

Requires macOS with [Homebrew](https://brew.sh) installed.

```bash
curl -fsSL https://raw.githubusercontent.com/Chadi00/thr/master/install.sh | bash
```

### Uninstall

```bash
curl -fsSL https://raw.githubusercontent.com/Chadi00/thr/master/uninstall.sh | bash
```

---

## Quick start

After install, try:

```bash
thr add "prefers small CLIs with good docs"
thr list
thr ask "what are their CLI preferences?"
thr search "cli docs"
```

Full help: `thr --help` and `thr <command> --help`.

**Scripts and agents:** add `--json` to `list`, `show`, `ask`, `search`, or `stats` for stable output. Multiline input: `printf "a\nb\n" | thr add -` or `thr edit 1 -`.

---

## Commands

| Command | Description |
|---------|-------------|
| `thr add <text>` · `thr add -` | Save a memory from text or stdin |
| `thr list` | List memories (with ids) |
| `thr show <id>` | Print one memory |
| `thr ask <question>` | Semantic search (meaning, not an LLM answer) |
| `thr search <query>` | Text recall: FTS + substring + fuzzy / subsequence ranking (recent window) |
| `thr edit <id> <text>` · `thr edit <id> -` | Replace a memory |
| `thr forget <id>` | Delete a memory |
| `thr stats` | Database path and count |
| `thr prefetch` | Cache the embedding model |
| `thr version` | Build version (`-v` / `--version` also work) |

**Globals:** `--db <path>` or `THR_DB` for the database. On read commands, `--json` emits stable JSON for scripts and agents.

---

## Where data lives

| | Default |
|--|--------|
| Database | `~/.thr/thr.db` |
| Embedding cache | `~/.thr/models` (`THR_MODEL_CACHE` overrides) |

Semantic vectors use [BAAI/bge-base-en-v1.5](https://huggingface.co/BAAI/bge-base-en-v1.5) (768-d) with sqlite-vec. Text recall uses SQLite FTS5, bounded recent substring search, and fuzzy / subsequence scoring so short queries (e.g. `rst` → `rust`) can still match.

Use the numeric **id** from `thr list` (or from `ask` / `search`) with `show`, `edit`, and `forget`.

---

## Platform support

**Supported:** macOS **arm64** and **x86_64**. Prebuilt archives are attached to [Releases](https://github.com/Chadi00/thr/releases), and the recommended install path assumes **Homebrew** is available (see [Install](#install)).

**Focus:** macOS only. Other platforms are not part of the supported surface.

---

## Changelog

See [CHANGELOG.md](CHANGELOG.md) for release notes by version.

---
