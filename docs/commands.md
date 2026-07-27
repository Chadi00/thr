# Command Guide

This guide covers every `thr` command, its flags, and the common workflows for
storing, finding, and managing memories.

## Basics

```text
thr [global flags] <command> [command flags] [arguments]
```

Run `thr --help` to list commands or `thr <command> --help` for built-in help.

### Global Flags

| Flag | Purpose |
| --- | --- |
| `--db <path>` | Use a specific SQLite database. Overrides `THR_DB`; the default is `~/.thr/thr.db`. |
| `--cwd <directory>` | Resolve repository context from another directory without changing the shell directory. |
| `--format human` | Use the default human-readable output. |
| `--format json-v2` | Use the versioned JSON envelope recommended for agents and scripts. |
| `--format legacy-json` | Use the deprecated JSON format supported by read-oriented commands. |
| `--json` | Alias for `--format legacy-json`; cannot be combined with `--format`. |
| `-v`, `--version` | Print version information. |
| `-h`, `--help` | Show help. |

Configuration precedence:

| Data | Selection order |
| --- | --- |
| Database | `--db`, `THR_DB`, `~/.thr/thr.db` |
| Model cache | `THR_MODEL_CACHE`, `~/.thr/models` |
| Installer prefix | `THR_INSTALL_PREFIX`, `~/.local` |

Examples:

```bash
thr --db ./memories.db list
THR_DB=./memories.db thr stats
thr --cwd ../other-repo ask "release process"
thr --format json-v2 context
```

## Scope Rules

Every memory belongs to one scope:

- `user`: available across repositories in the selected database.
- `repo`: the repository resolved from the current directory or `--cwd`.
- `repo:<id>`: a specific persisted repository scope.

Inside a repository, unqualified `add` writes to that repository. Default
`list`, `search`, and `ask` operations search the current repository scope, when
one exists, plus `user`.

Outside a repository, default reads search `user`, while writes require an
explicit scope:

```bash
thr add --scope user "The user prefers concise explanations"
```

Read commands accept repeated scopes or all scopes:

```bash
thr search --scope repo --scope user "release process"
thr list --all-scopes
```

Do not combine `--scope` with `--all-scopes`. Exact-ID commands (`show`, `edit`,
`forget`, and `move`) can address a memory from any directory.

## Output

Human output is the default. Collection commands use labeled columns. For
example, `thr scope list` prints:

```text
SCOPE  TYPE        MEMORIES  LABEL                        CURRENT
user   user        1         -                            false
repo:01KABC  repository  4         github.com/example/project  true
```

- `SCOPE`: stable scope ID used by commands.
- `TYPE`: `user` or `repository`.
- `MEMORIES`: number of memories in that scope.
- `LABEL`: human-readable repository label; `-` means the ID is sufficient.
- `CURRENT`: whether this is the current repository scope.

Human warnings and errors are written to stderr. When one action can resolve a
problem, output includes a `Next:` line.

Use JSON v2 for automation:

```bash
thr --format json-v2 list
```

JSON v2 includes the command name, database and working-directory context,
scope selection where applicable, result, warnings, and structured errors.
Legacy JSON is supported only by `list`, `show`, `search`, `ask`, and `stats`;
it omits scope metadata and should not be used for new integrations.

## Memory Commands

### Add A Memory

```text
thr add "<text>"|- [--scope <scope>] [--max-bytes <bytes>]
```

Multiword direct text must be wrapped in ASCII double quotes so the shell
passes it as one argument. Use `thr add -` to read from stdin instead.

Store a repository memory from inside a Git repository:

```bash
thr add "This repository uses pnpm"
```

Store a user-wide memory:

```bash
thr add --scope user "The user prefers concise explanations"
```

Store multiline text from stdin:

```bash
printf 'First line\nSecond line\n' | thr add -
```

Flags:

| Flag | Default | Purpose |
| --- | --- | --- |
| `--scope user|repo|repo:<id>` | Current repository | Select exactly one destination. |
| `--max-bytes <bytes>` | `262144` | Set the maximum UTF-8 text size. |

Memory text has a non-overridable limit of 508 Unicode code points so the
semantic model embeds the complete memory without truncation. `--max-bytes`
can impose a lower input limit but cannot raise the code-point limit.

`add` creates the embedding immediately. An unqualified write outside a
repository fails instead of silently creating a user-wide memory.

### List Memories

```text
thr list [flags]
```

```bash
thr list
thr list --last 20
thr list --scope user
thr list --scope repo --scope user
thr list --all-scopes
thr list --scope user --legacy
```

Flags:

| Flag | Default | Purpose |
| --- | --- | --- |
| `-n`, `--limit <count>` | `100` | Limit results; maximum `1000`. |
| `--last <count>` | `100` | Alias for `--limit`. |
| `--scope <scope>` | Automatic visible scopes | Select a scope; repeat for a union. |
| `--all-scopes` | Off | List every registered scope. |
| `--legacy` | Off | Include only migrated legacy assignments. |

### Show A Memory

