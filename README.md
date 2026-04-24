# thr

**Tiny History Recall** — a local CLI that stores short notes and finds them again with **semantic** (“what matches this idea?”) and **keyword** search. Data stays on your machine; **retrieval does not call a cloud API**.

Useful for people and for coding agents that need a small, durable memory layer without running a server.

## Platform support

**Supported (first-class):**

| OS | CPU | Notes |
|----|-----|--------|
| **macOS** | **arm64** (Apple Silicon) | Prebuilt binary in [Releases](https://github.com/Chadi00/thr/releases). Source builds need Xcode Command Line Tools and ONNX Runtime (installer uses Homebrew when possible). |
| **macOS** | **x86_64** (Intel) | Same as above; prebuilt `thr_darwin_amd64.tar.gz` when published. |
| **Linux** | **x86_64** | Prebuilt `thr_linux_amd64.tar.gz`. **glibc**-based distros (e.g. Ubuntu 22.04+, Debian, Fedora) are expected; release builds are produced on Ubuntu (GitHub-hosted runners). |
| **Linux** | **arm64** (`aarch64`) | Prebuilt `thr_linux_arm64.tar.gz`; same glibc-oriented expectations. |

**Building from source** additionally requires **Go 1.26+** (see `go.mod`), a **C compiler** (CGO), **SQLite dev headers** (`libsqlite3-dev` or equivalent), and a working **ONNX Runtime** shared library at runtime (macOS: Homebrew `onnxruntime`; Linux: install or provide `libonnxruntime` / `onnxruntime.so` where the embedding stack can load it).

**Not supported:**

- **Windows** (no installer, no release artifacts, not tested).
- **32-bit** (`i386`, `armv7`, etc.).
- **Non-macOS BSD**, **iOS**, **Android**, and similar — not tested; you are on your own.
- **musl-centric** environments (e.g. stock Alpine) — not tested; linking and ONNX/SQLite may need extra work.

If you need a platform outside this list, open an issue with your OS, arch, and what you tried.

## Install (macOS or Linux)

**Recommended:** install a **prebuilt binary** from [GitHub Releases](https://github.com/Chadi00/thr/releases) (no local Go toolchain required):

```bash
curl -fsSL https://raw.githubusercontent.com/Chadi00/thr/master/install.sh | bash
```

The script picks the correct `thr_<os>_<arch>.tar.gz`, verifies SHA-256 checksums from `checksums.txt`, and installs `thr` to a standard location (macOS: Homebrew or `/opt/homebrew/bin` when possible; Linux: `~/.local/bin` by default). Useful environment variables:

- `THR_VERSION` — `latest` (default) or an exact tag (for example `v0.1.0`).
- `GITHUB_TOKEN` — optional; raises GitHub API rate limits for the release metadata request.
- `THR_USER_BIN` — override the Linux install directory (default `~/.local/bin`).
- `THR_USE_SOURCE=1` — build from source with Go + CGO instead of using a release binary.
- `THR_INSTALL_REF` — when using `THR_USE_SOURCE=1`, which git ref to build (branch, tag, or commit; default `master`).

**Go install** (works best with a **semver tag**, not a moving branch):

```bash
go install -tags sqlite_fts5 github.com/Chadi00/thr/cmd/thr@v0.1.0
```

### Publishing a release (maintainers)

Pushing a version tag builds native binaries (CGO) on GitHub Actions and uploads them to a GitHub Release:

```bash
git tag v0.1.0
git push origin v0.1.0
```

## Commands

| Command | What it does |
|--------|----------------|
| `thr add <text>` or `thr add -` | Save a new memory from text, or explicit stdin with `-` |
| `thr list` | List stored memories (with ids) |
| `thr show <id>` | Print one memory in full |
| `thr ask <question>` | **Semantic** search: memories closest in meaning to the question (retrieval only; no LLM answer text) |
| `thr search <query>` | **Default text recall**: FTS + recent literal substring + fuzzy ranking |
| `thr edit <id> <text>` or `thr edit <id> -` | Replace a memory’s text, or use explicit stdin with `-` |
| `thr forget <id>` | Delete a memory |
| `thr stats` | Show database path and memory count |
| `thr prefetch` | Download the embedding model into the cache so the first add or ask is not slow |
| `thr version` (or `thr` with `-v` / `--version`) | Print build version |
| `thr completion` | Print a shell completion script — use `bash`, `zsh`, or `fish` as the argument |

**Global options (work with any subcommand):** `--db <path>` (or env `THR_DB`) for the database file, `--json` on **read** commands (see below).

**Machine-oriented output:** add `--json` to `list`, `show`, `ask`, or `search` for stable JSON (good for scripts and agents).

## Examples

**Add a memory** (text argument, or explicit `-` for multiline/programmatic stdin):

```bash
thr add "prefers small CLIs with good docs"
printf "line one\nline two\n" | thr add -
```

**List, inspect, and search**

```bash
thr list
thr list --json
thr show 1
thr ask "what are their CLI preferences?"        # meaning-based matches
thr search "pref golnag cli"                     # text recall (FTS + substring + fuzzy)
thr ask "deployment?" --json
```

**Change and remove data**

```bash
thr edit 1 "updated text"
printf "line one\nline two\n" | thr edit 1 -
thr forget 1
```

**Database, model, and version**

```bash
thr stats
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
- **Semantic index:** [BAAI/bge-base-en-v1.5](https://huggingface.co/BAAI/bge-base-en-v1.5) vectors (768 dimensions) via sqlite-vec.
- **Text recall:** SQLite FTS5 plus bounded recent substring matching and fuzzy ranking.

## Contributing

Issues and pull requests are welcome. If something breaks, say your OS and the exact command you ran.
