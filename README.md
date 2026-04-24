# thr

`thr` (Tiny History Recall) is a local-first memory CLI for coding agents.

## Install or update in one command

Run this from anywhere:

```bash
curl -fsSL https://raw.githubusercontent.com/Chadi00/thr/master/install.sh | bash
```

This command:

- installs required dependencies (Go toolchain, CGO build tools, ONNX Runtime) when possible
- installs `thr` globally via `go install`
- updates to the latest `thr` version when re-run

If you are using a private fork/repo, use an authenticated install command:

```bash
gh api repos/Chadi00/thr/contents/install.sh?ref=master --jq '.content' | base64 --decode | bash
```

## Commands

- `thr add <text>` stores a memory.
- `thr list` lists stored memories.
- `thr ask <question>` runs semantic retrieval over memories.
- `thr search <query>` runs keyword search over memories.
- `thr edit <id> <text>` replaces memory text.
- `thr forget <id>` deletes a memory.

## Storage and search

- SQLite database at `~/.thr/thr.db` by default.
- Semantic vectors in `sqlite-vec` (`float[768]`) for `BAAI/bge-base-en-v1.5`.
- Keyword search via SQLite FTS5.

## Build prerequisites

### CGO + C toolchain

This project uses `github.com/mattn/go-sqlite3` and `sqlite-vec` CGO bindings.

- macOS: Xcode Command Line Tools
- Linux: gcc/clang + libc headers

### ONNX Runtime

Embeddings use `github.com/bdombro/fastembed-go` with ONNX Runtime.

- macOS: `brew install onnxruntime`
- If auto-detection fails, set `ONNX_PATH` to your ONNX Runtime shared library.

Examples:

```bash
export ONNX_PATH="/opt/homebrew/lib/libonnxruntime.dylib"
# Linux example:
# export ONNX_PATH="/path/to/libonnxruntime.so"
```

## Run locally

```bash
go run ./cmd/thr add "the user likes sports cars"
go run ./cmd/thr ask "what type of car does the user like?"
go run ./cmd/thr search "sports cars"
go run ./cmd/thr list
```

