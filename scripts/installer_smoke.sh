#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
MODE="${1:-binary}"
WORK_DIR="$(mktemp -d "${TMPDIR:-/tmp}/thr-smoke.XXXXXX")"

cleanup() {
  rm -rf "$WORK_DIR"
}

trap cleanup EXIT

log() {
  printf '[thr-smoke] %s\n' "$*"
}

fail() {
  printf '[thr-smoke] %s\n' "$*" >&2
  exit 1
}

normalize_arch() {
  case "$(uname -m)" in
    x86_64 | amd64) printf '%s' 'amd64' ;;
    arm64 | aarch64) printf '%s' 'arm64' ;;
    *) fail "unsupported architecture: $(uname -m)" ;;
  esac
}

normalize_os() {
  case "$(uname -s)" in
    Darwin) printf '%s' 'darwin' ;;
    Linux) printf '%s' 'linux' ;;
    *) fail "unsupported OS: $(uname -s)" ;;
  esac
}

sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

create_stub_binary() {
  local path="$1"
  cat >"$path" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
case "${1:-}" in
  prefetch)
    printf 'prefetch ok\n'
    ;;
  --help | help | "")
    printf 'stub thr help\n'
    ;;
  *)
    printf 'stub thr %s\n' "$*"
    ;;
esac
EOF
  chmod +x "$path"
}

create_release_fixture() {
  local goos goarch archive checksum
  goos="$(normalize_os)"
  goarch="$(normalize_arch)"
  archive="thr_${goos}_${goarch}.tar.gz"

  mkdir -p "$WORK_DIR/release"
  create_stub_binary "$WORK_DIR/release/thr"
  tar -czf "$WORK_DIR/release/$archive" -C "$WORK_DIR/release" thr
  checksum="$(sha256_file "$WORK_DIR/release/$archive")"
  printf '%s  %s\n' "$checksum" "$archive" >"$WORK_DIR/release/checksums.txt"

  cat >"$WORK_DIR/release/release.json" <<EOF
{
  "assets": [
    {
      "name": "$archive",
      "browser_download_url": "file://$WORK_DIR/release/$archive"
    },
    {
      "name": "checksums.txt",
      "browser_download_url": "file://$WORK_DIR/release/checksums.txt"
    }
  ]
}
EOF
}

create_source_fixture() {
  mkdir -p "$WORK_DIR/source/cmd/thr"
  cat >"$WORK_DIR/source/go.mod" <<'EOF'
module github.com/Chadi00/thr

go 1.26.2
EOF

  cat >"$WORK_DIR/source/cmd/thr/main.go" <<'EOF'
package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "prefetch" {
		fmt.Println("prefetch ok")
		return
	}
	fmt.Println("stub thr help")
}
EOF

  tar -czf "$WORK_DIR/thr-source.tar.gz" -C "$WORK_DIR" source
}

prepare_home() {
  local name="$1"
  export HOME="$WORK_DIR/home-$name"
  export SHELL=/bin/bash
  mkdir -p "$HOME"
  : >"$HOME/.bashrc"
}

resolve_install_dir() {
  if [[ "${THR_SMOKE_USE_DEFAULT_DIR:-}" == '1' ]]; then
    case "$(uname -s)" in
      Linux) printf '%s' "${THR_USER_BIN:-$HOME/.local/bin}" ;;
      Darwin)
        if command -v brew >/dev/null 2>&1; then
          printf '%s' "$(brew --prefix)/bin"
        else
          printf '%s' '/usr/local/bin'
        fi
        ;;
      *) fail "unsupported OS: $(uname -s)" ;;
    esac
    return 0
  fi

  printf '%s' "$HOME/bin"
}

assert_path_block_present() {
  grep -qF '# thr install:' "$HOME/.bashrc" || fail "expected PATH block in $HOME/.bashrc"
}

assert_path_block_removed() {
  if grep -qF '# thr install:' "$HOME/.bashrc"; then
    fail "expected PATH block to be removed from $HOME/.bashrc"
  fi
}

assert_thr_usable() {
  if grep -qF '# thr install:' "$HOME/.bashrc"; then
    bash -c 'source "$HOME/.bashrc" && command -v thr >/dev/null && thr --help >/dev/null && thr prefetch >/dev/null'
    return 0
  fi

  command -v thr >/dev/null
  thr --help >/dev/null
  thr prefetch >/dev/null
}

run_binary_smoke() {
  local install_dir release_json
  prepare_home binary
  install_dir="$(resolve_install_dir)"
  release_json="${THR_SMOKE_RELEASE_JSON:-file://$WORK_DIR/release/release.json}"

  log "Running binary install smoke test"
  if [[ "${THR_SMOKE_USE_DEFAULT_DIR:-}" == '1' ]]; then
    THR_RELEASE_API_URL="$release_json" bash "$ROOT_DIR/install.sh"
  else
    THR_INSTALL_DIR="$install_dir" THR_RELEASE_API_URL="$release_json" bash "$ROOT_DIR/install.sh"
  fi

  [[ -x "$install_dir/thr" ]] || fail "binary install did not place thr in $install_dir"
  assert_path_block_present
  assert_thr_usable

  log "Running uninstall smoke test"
  if [[ "${THR_SMOKE_USE_DEFAULT_DIR:-}" == '1' ]]; then
    bash "$ROOT_DIR/uninstall.sh"
  else
    THR_INSTALL_DIR="$install_dir" bash "$ROOT_DIR/uninstall.sh"
  fi

  [[ ! -e "$install_dir/thr" ]] || fail "uninstall left thr behind in $install_dir"
  assert_path_block_removed
}

run_source_smoke() {
  local install_dir source_archive
  prepare_home source
  install_dir="$(resolve_install_dir)"
  source_archive="${THR_SMOKE_SOURCE_ARCHIVE_URL:-file://$WORK_DIR/thr-source.tar.gz}"

  log "Running source install smoke test"
  if [[ "${THR_SMOKE_USE_DEFAULT_DIR:-}" == '1' ]]; then
    THR_USE_SOURCE=1 THR_SOURCE_ARCHIVE_URL="$source_archive" bash "$ROOT_DIR/install.sh"
  else
    THR_INSTALL_DIR="$install_dir" THR_USE_SOURCE=1 THR_SOURCE_ARCHIVE_URL="$source_archive" bash "$ROOT_DIR/install.sh"
  fi

  [[ -x "$install_dir/thr" ]] || fail "source install did not place thr in $install_dir"
  assert_path_block_present
  assert_thr_usable

  if [[ "${THR_SMOKE_USE_DEFAULT_DIR:-}" == '1' ]]; then
    bash "$ROOT_DIR/uninstall.sh"
  else
    THR_INSTALL_DIR="$install_dir" bash "$ROOT_DIR/uninstall.sh"
  fi
  [[ ! -e "$install_dir/thr" ]] || fail "source uninstall left thr behind in $install_dir"
  assert_path_block_removed
}

if [[ -z "${THR_SMOKE_RELEASE_JSON:-}" ]]; then
  create_release_fixture
fi
if [[ -z "${THR_SMOKE_SOURCE_ARCHIVE_URL:-}" ]]; then
  create_source_fixture
fi

case "$MODE" in
  binary) run_binary_smoke ;;
  source) run_source_smoke ;;
  *) fail "unknown mode: $MODE" ;;
esac
