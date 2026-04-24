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
| `thr list` | List stored memories |
| `thr ask <question>` | Semantic search over memories |
| `thr search <query>` | Keyword (FTS) search |
| `thr edit <id> <text>` | Replace a memory’s text |
| `thr forget <id>` | Delete a memory |

### Examples

```bash
thr add "the user prefers Rust for new services"
thr list
thr ask "what language should we use for a new service?"
thr search "Rust"
thr edit 1 "the user prefers Go for new services"
thr forget 1
```

Use the numeric `id` from `thr list` (or the output of `thr ask` / `thr search`) for `edit` and `forget`.

### Where data lives

- **Database:** `~/.thr/thr.db` by default
- **Semantic index:** `sqlite-vec` vectors (`float[768]`) for [BAAI/bge-base-en-v1.5](https://huggingface.co/BAAI/bge-base-en-v1.5)
- **Keywords:** SQLite FTS5

## Contributing

Issues and pull requests are welcome. If the installer or CLI misbehaves, say your OS and what you ran.
