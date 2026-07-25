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

The installer downloads and verifies the latest macOS or Linux release, installs the binary and packaged ONNX Runtime, migrates the selected default database (`THR_DB` or `~/.thr/thr.db`) when needed, and prepares the bundled model cache. Recognized managed agent skills are updated automatically; skill setup is offered when no managed skill is found. Prefer manual verification? See [MANUAL_INSTALL.md](MANUAL_INSTALL.md).

## Quick Start

From inside a Git repository:

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

After that, an agent can retrieve durable facts with `thr --format json-v2 ask "<question>"` or `thr --format json-v2 search "<query>"`, then maintain memories with `add`, `edit`, `move`, and `forget`.

## Scopes

When Git safely resolves the current repository, default `ask`, `search`, and `list` operations search its repository scope, when one exists, plus `user`. An unqualified `add` creates or binds the repository scope when needed and writes there. Broad writes are explicit:

```bash
thr add "This repository uses pnpm"
thr add --scope user "The user prefers concise explanations"
```

Outside a repository, default recall searches `user` and writes require `--scope user`. On `ask`, `search`, and `list`, repeat `--scope user|repo|repo:<id>` to search a union, or use `--all-scopes`; the two forms are mutually exclusive. `stats` and `index` accept the same selectors but inspect all scopes by default. `add` accepts one destination `--scope`, while `move` requires one destination `--to`. Exact-ID `show`, `edit`, and `forget` operations do not depend on repository resolution. `--cwd /path/to/repo` changes repository context without changing the shell directory.

```bash
thr --format json-v2 context
thr --cwd ../other-repo --format json-v2 ask "release process"
thr --format json-v2 scope list
thr move 42 --to repo:01KABC
```

Repository identity prefers the configured `thr.identityRemote`, then `origin`, then a sole remote. If an unbound repository has multiple remotes and no `origin`, select one with `git config --local thr.identityRemote <remote-name>`. `thr context` reports database status, current or prospective scope, defaults, and repository resolution without creating or migrating the database.

`--format` accepts `human`, `legacy-json`, or `json-v2`; `--json` selects legacy JSON and cannot be combined with `--format`. New integrations should use JSON v2, whose versioned envelope includes database and working-directory context, scope selection where applicable, memory scopes and revisions, warnings, and structured errors. Legacy JSON remains available for read-oriented commands but omits scope metadata.

## Features

- Local first: memories live in your own SQLite database.
- Semantic and text recall: use `ask` for meaning, `search` for exact or fuzzy text.
- Agent ready: JSON output and packaged skills make it easy for agents to use safely.
- Offline by default: the embedding model and ONNX Runtime are bundled with the release.
- Explicit memory: `thr` stores only what you save.

## CLI

```text
thr add <text|->                    Save a memory; '-' reads stdin
thr list                            List newest memories, IDs, and scopes
thr show <id>                       Print one memory by global ID
thr ask <question>                  Retrieve semantically similar memories
thr search <query>                  Run text and fuzzy recall
thr edit <id> <text|->              Replace a memory; '-' reads stdin
thr forget <id>                     Delete a memory
thr move <id> --to <scope>          Move without changing ID, text, or embedding
thr stats                           Show database, model, index, and scope health
thr index                           Build missing or stale embeddings
thr update                          Update thr to the latest release
thr context                         Inspect database and repository scope context
thr scope list                      List registered scopes
thr scope show <scope>              Show one scope and its bindings
thr scope create repo               Create and bind the current repository scope
thr scope bind <repo:id>            Bind the current checkout
thr scope unbind                    Unbind the current checkout
thr scope rebind <repo:id>          Transfer the current checkout binding
thr scope rename <repo:id> <label>  Rename a repository scope
thr scope split                     Bind the checkout to a new empty scope
thr migrate                         Back up and migrate a legacy database
thr prefetch                        Prepare the bundled model cache
thr setup <agent>                    Install a codex, opencode, or claude-code skill
thr version                         Print version information
```

Useful flags:

```bash
thr --db ./thr.db ...
THR_DB=./thr.db thr list
thr --cwd ../other-repo --format json-v2 context
thr --format json-v2 ask "repo conventions"
thr ask --scope repo --scope user "release process"
thr list --scope user --legacy
thr stats --all-scopes
thr search -n 5 "release notes"
thr edit 42 "updated text" --if-revision 3
```

Use `thr <command> --help` for command-specific limits, scope selectors, concurrency preconditions, and management flags.
See the [complete command guide](docs/commands.md) for every command, flag, and workflow.

## Data

| Data | Selection/default |
|------|-------------------|
| Memories | `--db`, then `THR_DB`, then `~/.thr/thr.db` |
| Model cache | `THR_MODEL_CACHE`, then `~/.thr/models` |
| Install prefix | `THR_INSTALL_PREFIX`, then `~/.local` |

Memories are stored as local plaintext SQLite. The default data directories are created with private filesystem permissions, but the database is not encrypted at rest.

The installer migrates its selected default database automatically. Other legacy databases, including custom `--db` paths, migrate on first operational access after a private verified backup is created. `thr context` is the read-only exception: it reports `migration_required` without changing the database. You can run `thr --db <path> migrate` explicitly at any time.

Legacy memories are assigned to `user` with `scope_assignment: legacy_default` so existing visibility is preserved until reviewed. List them with `thr list --scope user --legacy`; moving one to `user`, `repo`, or `repo:<id>` records an explicit assignment. A scoped database cannot be opened by older binaries; downgrade by restoring the reported pre-migration backup.

`thr` has no telemetry. Release archives include the embedding model and runtime needed for offline semantic search.

## Platform

Prebuilt releases support macOS arm64/x86_64 and glibc Linux arm64/x86_64.

Windows and Alpine/musl Linux are not supported yet.

## Links

- [Releases](https://github.com/Chadi00/thr/releases)
- [Command guide](docs/commands.md)
- [Changelog](CHANGELOG.md)
- [License](LICENSE)
- [Third-party notices](THIRD_PARTY_NOTICES.md)
