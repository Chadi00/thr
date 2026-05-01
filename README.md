<div align="center">

# thr

local memory for your terminal and coding agents. Save notes for yourself or your AI agent, search them by text or meaning, and keep everything local.

[![Latest release](https://img.shields.io/github/v/release/Chadi00/thr?style=flat-square&logo=github)](https://github.com/Chadi00/thr/releases)
[![Platform](https://img.shields.io/badge/platform-macOS%20%2B%20Linux-31363b?style=flat-square)](https://github.com/Chadi00/thr#platform-support)

</div>

---

## Install

For macOS and Linux, use:

```bash
curl -fsSL https://raw.githubusercontent.com/Chadi00/thr/master/install.sh | bash
```

The installer downloads a signed, self-contained release archive of about 210 MB so semantic search works without a separate model or runtime install.

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

### Agent setup

Install the `thr` Agent Skill for a supported coding agent:

```bash
thr setup claude-code
thr setup opencode
thr setup codex
```

The skill teaches agents to retrieve durable preferences and project facts with `thr ask` / `thr search`, save explicit non-sensitive memories with `thr add`, and maintain memories with `thr edit` / `thr forget`.

`claude-code` installs to `~/.claude/skills/thr/SKILL.md`. `opencode` and `codex` install to the shared global Agent Skills location at `~/.agents/skills/thr/SKILL.md`.

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

thr stores memories as local plaintext in SQLite and hardens the default data and model-cache paths with private filesystem permissions.

Semantic search uses a bundled embedding model cached under `~/.thr/models`. `thr prefetch` prepares that cache ahead of time, and `thr index` rebuilds embeddings if needed.

Use the numeric **id** from `thr list` (or from `ask` / `search`) with `show`, `edit`, and `forget`.

---

## Platform support

**Supported:** macOS **arm64** / **x86_64** and glibc Linux **arm64** / **x86_64**. Prebuilt, self-contained archives are attached to [Releases](https://github.com/Chadi00/thr/releases).

**Not yet supported:** Alpine/musl Linux and Windows.

---

## Changelog

See [CHANGELOG.md](CHANGELOG.md) for release history by version.

---

## License

`thr` is released under the [MIT License](LICENSE). See [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md) for bundled model, vendored library, and packaged runtime notices.

---

## Uninstall

```bash
curl -fsSL https://raw.githubusercontent.com/Chadi00/thr/master/uninstall.sh | bash
```
