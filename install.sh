#!/usr/bin/env bash
set -euo pipefail

# Default: install a prebuilt binary from GitHub Releases (same pattern as widely used Go CLIs).
# Fallback: build from source (needs Go + CGO toolchain). Force source with THR_USE_SOURCE=1.
REPO_SLUG="Chadi00/thr"
REPO_MODULE="github.com/Chadi00/thr/cmd/thr"
# Release tag (v1.2.3) or "latest" for the newest GitHub release.
THR_VERSION="${THR_VERSION:-latest}"
# Source-build ref when falling back: branch, tag, or commit SHA (default: master).
INSTALL_REF="${THR_INSTALL_REF:-master}"
GO_TAGS="sqlite_fts5"

# Set after a successful install (absolute path to thr binary).
THR_INSTALLED_BIN=""
# Temp dir for release download (removed after copying thr to its final location).
THR_RELEASE_TMPDIR=""

log() {
  printf '[thr-install] %s\n' "$*"
}

warn() {
  printf '[thr-install] warning: %s\n' "$*" >&2
}

need_cmd() {
  command -v "$1" >/dev/null 2>&1
}

_thr_rm_tmp_dir() {
  [[ -n "${1:-}" ]] && rm -rf "$1"
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

_thr_validate_install_ref() {
  local ref="$1"
  case "$ref" in
    *..*)
      warn "Invalid THR_INSTALL_REF (contains '..'): $ref"
      return 1
      ;;
  esac
  if [[ "$ref" == *[![:print:]]* ]]; then
    warn "Invalid THR_INSTALL_REF (non-printable characters): $ref"
    return 1
  fi
  return 0
}

_thr_validate_release_version() {
  case "$1" in
    latest) return 0 ;;
    v*)
      return 0
      ;;
    *)
      warn "Invalid THR_VERSION (use latest or a tag like v0.1.2): $1"
      return 1
      ;;
  esac
}

# GitHub serves tarballs for branches, tags, and commits; content matches the web UI.
_thr_archive_url() {
  local ref="$1"

  if [[ "$ref" == v* ]] && [[ "$ref" =~ ^v[0-9] ]]; then
    printf 'https://github.com/%s/archive/refs/tags/%s.tar.gz' "$REPO_SLUG" "$ref"
    return 0
  fi

  if [[ "$ref" =~ ^[0-9a-f]{7,40}$ ]]; then
    printf 'https://github.com/%s/archive/%s.tar.gz' "$REPO_SLUG" "$ref"
    return 0
  fi

  printf 'https://github.com/%s/archive/refs/heads/%s.tar.gz' "$REPO_SLUG" "$ref"
}

_thr_goos_goarch() {
  local os_raw arch_raw
  os_raw="$(uname -s)"
  arch_raw="$(uname -m)"
  case "$os_raw" in
    Darwin) printf '%s %s' "darwin" "$(_thr_normalize_arch "$arch_raw")" ;;
    Linux) printf '%s %s' "linux" "$(_thr_normalize_arch "$arch_raw")" ;;
    *) return 1 ;;
  esac
}

_thr_normalize_arch() {
  case "$1" in
    x86_64 | amd64) printf '%s' "amd64" ;;
    arm64 | aarch64) printf '%s' "arm64" ;;
    *) return 1 ;;
  esac
}

