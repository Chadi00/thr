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

normalize_os() {
  case "$(uname -s)" in
    Darwin) printf '%s' 'darwin' ;;
    Linux) printf '%s' 'linux' ;;
    *) fail "unsupported operating system: $(uname -s)" ;;
  esac
}

runtime_library_name() {
  case "$1" in
    darwin) printf '%s' 'libonnxruntime.dylib' ;;
    linux) printf '%s' 'libonnxruntime.so' ;;
    *) fail "unsupported runtime operating system: $1" ;;
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
  local os arch target archive checksum runtime_lib
  local stage="$WORK_DIR/stage"
  local runtime_dir

  os="$(normalize_os)"
  arch="$(normalize_arch)"
  target="${os}-${arch}"
  archive="thr_${os}_${arch}.tar.gz"
  runtime_lib="$(runtime_library_name "$os")"
  mkdir -p "$WORK_DIR/release"
  runtime_dir="$stage/lib/thr/onnxruntime/1.25.1/${target}"
  mkdir -p "$stage/bin" "$runtime_dir"
  create_stub_binary "$stage/bin/thr"
  printf 'stub onnxruntime\n' >"${runtime_dir}/${runtime_lib}"
  cat >"$stage/manifest.json" <<EOF
{"schema_version":1,"target":"${target}","thr":{"path":"bin/thr"},"onnxruntime":{"version":"1.25.1","library_path":"lib/thr/onnxruntime/1.25.1/${target}/${runtime_lib}"}}
EOF
  tar -czf "$WORK_DIR/release/$archive" -C "$stage" bin lib manifest.json
  checksum="$(sha256_file "$WORK_DIR/release/$archive")"
  printf '%s  %s\n' "$checksum" "$archive" >"$WORK_DIR/release/checksums.txt"
}

sign_release_fixture() {
  local key="$WORK_DIR/signing_key"
  local pub

  ssh-keygen -q -t ed25519 -N '' -C 'thr-smoke' -f "$key"
  ssh-keygen -Y sign -f "$key" -n thr-release "$WORK_DIR/release/checksums.txt" >/dev/null 2>/dev/null
  pub="$(cat "${key}.pub")"
  export THR_INSTALL_ALLOWED_SIGNERS="thr-release ${pub}"
}

hide_homebrew_from_path() {
  local path_entry filtered=""

  IFS=: read -r -a path_parts <<<"$PATH"
  for path_entry in "${path_parts[@]}"; do
    case "$path_entry" in
      *homebrew* | /usr/local/bin)
        continue
        ;;
    esac
    if [[ -z "$filtered" ]]; then
      filtered="$path_entry"
    else
      filtered="${filtered}:$path_entry"
    fi
  done
  export PATH="$filtered"
}

prepare_home() {
  export HOME="$WORK_DIR/home"
  if command -v zsh >/dev/null 2>&1; then
    export SHELL=/bin/zsh
  else
    export SHELL=/bin/bash
  fi
  mkdir -p "$HOME"
  case "$(basename "$SHELL")" in
    zsh) : >"$HOME/.zshrc" ;;
    *) : >"$HOME/.bashrc" ;;
  esac
}

assert_path_block_present() {
  local rc
  rc="$(shell_rc_file)"
  if grep -qF '# thr install:' "$rc"; then
    return 0
  fi
  command -v thr >/dev/null 2>&1 || fail "expected PATH block in ${rc} or thr already on PATH"
}

assert_path_block_removed() {
  local rc
  rc="$(shell_rc_file)"
  if grep -qF '# thr install:' "$rc"; then
    fail "expected PATH block to be removed from ${rc}"
  fi
}

shell_rc_file() {
  case "$(basename "${SHELL:-/bin/bash}")" in
    zsh) printf '%s' "$HOME/.zshrc" ;;
    bash) printf '%s' "$HOME/.bashrc" ;;
    *) printf '%s' "$HOME/.profile" ;;
  esac
}

