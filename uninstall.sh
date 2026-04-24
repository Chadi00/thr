#!/usr/bin/env bash
set -euo pipefail

# Removes the thr binary, default data (~/.thr), and PATH lines added by install.sh.
# Optional: THR_KEEP_DATA=1 keeps ~/.thr; THR_UNINSTALL_ONNX=1 runs brew uninstall onnxruntime.

THR_PATH_MARKER="# thr install: add thr bin dir to PATH (https://github.com/Chadi00/thr)"
THR_OLD_PATH_MARKER="# thr install: add Go bin to PATH (https://github.com/Chadi00/thr)"

log() {
  printf '[thr-uninstall] %s\n' "$*"
}

warn() {
  printf '[thr-uninstall] warning: %s\n' "$*" >&2
}

need_cmd() {
  command -v "$1" >/dev/null 2>&1
}

_go_bin_dir() {
  local gobin
  if ! need_cmd go; then
    return 1
  fi
  gobin="$(go env GOBIN)"
  if [[ -z "$gobin" ]]; then
    gobin="$(go env GOPATH)/bin"
  fi
  gobin="${gobin/#\~/$HOME}"
  printf '%s' "$gobin"
}

_try_rm_thr() {
  local path="$1"
  [[ -e "$path" ]] || return 0

  if rm -f "$path" 2>/dev/null; then
    log "Removed $path"
    return 0
  fi
  if sudo rm -f "$path" 2>/dev/null; then
    log "Removed $path (needed sudo)"
    return 0
  fi

  warn "Could not remove $path"
}

_remove_thr_binaries() {
  local dir gobin

  if [[ -n "${THR_INSTALL_DIR:-}" ]]; then
    _try_rm_thr "${THR_INSTALL_DIR}/thr"
    return 0
  fi

  for dir in /opt/homebrew/bin /usr/local/bin "$HOME/.local/bin" "${THR_USER_BIN:-$HOME/.local/bin}"; do
    _try_rm_thr "$dir/thr"
  done

  if need_cmd brew; then
    _try_rm_thr "$(brew --prefix)/bin/thr"
  fi

  if gobin="$(_go_bin_dir 2>/dev/null)"; then
    _try_rm_thr "$gobin/thr"
  fi
}

_strip_path_blocks_from_file() {
  local file="$1"
  local tmp

  [[ -f "$file" ]] || return 0
  tmp="$(mktemp "${TMPDIR:-/tmp}/thr-uninstall.XXXXXX")"
  awk -v m1="$THR_PATH_MARKER" -v m2="$THR_OLD_PATH_MARKER" '
    $0 == m1 || $0 == m2 { skip = 1; next }
    skip && /^export PATH=/ { skip = 0; next }
    skip { next }
    { print }
  ' "$file" >"$tmp"
  mv "$tmp" "$file"
  log "Removed thr PATH block from $file"
}

_remove_install_path_lines() {
  local file
  for file in "${ZDOTDIR:-$HOME}/.zshrc" "$HOME/.zshrc" "$HOME/.bashrc" "$HOME/.bash_profile" "$HOME/.profile"; do
    [[ -f "$file" ]] || continue
    if grep -qF "$THR_PATH_MARKER" "$file" 2>/dev/null || grep -qF "$THR_OLD_PATH_MARKER" "$file" 2>/dev/null; then
      _strip_path_blocks_from_file "$file"
    fi
  done
}

_remove_data_dir() {
  local dir="$HOME/.thr"

  if [[ "${THR_KEEP_DATA:-}" == '1' ]]; then
    log "Keeping ~/.thr (THR_KEEP_DATA=1)"
    return 0
  fi
  [[ -e "$dir" ]] || return 0

  rm -rf "$dir"
  log "Removed $dir (database and embedding cache)"
}

_maybe_uninstall_onnx_brew() {
  if [[ "${THR_UNINSTALL_ONNX:-}" != '1' ]]; then
    return 0
  fi
  if ! need_cmd brew; then
    warn "THR_UNINSTALL_ONNX=1 but brew was not found; skipping onnxruntime"
    return 0
  fi
  if ! brew list --versions onnxruntime >/dev/null 2>&1; then
    log "onnxruntime is not installed via Homebrew; nothing to remove"
    return 0
  fi

  log "Uninstalling Homebrew formula onnxruntime..."
  brew uninstall onnxruntime
}

main() {
  log "Removing thr binaries..."
  _remove_thr_binaries

  log "Removing install PATH snippets (if any)..."
  _remove_install_path_lines

  _remove_data_dir
  _maybe_uninstall_onnx_brew

  log "Done."
  if [[ "${THR_UNINSTALL_ONNX:-}" != '1' ]]; then
    log "Homebrew onnxruntime was left installed (other tools may use it). To remove: brew uninstall onnxruntime"
  fi
}

main "$@"