_thr_sha256_file() {
  if need_cmd sha256sum; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

_thr_release_api_url() {
  local version="$1"
  if [[ "$version" == "latest" ]]; then
    printf 'https://api.github.com/repos/%s/releases/latest' "$REPO_SLUG"
  else
    printf 'https://api.github.com/repos/%s/releases/tags/%s' "$REPO_SLUG" "$version"
  fi
}

_thr_curl_github_json() {
  local url="$1"
  if [[ -n "${GITHUB_TOKEN:-}" ]]; then
    curl -fsSL -H "Authorization: Bearer ${GITHUB_TOKEN}" -H "Accept: application/vnd.github+json" "$url"
  else
    curl -fsSL -H "Accept: application/vnd.github+json" "$url"
  fi
}

_thr_json_asset_url() {
  local archive_name="$1"
  python3 -c "
import json, sys
name = sys.argv[1]
data = json.load(sys.stdin)
for a in data.get('assets', []):
    if a.get('name') == name:
        print(a.get('browser_download_url', ''))
        break
" "$archive_name"
}

install_thr_from_github_release() {
  local goos goarch triple archive checksums_url asset_url tmpdir expected actual

  _thr_validate_release_version "$THR_VERSION" || return 1

  if ! need_cmd curl || ! need_cmd tar || ! need_cmd python3; then
    warn "Binary install needs curl, tar, and python3."
    return 1
  fi

  triple="$(_thr_goos_goarch)" || {
    warn "Unsupported platform for prebuilt thr: $(uname -s) $(uname -m)"
    return 1
  }
  goos="${triple%% *}"
  goarch="${triple##* }"
  archive="thr_${goos}_${goarch}.tar.gz"

  local json
  if ! json="$(_thr_curl_github_json "$(_thr_release_api_url "$THR_VERSION")")"; then
    warn "No GitHub release found for THR_VERSION=$THR_VERSION (publish a tag like v0.1.2 first)."
    return 1
  fi

  asset_url="$(printf '%s' "$json" | _thr_json_asset_url "$archive")"
  checksums_url="$(printf '%s' "$json" | _thr_json_asset_url "checksums.txt")"
  if [[ -z "$asset_url" ]] || [[ -z "$checksums_url" ]]; then
    warn "Release is missing $archive or checksums.txt."
    return 1
  fi

  tmpdir="$(mktemp -d "${TMPDIR:-/tmp}/thr-install.XXXXXX")"

  log "Downloading $archive from GitHub Releases (THR_VERSION=$THR_VERSION)..."
  if ! curl -fsSL "$asset_url" -o "$tmpdir/$archive"; then
    _thr_rm_tmp_dir "$tmpdir"
    return 1
  fi
  if ! curl -fsSL "$checksums_url" -o "$tmpdir/checksums.txt"; then
    _thr_rm_tmp_dir "$tmpdir"
    return 1
  fi

  expected="$(grep -F "$archive" "$tmpdir/checksums.txt" | head -n 1 | awk '{print $1}')"
  if [[ -z "$expected" ]]; then
    warn "Could not find checksum line for $archive."
    _thr_rm_tmp_dir "$tmpdir"
    return 1
  fi

  actual="$(_thr_sha256_file "$tmpdir/$archive")"
  if [[ "$actual" != "$expected" ]]; then
    warn "Checksum mismatch for $archive (expected $expected, got $actual)."
    _thr_rm_tmp_dir "$tmpdir"
    return 1
  fi

  if ! tar -xzf "$tmpdir/$archive" -C "$tmpdir"; then
    _thr_rm_tmp_dir "$tmpdir"
    return 1
  fi
  if [[ ! -f "$tmpdir/thr" ]]; then
    warn "Archive did not contain thr binary."
    _thr_rm_tmp_dir "$tmpdir"
    return 1
  fi
  if [[ ! -x "$tmpdir/thr" ]]; then
    chmod +x "$tmpdir/thr"
  fi
  THR_INSTALLED_BIN="$tmpdir/thr"
  THR_RELEASE_TMPDIR="$tmpdir"
  return 0
}

_go_bin_dir() {
  local gobin
  gobin="$(go env GOBIN)"
  if [ -z "$gobin" ]; then
    gobin="$(go env GOPATH)/bin"
  fi
  gobin="${gobin/#\~/$HOME}"
  printf '%s' "$gobin"
}

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

ensure_dir_on_path() {
  local dir="$1"
  local marker="$2"

  if [[ ":$PATH:" == *":$dir:"* ]]; then
    log "Directory already on PATH ($dir)."
    return 0
  fi

  local rc
  rc="$(_shell_rc_file)"

  if [[ -f "$rc" ]] && grep -qF "$marker" "$rc" 2>/dev/null; then
    log "A thr PATH entry exists in $rc; open a new terminal or: source $rc"
    return 0
  fi

  {
    printf '\n%s\n' "$marker"
    printf 'export PATH="%s:$PATH"\n' "$dir"
  } >>"$rc"

  log "Added $dir to PATH in $rc"
  log "If thr is not found in this window: source $rc   (or open a new terminal)"
}

_install_thr_to_one_dir() {
  local src=$1
  local dir=$2
  local dst="${dir}/thr"

  [[ -d "$dir" ]] || return 1
  if [[ -w "$dir" ]]; then
    if install -m 0755 "$src" "$dst"; then
      log "Installed thr to $dst"
      THR_INSTALLED_BIN="$dst"
      return 0
    fi
    return 1
  fi
  log "Installing thr to $dst (enter your macOS password if prompted)..."
  if sudo install -m 0755 "$src" "$dst"; then
    log "Installed thr to $dst"
    THR_INSTALLED_BIN="$dst"
    return 0
  fi
  return 1
}

install_thr_to_system_path_macos() {
  local src="${1:-$(_go_bin_dir)/thr}"
  local dir

  if [[ ! -f "$src" ]]; then
    warn "thr binary not found: $src"
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

install_thr_user_local_linux() {
  local src="$1"
  local destdir="${THR_USER_BIN:-$HOME/.local/bin}"
  mkdir -p "$destdir"
  if install -m 0755 "$src" "$destdir/thr"; then
    log "Installed thr to $destdir/thr"
    THR_INSTALLED_BIN="$destdir/thr"
    ensure_dir_on_path "$destdir" "$_THR_PATH_MARKER"
    return 0
  fi
  return 1
}

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
  ensure_dir_on_path "$(_go_bin_dir)" "$_THR_PATH_MARKER"
}

apply_gobin_to_path_in_this_process() {
  export PATH="$(_go_bin_dir):$PATH"
}

apply_local_bin_to_path_in_this_process() {
  export PATH="${THR_USER_BIN:-$HOME/.local/bin}:$PATH"
}

prefetch_embedding_model() {
  local gothr
  gothr="${THR_INSTALLED_BIN:-}"

  if [[ -z "$gothr" ]] && command -v thr >/dev/null 2>&1; then
    gothr="$(command -v thr)"
  fi
  if [[ -z "$gothr" ]]; then
    gothr="$(_go_bin_dir)/thr"
  fi

  if command -v thr >/dev/null 2>&1; then
    thr prefetch
  elif [[ -x "$gothr" ]]; then
    log "Using $gothr for prefetch (add that directory to PATH to run thr from anywhere)..."
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
    warn "Could not confirm \`source $rc\` + thr; check $rc and PATH"
  fi
}

install_thr_from_source() {
  _thr_validate_install_ref "$INSTALL_REF" || return 1

  local archive_url
  archive_url="$(_thr_archive_url "$INSTALL_REF")"

  log "Building thr from GitHub source..."
  log "Archive: $archive_url"

  local tmpdir
  tmpdir="$(mktemp -d "${TMPDIR:-/tmp}/thr-install.XXXXXX")"

  if ! curl -fsSL "$archive_url" | tar -xz -C "$tmpdir"; then
    warn "Failed to download or extract: $archive_url"
    _thr_rm_tmp_dir "$tmpdir"
    return 1
  fi

  local top dirs
  dirs=("$tmpdir"/*/)
  if [[ ! -e "${dirs[0]:-}" ]]; then
    warn "Archive layout unexpected after extract (no top-level directory)."
    _thr_rm_tmp_dir "$tmpdir"
    return 1
  fi
  top="${dirs[0]%/}"
  if [[ ! -f "$top/go.mod" ]]; then
    warn "Archive layout unexpected after extract (missing go.mod)."
    _thr_rm_tmp_dir "$tmpdir"
    return 1
  fi

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

  trap '_stop_install_spinner; trap - INT; kill -INT $$' INT

  local ec=0
  set +e
  (cd "$top" && CGO_ENABLED=1 go install -tags "$GO_TAGS" ./cmd/thr)
  ec=$?
  set -e

  trap - INT
  _stop_install_spinner
  if [[ "$ec" -ne 0 ]]; then
    _thr_rm_tmp_dir "$tmpdir"
    return "$ec"
  fi
  _thr_rm_tmp_dir "$tmpdir"
  THR_INSTALLED_BIN="$(_go_bin_dir)/thr"
  return 0
}

run_source_install_flow() {
  local os="$1"
  local mac_system=0

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
      warn "Install dependencies manually, then fetch $(_thr_archive_url "$INSTALL_REF"), extract, and run:"
      warn "  CGO_ENABLED=1 go install -tags \"$GO_TAGS\" ./cmd/thr"
      exit 1
      ;;
  esac

  install_thr_from_source || exit 1

  if [[ "$os" == "Darwin" ]]; then
    if install_thr_to_system_path_macos "$THR_INSTALLED_BIN"; then
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

  finalize_messages "$os" "$mac_system"
}

run_binary_install_flow() {
  local os="$1"
  local mac_system=0

  install_thr_from_github_release || return 1

  if [[ "$os" == "Darwin" ]]; then
    if install_thr_to_system_path_macos "$THR_INSTALLED_BIN"; then
      mac_system=1
    else
      warn "Could not install to system PATH locations; copying to Go bin if available."
      if need_cmd go; then
        mkdir -p "$(_go_bin_dir)"
        install -m 0755 "$THR_INSTALLED_BIN" "$(_go_bin_dir)/thr"
        THR_INSTALLED_BIN="$(_go_bin_dir)/thr"
        ensure_gobin_in_path
      else
        warn "Install failed: could not write to /opt/homebrew/bin and Go is not installed."
        [[ -n "${THR_RELEASE_TMPDIR:-}" ]] && rm -rf "$THR_RELEASE_TMPDIR"
        THR_RELEASE_TMPDIR=""
        return 1
      fi
    fi
  else
    if ! install_thr_user_local_linux "$THR_INSTALLED_BIN"; then
      [[ -n "${THR_RELEASE_TMPDIR:-}" ]] && rm -rf "$THR_RELEASE_TMPDIR"
      THR_RELEASE_TMPDIR=""
      return 1
    fi
  fi

  if [[ -n "${THR_RELEASE_TMPDIR:-}" ]]; then
    rm -rf "$THR_RELEASE_TMPDIR"
    THR_RELEASE_TMPDIR=""
  fi

  if [[ "$os" == "Darwin" ]]; then
    _prepend_default_macos_path
  fi
  if [[ "$mac_system" -eq 0 ]]; then
    apply_local_bin_to_path_in_this_process
    source_shell_rc_in_subshell
  fi

  finalize_messages "$os" "$mac_system"
}

finalize_messages() {
  local os="$1"
  local mac_system="$2"

  if command -v thr >/dev/null 2>&1; then
    log "Ready: $(command -v thr)"
  elif [[ -n "$THR_INSTALLED_BIN" ]] && [[ -x "$THR_INSTALLED_BIN" ]]; then
    log "Installed binary: $THR_INSTALLED_BIN"
  else
    warn "thr is not on PATH in this install session."
  fi

  log "Ensuring the embedding model is in cache (first install may take a minute)..."
  if prefetch_embedding_model; then
    log "Embedding model is available."
  else
    warn "Could not run thr prefetch. The model will download on the first add, ask, or edit."
  fi

  if [[ "$os" == "Darwin" ]] && [[ "$mac_system" -eq 1 ]]; then
    log "On macOS, thr is on your default PATH. Run: thr --help   (re-run this installer anytime to update)"
  else
    log "If thr is not found in this window: source $(_shell_rc_file)  (or open a new tab)"
    log "Re-run this installer to update. Verify: thr --help"
  fi
}

main() {
  local os
  os="$(uname -s)"

  if [[ "${THR_USE_SOURCE:-}" == "1" ]]; then
    _thr_validate_install_ref "$INSTALL_REF" || exit 1
    run_source_install_flow "$os"
    exit 0
  fi

  if run_binary_install_flow "$os"; then
    exit 0
  fi

  warn "Prebuilt install unavailable; falling back to building from source (requires Go)."
  warn "Tip: publish a release (git tag v0.x.x) so binary installs work without a toolchain."
  _thr_validate_install_ref "$INSTALL_REF" || exit 1
  run_source_install_flow "$os"
}

main "$@"
