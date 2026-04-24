#!/usr/bin/env bash
set -euo pipefail

REPO_SLUG="Chadi00/thr"
THR_PATH_MARKER="# thr install: add Homebrew bin to PATH (https://github.com/Chadi00/thr)"
THR_OLD_PATH_MARKER="# thr install: add thr bin dir to PATH (https://github.com/Chadi00/thr)"
THR_OLD_GO_PATH_MARKER="# thr install: add Go bin to PATH (https://github.com/Chadi00/thr)"
THR_DOWNLOAD_BASE_URL="${THR_INSTALL_TEST_BASE_URL:-https://github.com/${REPO_SLUG}/releases/latest/download}"

THR_TMPDIR=""
THR_INSTALLED_BIN=""
THR_UPDATED_SHELL_RC=0

log() {
  printf '[thr-install] %s\n' "$*"
}

warn() {
  printf '[thr-install] warning: %s\n' "$*" >&2
}

need_cmd() {
  command -v "$1" >/dev/null 2>&1
}

cleanup() {
  if [[ -n "$THR_TMPDIR" ]]; then
    rm -rf "$THR_TMPDIR"
  fi
}

trap cleanup EXIT

ensure_macos() {
  if [[ "$(uname -s)" == 'Darwin' ]]; then
    return 0
  fi

  warn "thr install currently supports macOS only."
  return 1
}

ensure_homebrew() {
  if need_cmd brew; then
    return 0
  fi

  warn "Homebrew is required. Install it from https://brew.sh and re-run this command."
  return 1
}

ensure_onnxruntime() {
  if brew list --versions onnxruntime >/dev/null 2>&1; then
    return 0
  fi

  log "Installing ONNX Runtime via Homebrew..."
  brew install onnxruntime
}

normalize_arch() {
  case "$(uname -m)" in
    arm64 | aarch64) printf '%s' 'arm64' ;;
    x86_64 | amd64) printf '%s' 'amd64' ;;
    *)
      warn "Unsupported macOS architecture: $(uname -m)"
      return 1
      ;;
  esac
}

sha256_file() {
  if need_cmd sha256sum; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

download_release_binary() {
  local arch archive expected actual

  if ! need_cmd curl || ! need_cmd tar; then
    warn "Install requires curl and tar."
    return 1
  fi

  arch="$(normalize_arch)" || return 1
  archive="thr_darwin_${arch}.tar.gz"
  THR_TMPDIR="$(mktemp -d "${TMPDIR:-/tmp}/thr-install.XXXXXX")"

  log "Downloading ${archive}..."
  curl -fsSL "${THR_DOWNLOAD_BASE_URL}/${archive}" -o "${THR_TMPDIR}/${archive}"
  curl -fsSL "${THR_DOWNLOAD_BASE_URL}/checksums.txt" -o "${THR_TMPDIR}/checksums.txt"

  expected="$(awk -v name="$archive" '$2 == name {print $1; exit}' "${THR_TMPDIR}/checksums.txt")"
  if [[ -z "$expected" ]]; then
    warn "Could not find checksum for ${archive}."
    return 1
  fi

  actual="$(sha256_file "${THR_TMPDIR}/${archive}")"
  if [[ "$actual" != "$expected" ]]; then
    warn "Checksum mismatch for ${archive}."
    return 1
  fi

  tar -xzf "${THR_TMPDIR}/${archive}" -C "$THR_TMPDIR"
  if [[ ! -f "${THR_TMPDIR}/thr" ]]; then
    warn "Archive did not contain a thr binary."
    return 1
  fi

  chmod +x "${THR_TMPDIR}/thr"
  THR_INSTALLED_BIN="${THR_TMPDIR}/thr"
}

shell_rc_file() {
  case "$(basename "${SHELL:-/bin/zsh}")" in
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

strip_thr_path_blocks() {
  local file="$1"
  local tmp

  [[ -f "$file" ]] || return 0
  tmp="$(mktemp "${TMPDIR:-/tmp}/thr-install.XXXXXX")"
  awk -v m1="$THR_PATH_MARKER" -v m2="$THR_OLD_PATH_MARKER" -v m3="$THR_OLD_GO_PATH_MARKER" '
    $0 == m1 || $0 == m2 || $0 == m3 { skip = 1; next }
    skip && /^export PATH=/ { skip = 0; next }
    skip { next }
    { print }
  ' "$file" >"$tmp"
  mv "$tmp" "$file"
}

ensure_dir_on_path() {
  local dir="$1"
  local rc

  if [[ ":$PATH:" == *":$dir:"* ]]; then
    return 0
  fi

  rc="$(shell_rc_file)"
  mkdir -p "$(dirname "$rc")"
  strip_thr_path_blocks "$rc"
  {
    printf '\n%s\n' "$THR_PATH_MARKER"
    printf 'export PATH="%s:$PATH"\n' "$dir"
  } >>"$rc"

  export PATH="$dir:$PATH"
  THR_UPDATED_SHELL_RC=1
  log "Added ${dir} to PATH in ${rc}"
}

install_dir() {
  printf '%s' "$(brew --prefix)/bin"
}

ensure_install_dir_exists() {
  local dir="$1"

  if [[ -d "$dir" ]]; then
    return 0
  fi

  if mkdir -p "$dir" 2>/dev/null; then
    return 0
  fi

  log "Creating ${dir} (you may be prompted for sudo)..."
  sudo mkdir -p "$dir"
}

install_binary() {
  local dir dst

  dir="$(install_dir)"
  dst="${dir}/thr"
  ensure_install_dir_exists "$dir"

  if [[ -w "$dir" ]]; then
    install -m 0755 "$THR_INSTALLED_BIN" "$dst"
  else
    log "Installing thr to ${dst} (you may be prompted for sudo)..."
    sudo install -m 0755 "$THR_INSTALLED_BIN" "$dst"
  fi

  THR_INSTALLED_BIN="$dst"
  ensure_dir_on_path "$dir"
  log "Installed thr to ${dst}"
}

prefetch_model() {
  log "Caching the embedding model..."
  if "$THR_INSTALLED_BIN" prefetch; then
    log "Embedding model is ready."
    return 0
  fi

  warn "Could not run thr prefetch. The model will download on the first add, ask, or edit."
  return 0
}

main() {
  ensure_macos
  ensure_homebrew
  download_release_binary
  ensure_onnxruntime
  install_binary
  prefetch_model

  log "Ready: ${THR_INSTALLED_BIN}"
  if [[ "$THR_UPDATED_SHELL_RC" -eq 1 ]]; then
    log "If thr is not found in new shells yet: source $(shell_rc_file)  (or open a new terminal)"
  fi
  log "Verify: thr --help"
}

main "$@"
