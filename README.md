# thr

**Tiny History Recall** — a local CLI that stores short notes and finds them again with **semantic** (“what matches this idea?”) and **keyword** search. Data stays on your machine; **retrieval does not call a cloud API**.

Useful for people and for coding agents that need a small, durable memory layer without running a server.

## Install (macOS or Linux)

```bash
curl -fsSL https://raw.githubusercontent.com/Chadi00/thr/master/install.sh | bash
```

## Commands

| Command | What it does |
|--------|----------------|
| `thr add [text]`, `thr add -f <file>`, or stdin | Save a new memory (text, file, or pipe) |
| `thr list` | List stored memories (with ids) |
| `thr show <id>` | Print one memory in full |
| `thr ask <question>` | **Semantic** search: memories closest in meaning to the question (retrieval only; no LLM answer text) |
| `thr search <query>` | **Keyword** search (SQLite FTS5 on words/tokens) |
| `thr search --substring <query>` | **Substring** search over the raw text (SQL `LIKE`-style) |
| `thr edit <id> [text]`, `thr edit <id> -f <file>`, or stdin | Replace a memory’s text |
| `thr forget <id>` | Delete a memory |
| `thr export` | Write all memories to JSONL (stdout) |
| `thr import` | Import JSONL produced by `thr export` (from a file path, `thr import -` for stdin, or `-f <file>`) |
| `thr stats` | Show database path and memory count |
| `thr vacuum` | Run `VACUUM` on the SQLite database |
| `thr prefetch` | Download the embedding model into the cache so the first add or ask is not slow |
| `thr version` (or `thr` with `-v` / `--version`) | Print build version |
| `thr completion` | Print a shell completion script — use `bash`, `zsh`, or `fish` as the argument |

**Global options (work with any subcommand):** `--db <path>` (or env `THR_DB`) for the database file, `--json` on **read** commands (see below).

**Machine-oriented output:** add `--json` to `list`, `show`, `ask`, or `search` for stable JSON (good for scripts and agents).

## Examples

**Add a memory** (argument, file, or pipe — pipes are for multiline or programmatic input):

```bash
thr add "prefers small CLIs with good docs"
thr add -f ./note.txt
printf "line one\nline two\n" | thr add
```

**List, inspect, and search**

```bash
thr list
thr list --json
thr show 1
thr ask "what are their CLI preferences?"        # meaning-based matches
thr search "cli"                                 # word/token search (FTS5)
thr search --substring "pref"                    # raw substring
thr ask "deployment?" --json
```

**Change, remove, and move data**

```bash
thr edit 1 "updated text"
thr forget 1
thr export > backup.jsonl
thr import backup.jsonl
```

**Database, model, and version**

```bash
thr stats
thr vacuum
thr prefetch
thr version
# same as: thr -v
```

**Shell completion** (write the script to a file or `source` it, depending on your shell — zsh and bash often use `source <(thr completion zsh)`).

```bash
source <(thr completion zsh)
```

**Full help:** `thr --help` and `thr <command> --help`.

### Memory ids

Use the numeric `id` from `thr list` (or from `ask` / `search` output) in `show`, `edit`, and `forget`.

## Where data lives

- **Database:** `~/.thr/thr.db` by default (override: `--db` or `THR_DB`).
- **Embedding model cache:** under `~/.thr/models` by default (override: `THR_MODEL_CACHE`).
- **Semantic index:** [BAAI/bge-base-en-v1.5](https://huggingface.co/BAAI/bge-base-en-v1.5) vectors (768 dimensions) via sqlite-vec; **keywords:** SQLite FTS5.

## Contributing

Issues and pull requests are welcome. If something breaks, say your OS and the exact command you ran.