```text
thr show <id>
```

```bash
thr show 42
thr --format json-v2 show 42
```

The human detail view labels the memory ID, scope, revision, timestamps, and
text. IDs are global within the selected database, so repository context does
not affect this command.

### Edit A Memory

```text
thr edit <id> <text|-> [flags]
```

```bash
thr edit 42 "Updated memory text"
printf 'Updated\nmultiline text\n' | thr edit 42 -
thr edit 42 "Updated text" --if-revision 3 --if-scope user
```

Flags:

| Flag | Purpose |
| --- | --- |
| `--max-bytes <bytes>` | Set the maximum replacement text size; default `262144`. |
| `--if-scope <scope-id>` | Update only if the memory remains in this exact scope. |
| `--if-revision <revision>` | Update only if the revision still matches. |

Replacement text has the same non-overridable 508 Unicode-code-point limit as
new memories.

Editing replaces the text and embedding, updates the content timestamp, and
increments the revision. It does not change the scope.

### Delete A Memory

```text
thr forget <id> [flags]
```

```bash
thr forget 42
thr forget 42 --if-scope user --if-revision 3
```

`forget` permanently deletes the memory and its embedding. `--if-scope` and
`--if-revision` provide the same concurrency protection as `edit`.

### Move A Memory

```text
thr move <id> --to <scope> [flags]
```

```bash
thr move 42 --to user
thr move 42 --to repo
thr move 42 --to repo:01KABCDEF0123456789ABCDEFG
thr move 42 --to user --if-scope repo:01KOLD --if-revision 3
```

Flags:

| Flag | Purpose |
| --- | --- |
| `--to user|repo|repo:<id>` | Required destination scope; specify exactly once. |
| `--if-scope <scope-id>` | Move only if the current scope matches. |
| `--if-revision <revision>` | Move only if the current revision matches. |

Moving preserves the ID, text, content timestamps, and embedding. It records an
explicit scope assignment and increments the revision when state changes.

## Retrieval Commands

### Text Search

```text
thr search <query> [flags]
```

```bash
thr search "release notes"
thr search -n 5 "release notes"
thr search --scope repo --scope user "pnpm workspace"
thr search --all-scopes "deployment"
```

`search` combines full-text, substring, and typo-tolerant matching. It does not
initialize the embedding model.

Flags:

| Flag | Default | Purpose |
| --- | --- | --- |
| `-n`, `--limit <count>` | `10` | Limit results; values are clamped to `1` through `100`. |
| `--scope <scope>` | Automatic visible scopes | Select a scope; repeat for a union. |
| `--all-scopes` | Off | Search every registered scope. |

### Semantic Recall

```text
thr ask <question> [flags]
```

```bash
thr ask "How should releases be prepared?"
thr ask -n 5 "How should releases be prepared?"
thr ask --max-distance 1.2 "release process"
thr ask --with-distance "release process"
thr ask --scope repo --scope user "coding conventions"
```

`ask` embeds the question and returns semantically similar memories. It does not
call an LLM or generate an answer.

Flags:

| Flag | Default | Purpose |
| --- | --- | --- |
| `-n`, `--limit <count>` | `3` | Limit semantic results; maximum `100`. |
| `--max-distance <distance>` | `0.80` | Exclude weaker matches; valid range is greater than `0` through `4`. |
| `--with-distance` | Off | Include distance in human output. Lower is more similar. |
| `--scope <scope>` | Automatic visible scopes | Select a scope; repeat for a union. |
| `--all-scopes` | Off | Search every registered scope. |

If selected memories have missing or stale embeddings, repair them and retry:

```bash
thr index
thr ask "How should releases be prepared?"
```

## Inspection And Maintenance

### Show Context

```text
thr context
```

```bash
thr context
thr --format json-v2 context
thr --cwd /work/api --format json-v2 context
```

`context` reports:

- Database path and compatibility status.
- Working directory used for repository resolution.
- Current or prospective repository scope.
- Default write scope.
- Default read scopes.
- Repository resolution status and warnings.

This command is read-only. It does not create or migrate a database, bind a
repository, or prepare the embedding model.

### Show Statistics

```text
thr stats [flags]
```

```bash
thr stats
thr stats --scope user
thr stats --scope repo --scope user
thr stats --all-scopes
```

`stats` reports the database, embedding model, index health, and per-scope
memory counts. It defaults to all scopes.

Flags:

| Flag | Purpose |
| --- | --- |
| `--scope <scope>` | Restrict statistics; repeat for a union. |
| `--all-scopes` | Explicitly inspect every scope. |

### Update The Semantic Index

```text
thr index [flags]
```

```bash
thr index
thr index --scope user
thr index --scope repo --scope user
thr index --all-scopes
```

`index` defaults to all scopes and rebuilds only missing or stale embeddings for
the active model.

Flags:

| Flag | Purpose |
| --- | --- |
| `--scope <scope>` | Restrict indexing; repeat for a union. |
| `--all-scopes` | Explicitly index every scope. |

## Scope Management

