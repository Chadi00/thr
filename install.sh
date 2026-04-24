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

_go_bin_dir() {
  local gobin
  gobin="$(go env GOBIN)"
  if [ -z "$gobin" ]; then
    gobin="$(go env GOPATH)/bin"
  fi
  # go env may return a path with ~; expand for reliable comparisons and writes
  gobin="${gobin/#\~/$HOME}"
  printf '%s' "$gobin"
}

# Shell rc file to update so `thr` is on PATH in new terminals (idempotent).
_shell_rc_file() {
  case "$(basename "${SHELL:-/bin/sh}")" in
    zsh) printf '%s' "${ZDOTDIR:-$HOME}/.zshrc" ;;
    bash)
      if [[ -f "$HOME/.bashrc" ]]; then
        printf '%s' "$HOME/.bashrc"
      elif [[ -f "$HOME/.bash_profile" ]]; then
        printf '%s' "$HOME/.bash_profile"
      else
        printf '%s' "$HOME/.bashrc"
      fi
      ;;
    *) printf '%s' "$HOME/.profile" ;;
  esac
}

_THR_PATH_MARKER="# thr install: add Go bin to PATH (https://github.com/Chadi00/thr)"

# Copy built thr into a system-wide bin directory. Returns 0 on success.
_install_thr_to_one_dir() {
  local src=$1
  local dir=$2
  local dst="${dir}/thr"

  [[ -d "$dir" ]] || return 1
  if [[ -w "$dir" ]]; then
    if install -m 0755 "$src" "$dst"; then
      log "Installed thr to $dst"
      return 0
    fi
    return 1
  fi
  log "Installing thr to $dst (enter your macOS password if prompted)..."
  if sudo install -m 0755 "$src" "$dst"; then
    log "Installed thr to $dst"
    return 0
  fi
  return 1
}

# macOS: place the built binary where normal shells already look (no PATH edits, no
# "source" step). Tries: Homebrew's bin, then /opt/homebrew/bin, /usr/local/bin, using
# sudo only when the directory is not user-writable.
install_thr_to_system_path_macos() {
  local src
  local dir

  src="$(_go_bin_dir)/thr"
  if [[ ! -f "$src" ]]; then
    warn "go install did not produce: $src"
    return 1
  fi

  if need_cmd brew; then
    if _install_thr_to_one_dir "$src" "$(brew --prefix)/bin"; then
      return 0
    fi
  fi

  for dir in /opt/homebrew/bin /usr/local/bin; do
    if _install_thr_to_one_dir "$src" "$dir"; then
      return 0
    fi
  done

  return 1
}

# `curl | bash` uses a non-login bash with a minimal PATH. Prepend the usual Mac CLI
# locations so a post-install `command -v thr` matches what zsh/Terminal users get.
_prepend_default_macos_path() {
  local d
  for d in /opt/homebrew/bin /usr/local/bin; do
    if [[ -d "$d" ]]; then
      export PATH="$d:$PATH"
    fi
  done
  if need_cmd brew; then
    d="$(brew --prefix)/bin"
    if [[ -d "$d" ]]; then
      export PATH="$d:$PATH"
    fi
  fi
}

ensure_gobin_in_path() {
  local gobin
  gobin="$(_go_bin_dir)"

  if [[ ":$PATH:" == *":$gobin:"* ]]; then
    log "Go bin is already on PATH ($gobin)."
    return 0
  fi

  local rc
  rc="$(_shell_rc_file)"

  if [[ -f "$rc" ]] && grep -qF "thr install: add Go bin" "$rc" 2>/dev/null; then
    log "A thr PATH entry exists in $rc; open a new terminal or: source $rc"
    return 0
  fi

  {
    printf '\n%s\n' "$_THR_PATH_MARKER"
    printf 'export PATH="%s:$PATH"\n' "$gobin"
  } >>"$rc"

  log "Added $gobin to PATH in $rc"
  log "If thr is not found in this window: source $rc   (or open a new terminal)"
}

