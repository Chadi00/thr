<div align="center">

# thr

Local semantic memory for your terminal and coding agents.

[![Latest release](https://img.shields.io/github/v/release/Chadi00/thr?style=flat-square&logo=github)](https://github.com/Chadi00/thr/releases)
[![Platform](https://img.shields.io/badge/platform-macOS%20%2B%20Linux-31363b?style=flat-square)](https://github.com/Chadi00/thr#platform)
[![License](https://img.shields.io/github/license/Chadi00/thr?style=flat-square)](LICENSE)

</div>

`thr` is a small local CLI for saving explicit memories and recalling them later by meaning or text. It is built for agent workflows: stable JSON output, offline semantic search, and installable skills for Codex, OpenCode, and Claude Code.

![thr semantic memory demo](docs/demo/show-hn.gif)

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/Chadi00/thr/master/install.sh | bash
```

The installer downloads the latest macOS or Linux release and verifies signed checksums before installing. Prefer manual verification? See [MANUAL_INSTALL.md](MANUAL_INSTALL.md).

## Quick Start

```bash
thr add "This project prefers small PRs with tests"
thr ask "how should I structure this PR?"
thr search "small PRs"
thr list
```

`thr ask` performs semantic recall over your saved memories. It does not call an LLM or generate an answer.

## Agents

Install the `thr` skill for your coding agent:

```bash
thr setup codex
thr setup opencode
thr setup claude-code
```

After that, an agent can retrieve durable project facts with `thr ask --json` or `thr search --json`, then maintain memories with `thr add`, `thr edit`, and `thr forget`.

## Features

- Local first: memories live in your own SQLite database.
- Semantic and text recall: use `ask` for meaning, `search` for exact or fuzzy text.
- Agent ready: JSON output and packaged skills make it easy for agents to use safely.
- Offline by default: the embedding model and ONNX Runtime are bundled with the release.
- Explicit memory: `thr` stores only what you save.

## CLI

```bash
thr add <text>          Save a memory
thr add -               Save stdin
thr list                List memories and ids
thr show <id>           Print one memory
thr ask <question>      Retrieve semantically similar memories
thr search <query>      Search memories by text
thr edit <id> <text>    Replace a memory
thr forget <id>         Delete a memory
thr stats               Show database path and count
thr index               Rebuild semantic embeddings
thr prefetch            Prepare the model cache
thr setup <agent>       Install an agent skill
```

Useful flags:

```bash
thr --db ./thr.db ...
THR_DB=./thr.db thr list
thr ask --json "repo conventions"
thr search -n 5 "release notes"
```

## Data

| Data | Default |
|------|---------|
| Memories | `~/.thr/thr.db` |
| Model cache | `~/.thr/models` |
| Install prefix | `~/.local` |

Memories are stored as local plaintext SQLite. The default data directories are created with private filesystem permissions, but the database is not encrypted at rest.

`thr` has no telemetry. Release archives include the embedding model and runtime needed for offline semantic search.

## Platform

Prebuilt releases support macOS arm64/x86_64 and glibc Linux arm64/x86_64.

Windows and Alpine/musl Linux are not supported yet.

## Links

- [Releases](https://github.com/Chadi00/thr/releases)
- [Changelog](CHANGELOG.md)
- [License](LICENSE)
- [Third-party notices](THIRD_PARTY_NOTICES.md)
