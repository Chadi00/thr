#!/usr/bin/env bash
set -euo pipefail

REPO_MODULE="github.com/Chadi00/thr/cmd/thr"
GO_TAGS="sqlite_fts5"

log() {
  printf '[thr-install] %s\n' "$*"
}

warn() {
  printf '[thr-install] warning: %s\n' "$*" >&2
}

need_cmd() {
  command -v "$1" >/dev/null 2>&1
}

ensure_homebrew() {
  if need_cmd brew; then
    return 0
  fi

  warn "Homebrew is required to auto-install dependencies on macOS."
  warn "Install Homebrew from https://brew.sh and re-run this command."
  return 1
}

ensure_go_macos() {
  if need_cmd go; then
    return 0
  fi

  ensure_homebrew || return 1
  log "Installing Go via Homebrew..."
  brew install go
}

ensure_go_linux() {
  if need_cmd go; then
    return 0
  fi

  if need_cmd apt-get; then
    log "Installing Go via apt..."
    sudo apt-get update
    sudo apt-get install -y golang-go
    return 0
  fi

  warn "Go is not installed and no supported package manager was detected."
  warn "Install Go 1.22+ manually and re-run this command."
  return 1
}

ensure_build_tools_macos() {
  if xcode-select -p >/dev/null 2>&1; then
    return 0
  fi

  warn "Xcode Command Line Tools are required for CGO builds."
  warn "Run: xcode-select --install"
  return 1
}

ensure_build_tools_linux() {
  if need_cmd gcc; then
    return 0
  fi

  if need_cmd apt-get; then
    log "Installing build-essential for CGO..."
    sudo apt-get update
    sudo apt-get install -y build-essential pkg-config
    return 0
  fi

  warn "gcc/build tools are required for CGO builds."
  return 1
}

ensure_onnx_macos() {
  ensure_homebrew || return 1
  if brew list --versions onnxruntime >/dev/null 2>&1; then
    return 0
  fi

  log "Installing ONNX Runtime via Homebrew..."
  brew install onnxruntime
}

ensure_onnx_linux() {
  if ldconfig -p 2>/dev/null | grep -q "libonnxruntime"; then
    return 0
  fi

  if need_cmd apt-get; then
    log "Attempting to install ONNX Runtime via apt..."
    sudo apt-get update
    if sudo apt-get install -y libonnxruntime-dev; then
      return 0
    fi
  fi

  warn "ONNX Runtime was not auto-installed."
  warn "Install libonnxruntime manually and set ONNX_PATH if needed."
}

# Spinner frames match briandowns/spinner CharSets[11] (braille).
_spinner_frames=(⣾ ⣽ ⣻ ⢿ ⡿ ⣟ ⣯ ⣷)

install_thr() {
  log "Installing/updating thr via go install..."
  log "Target module: $REPO_MODULE@latest"

  local spin_pid
  (
    i=0
    n=${#_spinner_frames[@]}
    while true; do
      printf '\r\033[K[thr-install] %s still working (go install)...' "${_spinner_frames[i]}" >&2
      i=$(( (i + 1) % n ))
      sleep 0.1
    done
  ) &
  spin_pid=$!

  _stop_install_spinner() {
    kill "$spin_pid" 2>/dev/null || true
    wait "$spin_pid" 2>/dev/null || true
    printf '\r\033[K' >&2
  }

  # Restore default INT after cleanup so a second Ctrl-C can force-quit.
  trap '_stop_install_spinner; trap - INT; kill -INT $$' INT

  local ec=0
  set +e
  CGO_ENABLED=1 go install -tags "$GO_TAGS" "$REPO_MODULE@latest"
  ec=$?
  set -e

  trap - INT
  _stop_install_spinner
  return "$ec"
}

print_path_hint() {
  local gobin
  gobin="$(go env GOBIN)"
  if [ -z "$gobin" ]; then
    gobin="$(go env GOPATH)/bin"
  fi

  if [[ ":$PATH:" != *":$gobin:"* ]]; then
    warn "Add $gobin to your PATH to run thr from anywhere."
    warn "Example: export PATH=\"$gobin:\$PATH\""
  fi
}

main() {
  local os
  os="$(uname -s)"

  case "$os" in
    Darwin)
      ensure_go_macos
      ensure_build_tools_macos
      ensure_onnx_macos
      ;;
    Linux)
      ensure_go_linux
      ensure_build_tools_linux
      ensure_onnx_linux
      ;;
    *)
      warn "Unsupported OS: $os"
      warn "Install dependencies manually, then run: go install -tags \"$GO_TAGS\" $REPO_MODULE@latest"
      exit 1
      ;;
  esac

  install_thr
  print_path_hint

  log "Done. Re-run this same command anytime to update to the latest thr version."
  log "Verify with: thr --help"
}

main "$@"