assert_thr_usable() {
  "$SHELL" -c "source \"\$1\" && command -v thr >/dev/null && thr --help >/dev/null && thr prefetch >/dev/null" _ "$(shell_rc_file)"
  if [[ "${THR_SMOKE_REAL_BINARY:-0}" == "1" ]]; then
    "$SHELL" -c "source \"\$1\" && thr add 'thr installer smoke memory about release validation' >/dev/null && thr ask --max-distance 4 'release validation memory' >/dev/null" _ "$(shell_rc_file)"
  fi
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

assert_install_fails() {
  local release_dir="$1"
  local label="$2"

  if THR_INSTALL_TEST_BASE_URL="file://${release_dir}" THR_INSTALL_SKIP_SKILL_PROMPT=1 bash "$ROOT_DIR/install.sh" >/dev/null 2>&1; then
    fail "expected install to reject ${label}"
  fi
}

assert_signature_and_checksum_fail_closed() {
  local bad_sig_release="$WORK_DIR/bad-signature-release"
  local bad_archive_release="$WORK_DIR/bad-archive-release"
  local archive

  cp -R "$WORK_DIR/release" "$bad_sig_release"
  printf 'tampered\n' >>"$bad_sig_release/checksums.txt"
  assert_install_fails "$bad_sig_release" "tampered signed checksums"

  cp -R "$WORK_DIR/release" "$bad_archive_release"
  archive="$(find "$bad_archive_release" -name 'thr_*.tar.gz' -type f | head -n 1)"
  printf 'tampered\n' >>"$archive"
  assert_install_fails "$bad_archive_release" "tampered archive"
}

main() {
  local release_base_url install_dir

  prepare_home
  release_base_url="${THR_INSTALL_TEST_BASE_URL:-}"
  if [[ -z "$release_base_url" ]]; then
    create_release_fixture
    sign_release_fixture
    assert_signature_and_checksum_fail_closed
    release_base_url="file://$WORK_DIR/release"
    export THR_INSTALL_PREFIX="$WORK_DIR/prefix"
    install_dir="$THR_INSTALL_PREFIX/bin"
    export PATH="${install_dir}:$PATH"
    export THR_UNINSTALL_TEST_BIN_DIRS="$install_dir"
    export THR_SMOKE_REAL_BINARY=0
  else
    hide_homebrew_from_path
    export THR_INSTALL_PREFIX="$WORK_DIR/prefix"
    install_dir="$THR_INSTALL_PREFIX/bin"
    export PATH="${install_dir}:$PATH"
    unset THR_UNINSTALL_TEST_BIN_DIRS || true
    export THR_SMOKE_REAL_BINARY=1
  fi

  log 'Running install smoke test'
  THR_INSTALL_TEST_BASE_URL="$release_base_url" THR_INSTALL_SKIP_SKILL_PROMPT=1 bash "$ROOT_DIR/install.sh"

  [[ -x "$install_dir/thr" ]] || fail 'install did not place thr in the install bin dir'
  [[ -f "$THR_INSTALL_PREFIX/lib/thr/manifest.json" ]] || fail 'install did not place thr manifest in the lib dir'
  assert_path_block_present
  assert_thr_usable
  assert_agent_skill_prompt_skipped
  mkdir -p "$HOME/.thr/models"
  : >"$HOME/.thr/thr.db"
  : >"$HOME/.thr/models/model"

  log 'Running uninstall smoke test'
  bash "$ROOT_DIR/uninstall.sh"

  [[ ! -e "$install_dir/thr" ]] || fail 'uninstall left thr behind'
  [[ ! -e "$THR_INSTALL_PREFIX/lib/thr" ]] || fail 'uninstall left thr runtime files behind'
  [[ -e "$HOME/.thr/thr.db" ]] || fail 'uninstall removed data without confirmation'
  [[ -e "$HOME/.thr/models/model" ]] || fail 'uninstall removed model cache without confirmation'
  assert_path_block_removed
}

main "$@"