# `go install` and PATH updates apply to the install *process* only. The shell you type in
# after `curl | bash` is a parent process and will not see PATH until you `source` the rc
# (or start a new terminal). This script exports PATH for the install process and, below,
# runs the same `source` in a *subshell* to verify the rc. That does not replace sourcing
# in your interactive session.
apply_gobin_to_path_in_this_process() {
  local gobin
  gobin="$(_go_bin_dir)"
  export PATH="$gobin:$PATH"
}

# Run the same `source` for the rc file we just updated (e.g. ~/.zshrc), in a matching
# shell, so the installer actually executes a source step (separate from your login shell).
# After install, load the BGE embedding model so the first add/ask is not slow.
prefetch_embedding_model() {
  local gothr
  gothr="$(_go_bin_dir)/thr"

  if command -v thr >/dev/null 2>&1; then
    thr prefetch
  elif [[ -x "$gothr" ]]; then
    log "Using $gothr for prefetch (add Go bin to PATH to run thr from anywhere)..."
    "$gothr" prefetch
  else
    return 1
  fi
}

source_shell_rc_in_subshell() {
  local rc qrc
  local ok=0

  rc="$(_shell_rc_file)"
  if [[ ! -f "$rc" ]]; then
    return 0
  fi
  qrc=$(printf '%q' "$rc")

  case "$(basename "${SHELL:-/bin/sh}")" in
    zsh)
      if command -v zsh >/dev/null 2>&1 && zsh -c "set -e; source $qrc >/dev/null 2>&1; command -v thr" 2>/dev/null; then
        ok=1
      fi
      ;;
    bash)
      if command -v bash >/dev/null 2>&1 && bash -c "set -e; source $qrc >/dev/null 2>&1; command -v thr" 2>/dev/null; then
        ok=1
      fi
      ;;
    *)
      if sh -c "set -e; . $qrc >/dev/null 2>&1; command -v thr" 2>/dev/null; then
        ok=1
      fi
      ;;
  esac

  if [[ "$ok" -eq 1 ]]; then
    log "Sourced $rc in a child shell; in *this* terminal you still need: source $rc  (or a new tab) before thr is found"
  else
    warn "Could not confirm \`source $rc\` + thr; check $rc and PATH under $(_go_bin_dir)"
  fi
}

main() {
  local os
  local mac_system=0
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

  if [[ "$os" == "Darwin" ]]; then
    if install_thr_to_system_path_macos; then
      mac_system=1
    else
      log "Falling back: adding Go bin to your shell config (or use: export PATH=\"$(_go_bin_dir):\$PATH\")."
      ensure_gobin_in_path
    fi
  else
    ensure_gobin_in_path
  fi

  if [[ "$os" == "Darwin" ]]; then
    _prepend_default_macos_path
  fi
  if [[ "$mac_system" -eq 0 ]]; then
    apply_gobin_to_path_in_this_process
  fi
  if [[ "$mac_system" -eq 0 ]]; then
    source_shell_rc_in_subshell
  fi

  if command -v thr >/dev/null 2>&1; then
    log "Ready: $(command -v thr)"
  else
    warn "thr is not on PATH in this install session; on macOS, try opening a new terminal, or: export PATH=\"$(_go_bin_dir):\$PATH\""
  fi

  log "Ensuring the embedding model is in cache (first install may take a minute)..."
  if prefetch_embedding_model; then
    log "Embedding model is available."
  else
    warn "Could not run thr prefetch. The model will download on the first add, ask, or edit."
  fi

  if [[ "$mac_system" -eq 1 ]]; then
    log "On macOS, thr is on your default PATH. Run: thr --help   (re-run this installer anytime to update)"
  else
    log "If thr is not found in this window: source $(_shell_rc_file)  (or open a new tab)"
    log "Re-run this same command anytime to update to the latest thr version. Verify: thr --help"
  fi
}

main "$@"
