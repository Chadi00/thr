<div align="center">

# thr: local semantic memory for your terminal and coding agents

Save durable facts once. Recall them from your shell, scripts, or coding agents without a cloud service.

[![Latest release](https://img.shields.io/github/v/release/Chadi00/thr?style=flat-square&logo=github)](https://github.com/Chadi00/thr/releases)
[![Platform](https://img.shields.io/badge/platform-macOS%20%2B%20Linux-31363b?style=flat-square)](https://github.com/Chadi00/thr#platform-support)

</div>

---

```bash
thr add "This project prefers small PRs with tests"
thr ask "how should I structure this PR?"
thr setup codex
```

`thr` is a small local CLI memory layer for agent workflows: explicit memories, keyword and semantic recall, stable JSON output, and installable Agent Skills for Claude Code, OpenCode, and Codex.

---

## Install

For macOS and Linux, use:

```bash
curl -fsSL https://raw.githubusercontent.com/Chadi00/thr/master/install.sh | bash
```

The installer downloads a signed, self-contained release archive and verifies its signed checksums before installing.

Prefer not to run `curl | bash`? Use the [manual verified install guide](MANUAL_INSTALL.md).

### Why is it 210 MB?

The archive is large because `thr` bundles the embedding model and ONNX Runtime so semantic search works offline with no Python, Node, Docker, server, cloud API, or separate model install.

---

## Quick start

After install, try:

```bash
thr add "This project prefers small PRs with tests"
thr list
thr ask "how should I structure this PR?"
thr search "small PRs"
```

Full help: `thr --help` and `thr <command> --help`.

**Scripts and agents:** add `--json` to `list`, `show`, `ask`, `search`, or `stats` for stable output. Multiline input: `printf "a\nb\n" | thr add -` or `thr edit 1 -`.

### Agent memory workflow

Save project conventions once:

```bash
thr add "This project prefers small PRs with tests"
thr add "Use the existing Cobra command style when adding CLI features"
```

Then install the Agent Skill:

```bash
thr setup codex
```

In a later session, your coding agent can retrieve those conventions before editing code with `thr ask --json "how should I work in this repo?"` or `thr search --json "Cobra command style"`.

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

## Why not just use notes.md?

Use `notes.md` if you want a file you read and edit yourself. `thr` is for agent-readable memory: stable JSON output, ids for edit/delete/list, keyword search, semantic recall, a local embedding index, and installable Agent Skills that teach coding agents how to retrieve and maintain explicit project facts.

---

## Why not use Mem0?

Mem0 is a broader memory platform with hosted and application-integration use cases. `thr` is intentionally smaller: a local CLI for explicit, plaintext, offline agent memory that works from a terminal, shell script, or coding-agent skill without running a service or sending memories to a cloud API.

---

## What thr is not

`thr` is not a cloud memory service, an LLM, a vector database server, or automatic surveillance memory. It only stores what you explicitly save with commands like `thr add`, and `thr ask` retrieves matching memories rather than generating an answer.

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

| Data | Default |
|------|---------|
| Memories database | `~/.thr/thr.db` |
| Embedding/model cache | `~/.thr/models` (`THR_MODEL_CACHE` overrides) |
| Install prefix | `~/.local` (`THR_INSTALL_PREFIX` overrides) |

`thr` stores memories as local plaintext SQLite and hardens the default data and model-cache paths with private filesystem permissions. It is not encrypted at rest.

Semantic search uses the bundled embedding model cached under `~/.thr/models`. The model cache and embeddings stay local. `thr prefetch` prepares that cache ahead of time, and `thr index` rebuilds embeddings if needed.

`thr` has no telemetry. The installer downloads release assets from GitHub Releases, verifies signed checksums, and installs the binary plus packaged runtime files.

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

---

## Contributing

`thr` is early and I’m not accepting external code contributions yet while the core design is still changing.

Issues, bug reports, use cases, and design feedback are very welcome. If you’re interested in contributing code later, please open an issue first so we can discuss whether it fits the direction of the project.
