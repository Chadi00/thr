#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
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
  local arch archive checksum

  arch="$(normalize_arch)"
  archive="thr_darwin_${arch}.tar.gz"
  mkdir -p "$WORK_DIR/release"
  create_stub_binary "$WORK_DIR/release/thr"
  tar -czf "$WORK_DIR/release/$archive" -C "$WORK_DIR/release" thr
  checksum="$(sha256_file "$WORK_DIR/release/$archive")"
  printf '%s  %s\n' "$checksum" "$archive" >"$WORK_DIR/release/checksums.txt"
  : >"$WORK_DIR/release/checksums.txt.minisig"
}

setup_fake_brew() {
  local brew_bin="$WORK_DIR/fakebrew/bin/brew"
  local minisign_bin="$WORK_DIR/fakebrew/bin/minisign"
  local brew_prefix="$WORK_DIR/homebrew"

  mkdir -p "$WORK_DIR/fakebrew/bin" "$brew_prefix/bin"
  : >"$brew_prefix/.onnxruntime-installed"
  cat >"$brew_bin" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
prefix="${THR_SMOKE_BREW_PREFIX:?}"
case "${1:-}" in
  --prefix)
    printf '%s\n' "$prefix"
    ;;
  list)
    if [[ "${2:-}" == '--versions' && "${3:-}" == 'onnxruntime' ]]; then
      if [[ -f "$prefix/.onnxruntime-installed" ]]; then
        printf 'onnxruntime 1.0.0\n'
        exit 0
      fi
      exit 1
    fi
    exit 1
    ;;
  install)
    if [[ "${2:-}" == 'onnxruntime' ]]; then
      : >"$prefix/.onnxruntime-installed"
      mkdir -p "$prefix/bin"
      exit 0
    fi
    exit 1
    ;;
  *)
    exit 1
    ;;
esac
EOF
  chmod +x "$brew_bin"
  cat >"$minisign_bin" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
exit 0
EOF
  chmod +x "$minisign_bin"
  export THR_SMOKE_BREW_PREFIX="$brew_prefix"
  export PATH="$WORK_DIR/fakebrew/bin:$brew_prefix/bin:$PATH"
}

prepare_home() {
  export HOME="$WORK_DIR/home"
  export SHELL=/bin/zsh
  mkdir -p "$HOME"
  : >"$HOME/.zshrc"
}

assert_path_block_present() {
  if grep -qF '# thr install:' "$HOME/.zshrc"; then
    return 0
  fi
  command -v thr >/dev/null 2>&1 || fail "expected PATH block in $HOME/.zshrc or thr already on PATH"
}

assert_path_block_removed() {
  if grep -qF '# thr install:' "$HOME/.zshrc"; then
    fail "expected PATH block to be removed from $HOME/.zshrc"
  fi
}

assert_thr_usable() {
  zsh -c 'source "$HOME/.zshrc" && command -v thr >/dev/null && thr --help >/dev/null && thr prefetch >/dev/null'
}

assert_agent_skill_prompt_skipped() {
  local path

  for path in \
    "$HOME/.claude/skills/thr/SKILL.md" \
    "$HOME/.config/opencode/skills/thr/SKILL.md" \
    "$HOME/.agents/skills/thr/SKILL.md"; do
    if [[ -e "$path" ]]; then
      fail "optional agent skill setup should be skipped in smoke test, but found $path"
    fi
  done
}

main() {
  local release_base_url install_dir

  if [[ "$(uname -s)" != 'Darwin' ]]; then
    fail 'installer smoke is macOS-only'
  fi

  prepare_home
  release_base_url="${THR_INSTALL_TEST_BASE_URL:-}"
  if [[ -z "$release_base_url" ]]; then
    setup_fake_brew
    create_release_fixture
    release_base_url="file://$WORK_DIR/release"
    install_dir="$WORK_DIR/homebrew/bin"
    export THR_UNINSTALL_TEST_BIN_DIRS="$WORK_DIR/homebrew/bin"
  else
    command -v brew >/dev/null 2>&1 || fail 'release smoke requires Homebrew on macOS runners'
    brew list --versions minisign >/dev/null 2>&1 || brew install minisign
    brew list --versions onnxruntime >/dev/null 2>&1 || brew install onnxruntime
    install_dir="$(brew --prefix)/bin"
    unset THR_UNINSTALL_TEST_BIN_DIRS || true
  fi

  log 'Running install smoke test'
  THR_INSTALL_TEST_BASE_URL="$release_base_url" THR_INSTALL_SKIP_SKILL_PROMPT=1 bash "$ROOT_DIR/install.sh"

  [[ -x "$install_dir/thr" ]] || fail 'install did not place thr in the Homebrew bin dir'
  assert_path_block_present
  assert_thr_usable
  assert_agent_skill_prompt_skipped
  mkdir -p "$HOME/.thr/models"
  : >"$HOME/.thr/thr.db"
  : >"$HOME/.thr/models/model"

  log 'Running uninstall smoke test'
  bash "$ROOT_DIR/uninstall.sh"

  [[ ! -e "$install_dir/thr" ]] || fail 'uninstall left thr behind'
  [[ -e "$HOME/.thr/thr.db" ]] || fail 'uninstall removed data without confirmation'
  [[ -e "$HOME/.thr/models/model" ]] || fail 'uninstall removed model cache without confirmation'
  assert_path_block_removed
}

main "$@"
