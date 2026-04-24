#!/usr/bin/env bash
set -euo pipefail

THR_PATH_MARKER="# thr install: add Homebrew bin to PATH (https://github.com/Chadi00/thr)"
THR_OLD_PATH_MARKER="# thr install: add thr bin dir to PATH (https://github.com/Chadi00/thr)"
THR_OLD_GO_PATH_MARKER="# thr install: add Go bin to PATH (https://github.com/Chadi00/thr)"

log() {
  printf '[thr-uninstall] %s\n' "$*"
}

warn() {
  printf '[thr-uninstall] warning: %s\n' "$*" >&2
}

need_cmd() {
  command -v "$1" >/dev/null 2>&1
}

ensure_macos() {
  if [[ "$(uname -s)" == 'Darwin' ]]; then
    return 0
  fi

  warn "thr uninstall currently supports macOS only."
  return 1
}

try_rm_thr() {
  local path="$1"

  [[ -e "$path" ]] || return 0
  if rm -f "$path" 2>/dev/null; then
    log "Removed ${path}"
    return 0
  fi
  if sudo rm -f "$path" 2>/dev/null; then
    log "Removed ${path} (needed sudo)"
    return 0
  fi

  warn "Could not remove ${path}"
}

remove_thr_binaries() {
  local dir
  local candidates=()
  local seen=""

  if [[ -n "${THR_UNINSTALL_TEST_BIN_DIRS:-}" ]]; then
    for dir in ${THR_UNINSTALL_TEST_BIN_DIRS}; do
      try_rm_thr "${dir}/thr"
    done
    return 0
  fi

  if need_cmd brew; then
    candidates+=("$(brew --prefix)/bin")
  fi
  candidates+=(/opt/homebrew/bin /usr/local/bin)

  for dir in "${candidates[@]}"; do
    case " ${seen} " in
      *" ${dir} "*) continue ;;
    esac
    seen="${seen} ${dir}"
    try_rm_thr "${dir}/thr"
  done
}

strip_path_blocks_from_file() {
  local file="$1"
  local tmp

  [[ -f "$file" ]] || return 0
  tmp="$(mktemp "${TMPDIR:-/tmp}/thr-uninstall.XXXXXX")"
  awk -v m1="$THR_PATH_MARKER" -v m2="$THR_OLD_PATH_MARKER" -v m3="$THR_OLD_GO_PATH_MARKER" '
    $0 == m1 || $0 == m2 || $0 == m3 { skip = 1; next }
    skip && /^export PATH=/ { skip = 0; next }
    skip { next }
    { print }
  ' "$file" >"$tmp"
  mv "$tmp" "$file"
  log "Removed thr PATH block from ${file}"
}

remove_install_path_lines() {
  local file

  for file in "${ZDOTDIR:-$HOME}/.zshrc" "$HOME/.zshrc" "$HOME/.bashrc" "$HOME/.bash_profile" "$HOME/.profile"; do
    [[ -f "$file" ]] || continue
    if grep -qF "$THR_PATH_MARKER" "$file" 2>/dev/null || grep -qF "$THR_OLD_PATH_MARKER" "$file" 2>/dev/null || grep -qF "$THR_OLD_GO_PATH_MARKER" "$file" 2>/dev/null; then
      strip_path_blocks_from_file "$file"
    fi
  done
}

remove_data_dir() {
  local dir="$HOME/.thr"

  [[ -e "$dir" ]] || return 0
  rm -rf "$dir"
  log "Removed ${dir}"
}

main() {
  ensure_macos
  log "Removing thr binaries..."
  remove_thr_binaries

  log "Removing install PATH snippets (if any)..."
  remove_install_path_lines

  remove_data_dir
  log "Done."
  if need_cmd brew && brew list --versions onnxruntime >/dev/null 2>&1; then
    log "Homebrew onnxruntime was left installed. Remove it manually if you no longer need it: brew uninstall onnxruntime"
  fi
}

main "$@"