Most users only need automatic repository scopes. These commands handle clones,
ambiguous remotes, moved checkouts, and intentional scope separation.

### List Scopes

```bash
thr scope list
thr --format json-v2 scope list
```

Lists every persisted user and repository scope with memory counts and the
current repository marker.

### Show A Scope

```bash
thr scope show user
thr scope show repo
thr scope show repo:01KABCDEF0123456789ABCDEFG
```

Shows the scope type, label, memory count, latest update, aliases, and checkout
bindings. `repo` means the current repository; a stable `repo:<id>` works from
any directory.

### Create A Repository Scope

```bash
thr scope create repo
thr --cwd /work/api scope create repo
```

Creates and binds an empty scope for the selected checkout. If the checkout
already matches a scope, use `bind` or `split` instead.

### Bind A Checkout

```bash
thr scope bind repo:01KABCDEF0123456789ABCDEFG
thr --cwd /work/api scope bind repo:01KABCDEF0123456789ABCDEFG
```

Binds the checkout to an existing repository scope. It does not move memories.
Use `rebind` if the checkout already has a binding.

### Unbind A Checkout

```bash
thr scope unbind
thr scope unbind --confirm-orphan
```

Removes the checkout binding without deleting the scope or memories. The
`--confirm-orphan` flag is required when removing the last binding from a
local-only scope.

### Rebind A Checkout

```bash
thr scope rebind repo:01KNEWDEF0123456789ABCDEFG
```

Transfers the current checkout binding to another existing scope. Memories stay
in their original scopes.

### Rename A Scope

```bash
thr scope rename repo:01KABCDEF0123456789ABCDEFG "payments API"
```

Changes only the human-readable label. The scope ID, bindings, aliases, and
memories remain unchanged.

### Split A Scope

```bash
thr scope split
thr --cwd /work/api scope split
```

Creates a new empty scope and binds the selected checkout to it. Existing
memories stay in the old scope. Future clones sharing that remote may need an
explicit `scope bind` because the remote now maps to multiple scopes.

### Resolve Ambiguous Remotes

Repository identity prefers the local `thr.identityRemote` setting, then
`origin`, then a sole configured remote. Select an identity remote when a
repository has multiple remotes and no `origin`:

```bash
git config --local thr.identityRemote upstream
thr context
```

## Database Migration

```text
thr migrate
```

```bash
thr migrate
thr --db /path/to/another.db migrate
thr --format json-v2 migrate
```

Operational commands migrate legacy databases automatically. `migrate` runs the
same process explicitly:

1. Creates and verifies a private timestamped backup beside the database.
2. Preserves memory IDs, text, timestamps, embeddings, and model metadata.
3. Assigns legacy memories to `user` with legacy provenance.
4. Replaces the old schema transactionally.

Review migrated assignments:

```bash
thr list --scope user --legacy
thr move 42 --to repo
thr move 17 --to user
```

Migrated databases cannot be opened by older binaries. Restore the reported
backup to downgrade.

## Model Preparation

```text
thr prefetch
```

```bash
thr prefetch
THR_MODEL_CACHE=/private/thr-models thr prefetch
thr --format json-v2 prefetch
```

Prepares and verifies the bundled embedding model and ONNX Runtime. It does not
open, create, or migrate the memory database.

## Agent Skill Setup

```bash
thr setup claude-code
thr setup opencode
thr setup codex
```

The commands install the managed `thr` Agent Skill:

| Agent | Path |
| --- | --- |
| Claude Code | `~/.claude/skills/thr/SKILL.md` |
| OpenCode | `~/.agents/skills/thr/SKILL.md` |
| Codex | `~/.agents/skills/thr/SKILL.md` |

OpenCode and Codex intentionally share the same file. Managed skill versions
are updated automatically. An unmanaged regular file is preserved unless
`--force` is supplied:

```bash
thr setup opencode --force
```

Symlinks and non-regular files are never overwritten.

## Update

```bash
thr update
```

Downloads the latest release's installer and verifies it against the signed
release checksums before running it. The command preserves the current
installation prefix and updates the binary, packaged runtime, selected database,
and managed agent skills.

## Version

```bash
thr version
thr --version
thr -v
thr --format json-v2 version
```

Prints the build version, commit, and build date without accessing the database,
repository, model, or agent skills.

## Common Workflows

### Save And Recall Repository Knowledge

```bash
thr add "Integration tests require Docker"
thr ask "How do I run integration tests?"
thr search "Docker"
```

### Save A Cross-Repository Preference

```bash
thr add --scope user "The user prefers concise explanations"
thr ask --scope user "response style"
```

### Correct A Memory Safely

```bash
thr --format json-v2 show 42
thr edit 42 "Corrected text" --if-scope user --if-revision 3
```

### Work With Another Checkout

```bash
thr --cwd ../other-repo context
thr --cwd ../other-repo ask "release process"
thr --cwd ../other-repo add "Releases require a clean working tree"
```

### Inspect Everything

```bash
thr scope list
thr stats --all-scopes
thr list --all-scopes
```
