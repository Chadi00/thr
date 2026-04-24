#!/usr/bin/env bash
set -euo pipefail

# Removes the thr binary, default data (~/.thr), and PATH lines added by install.sh.
# Optional: THR_KEEP_DATA=1 keeps ~/.thr; THR_UNINSTALL_ONNX=1 runs brew uninstall onnxruntime.

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
  if [ -z "$gobin" ]; then
    gobin="$(go env GOPATH)/bin"
  fi
  gobin="${gobin/#\~/$HOME}"
  printf '%s' "$gobin"
}

_try_rm_thr() {
  local p="$1"
  [[ -e "$p" ]] || return 0
  if [[ -f "$p" ]] || [[ -L "$p" ]]; then
    if rm -f "$p" 2>/dev/null; then
      log "Removed $p"
      return 0
    fi
    if sudo rm -f "$p" 2>/dev/null; then
      log "Removed $p (needed sudo)"
      return 0
    fi
    warn "Could not remove $p"
  fi
  return 0
}

_thr_path_marker() {
  printf '%s' "# thr install: add Go bin to PATH (https://github.com/Chadi00/thr)"
}

_remove_thr_binaries() {
  local d gobin

  for d in /opt/homebrew/bin /usr/local/bin; do
    _try_rm_thr "$d/thr"
  done

  if need_cmd brew; then
    _try_rm_thr "$(brew --prefix)/bin/thr"
  fi

  _try_rm_thr "${THR_USER_BIN:-$HOME/.local/bin}/thr"

  if gobin="$(_go_bin_dir 2>/dev/null)"; then
    _try_rm_thr "$gobin/thr"
  fi
}

_strip_path_marker_from_file() {
  local f="$1"
  local marker="$2"
  [[ -f "$f" ]] || return 0
  grep -qF "$marker" "$f" 2>/dev/null || return 0

  local tmp
  tmp="$(mktemp "${TMPDIR:-/tmp}/thr-uninstall.XXXXXX")"
  awk -v m="$marker" '
    $0 == m { skip = 1; next }
    skip && /^export PATH=/ { skip = 0; next }
    skip { next }
    { print }
  ' "$f" >"$tmp"
  mv "$tmp" "$f"
  log "Removed thr PATH block from $f"
}

_remove_install_path_lines() {
  # install.sh may have appended to any of these; scan all that contain the marker.
  local f marker="$(_thr_path_marker)"
  for f in "${ZDOTDIR:-$HOME}/.zshrc" "$HOME/.zshrc" "$HOME/.bashrc" "$HOME/.bash_profile" "$HOME/.profile"; do
    [[ -f "$f" ]] || continue
    _strip_path_marker_from_file "$f" "$marker"
  done
}

_remove_data_dir() {
  if [[ "${THR_KEEP_DATA:-}" == "1" ]]; then
    log "Keeping ~/.thr (THR_KEEP_DATA=1)"
    return 0
  fi
  local d="$HOME/.thr"
  if [[ ! -e "$d" ]]; then
    return 0
  fi
  rm -rf "$d"
  log "Removed $d (database and embedding cache)"
}

_maybe_uninstall_onnx_brew() {
  if [[ "${THR_UNINSTALL_ONNX:-}" != "1" ]]; then
    return 0
  fi
  if ! need_cmd brew; then
    warn "THR_UNINSTALL_ONNX=1 but brew not found; skipping onnxruntime"
    return 0
  fi
  if ! brew list --versions onnxruntime >/dev/null 2>&1; then
    log "onnxruntime not installed via Homebrew; nothing to remove"
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
  if [[ "${THR_UNINSTALL_ONNX:-}" != "1" ]]; then
    log "Homebrew onnxruntime was left installed (other tools may use it). To remove: brew uninstall onnxruntime"
  fi
}

main "$@"
