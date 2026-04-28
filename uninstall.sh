#!/usr/bin/env bash
set -euo pipefail

THR_PATH_MARKER="# thr install: add thr bin to PATH (https://github.com/Chadi00/thr)"
THR_LEGACY_HOMEBREW_PATH_MARKER="# thr install: add Homebrew bin to PATH (https://github.com/Chadi00/thr)"
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

confirm() {
  local prompt="$1"
  local reply

  if ! { exec 3<>/dev/tty; } 2>/dev/null; then
    return 1
  fi

  printf '%s [y/N] ' "$prompt" >&3
  IFS= read -r reply <&3 || {
    exec 3>&-
    return 1
  }
  exec 3>&-
  case "$reply" in
    y | Y | yes | YES | Yes) return 0 ;;
    *) return 1 ;;
  esac
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

  candidates+=("${THR_INSTALL_PREFIX:-$HOME/.local}/bin" /opt/homebrew/bin /usr/local/bin)

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
  awk -v m1="$THR_PATH_MARKER" -v m2="$THR_LEGACY_HOMEBREW_PATH_MARKER" -v m3="$THR_OLD_PATH_MARKER" -v m4="$THR_OLD_GO_PATH_MARKER" '
    $0 == m1 || $0 == m2 || $0 == m3 || $0 == m4 { skip = 1; next }
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
    if grep -qF "$THR_PATH_MARKER" "$file" 2>/dev/null || grep -qF "$THR_LEGACY_HOMEBREW_PATH_MARKER" "$file" 2>/dev/null || grep -qF "$THR_OLD_PATH_MARKER" "$file" 2>/dev/null || grep -qF "$THR_OLD_GO_PATH_MARKER" "$file" 2>/dev/null; then
      strip_path_blocks_from_file "$file"
    fi
  done
}

remove_runtime_files() {
  local dir="${THR_INSTALL_PREFIX:-$HOME/.local}/lib/thr"

  [[ -e "$dir" ]] || return 0
  if rm -rf "$dir" 2>/dev/null; then
    log "Removed ${dir}"
    return 0
  fi
  if sudo rm -rf "$dir" 2>/dev/null; then
    log "Removed ${dir} (needed sudo)"
    return 0
  fi
  warn "Could not remove ${dir}"
}

remove_memory_store() {
  local dir="$HOME/.thr"
  local removed=0

  [[ -e "$dir" ]] || return 0
  if ! confirm "Remove saved memories at ${dir}?"; then
    log "Preserved saved memories at ${dir}"
    return 0
  fi

  for path in "$dir/thr.db" "$dir/thr.db-wal" "$dir/thr.db-shm"; do
    if [[ -e "$path" ]]; then
      rm -f "$path"
      removed=1
    fi
  done

  if [[ "$removed" -eq 1 ]]; then
    log "Removed saved memories from ${dir}"
  else
    log "No saved memories found at ${dir}"
  fi
}

remove_model_cache() {
  local dir="$HOME/.thr/models"

  [[ -e "$dir" ]] || return 0
  if ! confirm "Remove cached embedding model at ${dir}?"; then
    log "Preserved cached embedding model at ${dir}"
    return 0
  fi

  rm -rf "$dir"
  log "Removed cached embedding model at ${dir}"
}

remove_empty_thr_home() {
  local dir="$HOME/.thr"

  [[ -d "$dir" ]] || return 0
  rmdir "$dir" 2>/dev/null || return 0
  log "Removed empty ${dir}"
}

main() {
  ensure_macos
  log "Removing thr binaries..."
  remove_thr_binaries
  remove_runtime_files

  log "Removing install PATH snippets (if any)..."
  remove_install_path_lines

  remove_memory_store
  remove_model_cache
  remove_empty_thr_home
  log "Done."
}

main "$@"
