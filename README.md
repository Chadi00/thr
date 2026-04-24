# thr

**Tiny History Recall** — a local-first CLI that stores short notes and answers questions with semantic + keyword search. Data stays on your machine; nothing is sent to a cloud API for retrieval.

Ideal for coding agents and humans who want a durable, grep-friendly memory layer without running a server.

## Install

One line (macOS or Linux):

```bash
curl -fsSL https://raw.githubusercontent.com/Chadi00/thr/master/install.sh | bash
```

## Usage

| Command | What it does |
|--------|----------------|
| `thr add <text>` | Store a memory |
| `thr add --file <path>` | Store memory text from file |
| `thr list` | List stored memories |
| `thr show <id>` | Show one memory in full |
| `thr ask <question>` | Semantic retrieval over memories (no LLM generation) |
| `thr search <query>` | Keyword (FTS5) search |
| `thr search --substring <query>` | Substring search over raw text |
| `thr edit <id> <text>` | Replace a memory’s text |
| `thr forget <id>` | Delete a memory |
| `thr export` | Export memories to JSONL |
| `thr import <file>` | Import memories from JSONL |
| `thr stats` | Show database path and memory count |
| `thr vacuum` | Run SQLite VACUUM |
| `thr version` / `thr --version` | Show build version |

### Examples

```bash
thr add "the user prefers Rust for new services"
printf "multi-line\ninput" | thr add
thr list
thr show 1
thr ask "what language should we use for a new service?"
thr search "Rust"
thr search --substring "lang"
thr edit 1 "the user prefers Go for new services"
thr export > backup.jsonl
thr import backup.jsonl
thr forget 1
```

Use the numeric `id` from `thr list` (or the output of `thr ask` / `thr search`) for `edit` and `forget`.

### Where data lives

- **Database:** `~/.thr/thr.db` by default
- **DB override:** `--db <path>` or `THR_DB=<path>`
- **Model cache override:** `THR_MODEL_CACHE=<path>`
- **Semantic index:** `sqlite-vec` vectors (`float[768]`) for [BAAI/bge-base-en-v1.5](https://huggingface.co/BAAI/bge-base-en-v1.5)
- **Keywords:** SQLite FTS5

### JSON output

Use `--json` with read-oriented commands (`list`, `show`, `search`, `ask`) for stable machine output.

### Shell completion

Generate and load shell completion:

```bash
source <(thr completion zsh)
```

Add that line to your shell profile (for zsh, usually `~/.zshrc`) to enable completion in new shells.

## Contributing

Issues and pull requests are welcome. If the installer or CLI misbehaves, say your OS and what you ran.
