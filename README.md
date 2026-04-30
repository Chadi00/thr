<div align="center">

# thr

A local CLI for memories you can find again with **semantic** and **keyword** search.

[![Latest release](https://img.shields.io/github/v/release/Chadi00/thr?style=flat-square&logo=github)](https://github.com/Chadi00/thr/releases)
[![Platform](https://img.shields.io/badge/platform-macOS%20%2B%20Linux-31363b?style=flat-square)](https://github.com/Chadi00/thr#platform-support)

Retrieval runs **on your machine** — no cloud API for search.

</div>

---

## Install

Requires macOS or glibc Linux. Homebrew is not required.

```bash
curl -fsSL https://raw.githubusercontent.com/Chadi00/thr/master/install.sh | bash
```

The installer can optionally add the `thr` Agent Skill for Claude Code, OpenCode, or Codex so your coding agent knows when and how to use local memories.

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
thr list --last 4
```

Full help: `thr --help` and `thr <command> --help`.

**Scripts and agents:** add `--json` to `list`, `show`, `ask`, `search`, or `stats` for stable output. Multiline input: `printf "a\nb\n" | thr add -` or `thr edit 1 -`.

### Agent setup

Install the `thr` Agent Skill for a supported coding agent:

```bash
thr setup claude-code
thr setup opencode
thr setup codex
```

The skill teaches agents to retrieve durable preferences and project facts with `thr ask` / `thr search`, save explicit non-sensitive memories with `thr add`, and maintain memories with `thr edit` / `thr forget`.

Other agents that support Agent Skills can install the same [`skills/thr`](skills/thr) directory manually.

---

## Commands

| Command | Description |
|---------|-------------|
| `thr add <text>` · `thr add -` | Save a memory from text or stdin |
| `thr list` | List memories (with ids); use `--last 4`, `--limit 4`, or `-n 4` to control the count |
| `thr show <id>` | Print one memory |
| `thr ask <question>` | Semantic search (meaning, not an LLM answer) |
| `thr search <query>` | Text recall: FTS + substring + fuzzy / subsequence ranking (recent window) |
| `thr edit <id> <text>` · `thr edit <id> -` | Replace a memory |
| `thr forget <id>` | Delete a memory |
| `thr index` | Rebuild missing or stale semantic search embeddings |
| `thr stats` | Database path and count |
| `thr prefetch` | Prepare the bundled embedding model cache |
| `thr setup claude-code` / `opencode` / `codex` | Install the `thr` Agent Skill |
| `thr version` | Build version (`-v` / `--version` also work) |

**Globals:** `--db <path>` or `THR_DB` for the database. On read commands, `--json` emits stable JSON for scripts and agents. `ask` accepts `--max-distance` to tune semantic match strictness. `add` and `edit` accept `--max-bytes` to raise or lower the memory text size limit.

---

## Where data lives

| | Default |
|--|--------|
| Database | `~/.thr/thr.db` |
| Embedding cache | `~/.thr/models` (`THR_MODEL_CACHE` overrides) |
| Install prefix | `~/.local` (`THR_INSTALL_PREFIX` overrides) |

thr stores memories as local plaintext in SQLite and hardens the default data and model-cache paths with private filesystem permissions. It does not encrypt memories at rest.

Semantic vectors use a bundled, pinned, SHA-256-verified [Qdrant/bge-base-en-v1.5-onnx-Q](https://huggingface.co/Qdrant/bge-base-en-v1.5-onnx-Q) model (768-d) with sqlite-vec. Release archives also include a pinned ONNX Runtime shared library, so semantic search works without Homebrew or a separate system ONNX Runtime install. `thr prefetch` prepares the local model cache from the installed `thr` binary, so no model files are downloaded during install or normal use. `thr ask` returns only matches within a default vector distance of `0.80`; pass `--max-distance 4` to restore closest-results behavior. If the active model changes in a future release, `thr index` rebuilds semantic embeddings while preserving saved memories. Text recall uses SQLite FTS5, bounded recent substring search, and fuzzy / subsequence scoring so short queries (e.g. `rst` → `rust`) can still match.

Release checksums are signed with an OpenSSH release key and verified by the installer before extraction. GitHub release attestations are published as an additional provenance signal for users who want to verify artifacts with `gh release verify-asset`.

Release signing keys are rotated by adding the new public key to `install.sh`, publishing at least one release that trusts both old and new keys, then removing the old key after older supported releases age out. Existing releases keep verifying with the public key that shipped in their installer.

Maintainers must set the `THR_SSH_SIGNING_KEY` GitHub Actions secret to the private OpenSSH key matching the public signer embedded in `install.sh`.

Native ONNX Runtime artifacts are built by the `native-runtime` workflow and published to a dedicated prerelease tag such as `thr-native-onnxruntime-v1.25.1`. Normal `thr` releases only consume pinned runtime assets from `native/onnxruntime.lock`; they do not compile ONNX Runtime. When a lockfile change leaves a shipping target without pinned runtime metadata, `native-runtime` builds only the missing target artifacts, updates `native/onnxruntime.lock`, and dispatches CI so the normal release workflow can continue from the pinned commit.

Use the numeric **id** from `thr list` (or from `ask` / `search`) with `show`, `edit`, and `forget`.

---

## Platform support

**Supported:** macOS **arm64** / **x86_64** and glibc Linux **arm64** / **x86_64**. Prebuilt, self-contained archives are attached to [Releases](https://github.com/Chadi00/thr/releases).

**Not yet supported:** Alpine/musl Linux and Windows.

---

## Changelog

See [CHANGELOG.md](CHANGELOG.md) for release history by version.
