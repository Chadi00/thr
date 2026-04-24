#!/usr/bin/env bash
set -euo pipefail

REPO_SLUG="Chadi00/thr"
THR_VERSION="${THR_VERSION:-latest}"
INSTALL_REF="${THR_INSTALL_REF:-master}"
THR_INSTALL_DIR="${THR_INSTALL_DIR:-}"
GO_TAGS="sqlite_fts5"
GO_MIN_VERSION="1.26.2"
THR_PATH_MARKER="# thr install: add thr bin dir to PATH (https://github.com/Chadi00/thr)"
THR_OLD_PATH_MARKER="# thr install: add Go bin to PATH (https://github.com/Chadi00/thr)"

THR_INSTALLED_BIN=""
THR_INSTALLED_DIR=""
THR_RELEASE_TMPDIR=""
THR_BUILD_TMPDIR=""
THR_SPINNER_PID=""
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
  stop_install_spinner
  if [[ -n "$THR_RELEASE_TMPDIR" ]]; then
    rm -rf "$THR_RELEASE_TMPDIR"
  fi
  if [[ -n "$THR_BUILD_TMPDIR" ]]; then
    rm -rf "$THR_BUILD_TMPDIR"
  fi
  return 0
}

trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

ensure_homebrew() {
  if need_cmd brew; then
    return 0
  fi

  warn "Homebrew is required on macOS to install ONNX Runtime automatically."
  warn "Install Homebrew from https://brew.sh and re-run this command."
  return 1
}

go_version() {
  local version
  version="$(go env GOVERSION 2>/dev/null || true)"
  if [[ -z "$version" ]]; then
    version="$(go version | awk '{print $3}')"
  fi
  printf '%s' "${version#go}"
}

version_ge() {
  local have="$1"
  local want="$2"
  local IFS=.
  local -a have_parts want_parts
  local i have_num want_num

  read -r -a have_parts <<<"$have"
  read -r -a want_parts <<<"$want"

  for i in 0 1 2; do
    have_num="${have_parts[i]:-0}"
    want_num="${want_parts[i]:-0}"
    have_num="${have_num%%[^0-9]*}"
    want_num="${want_num%%[^0-9]*}"
    have_num="${have_num:-0}"
    want_num="${want_num:-0}"
    if (( have_num > want_num )); then
      return 0
    fi
    if (( have_num < want_num )); then
      return 1
    fi
  done

  return 0
}

ensure_go_version() {
  local current
  current="$(go_version)"
  if version_ge "$current" "$GO_MIN_VERSION"; then
    return 0
  fi

  warn "Go $GO_MIN_VERSION+ is required for source installs, but found Go $current."
  warn "Install a newer Go toolchain and re-run this command."
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

  warn "Go $GO_MIN_VERSION+ is required for source installs."
  warn "This installer officially supports apt-based Linux for automatic dependency setup."
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
  if need_cmd gcc && need_cmd pkg-config && pkg-config --exists sqlite3; then
    return 0
  fi

  if need_cmd apt-get; then
    log "Installing Linux build dependencies for source install..."
    sudo apt-get update
    sudo apt-get install -y build-essential pkg-config libsqlite3-dev
    return 0
  fi

  warn "CGO build tools and sqlite3 development headers are required for source installs."
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

has_onnx_linux() {
  local pattern

  if need_cmd ldconfig && ldconfig -p 2>/dev/null | grep -q 'libonnxruntime'; then
    return 0
  fi

  for pattern in \
    /usr/lib/libonnxruntime.so* \
    /usr/local/lib/libonnxruntime.so* \
    /usr/lib/x86_64-linux-gnu/libonnxruntime.so* \
    /usr/lib/aarch64-linux-gnu/libonnxruntime.so*; do
    if compgen -G "$pattern" >/dev/null 2>&1; then
      return 0
    fi
  done

  return 1
}

ensure_onnx_linux() {
  if has_onnx_linux; then
    return 0
  fi

  if need_cmd apt-get; then
    log "Installing ONNX Runtime via apt..."
    sudo apt-get update
    if sudo apt-get install -y libonnxruntime-dev || sudo apt-get install -y libonnxruntime1; then
      if has_onnx_linux; then
        return 0
      fi
    fi
  fi

  warn "ONNX Runtime is required for thr embedding commands on Linux."
  warn "Install libonnxruntime with apt or manually place libonnxruntime on the system library path, then re-run this installer."
  return 1
}

# Spinner frames match briandowns/spinner CharSets[11] (braille).
_spinner_frames=(⣾ ⣽ ⣻ ⢿ ⡿ ⣟ ⣯ ⣷)

start_install_spinner() {
  local message="$1"
  local i n

  stop_install_spinner
  (
    i=0
    n=${#_spinner_frames[@]}
    while true; do
      printf '\r\033[K[thr-install] %s %s' "${_spinner_frames[i]}" "$message" >&2
      i=$(( (i + 1) % n ))
      sleep 0.1
    done
  ) &
  THR_SPINNER_PID=$!
}

stop_install_spinner() {
  if [[ -n "$THR_SPINNER_PID" ]]; then
    kill "$THR_SPINNER_PID" 2>/dev/null || true
    wait "$THR_SPINNER_PID" 2>/dev/null || true
    THR_SPINNER_PID=""
    printf '\r\033[K' >&2
  fi
  return 0
}

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
    v*) return 0 ;;
    *)
      warn "Invalid THR_VERSION (use latest or a tag like v0.1.2): $1"
      return 1
      ;;
  esac
}

_thr_archive_url() {
  local ref="$1"

  if [[ -n "${THR_SOURCE_ARCHIVE_URL:-}" ]]; then
    printf '%s' "$THR_SOURCE_ARCHIVE_URL"
    return 0
  fi

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
    Darwin) printf '%s %s' 'darwin' "$(_thr_normalize_arch "$arch_raw")" ;;
    Linux) printf '%s %s' 'linux' "$(_thr_normalize_arch "$arch_raw")" ;;
    *) return 1 ;;
  esac
}

_thr_normalize_arch() {
  case "$1" in
    x86_64 | amd64) printf '%s' 'amd64' ;;
    arm64 | aarch64) printf '%s' 'arm64' ;;
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

  if [[ -n "${THR_RELEASE_API_URL:-}" ]]; then
    printf '%s' "$THR_RELEASE_API_URL"
    return 0
  fi

  if [[ "$version" == 'latest' ]]; then
    printf 'https://api.github.com/repos/%s/releases/latest' "$REPO_SLUG"
  else
    printf 'https://api.github.com/repos/%s/releases/tags/%s' "$REPO_SLUG" "$version"
  fi
}

_thr_curl_github_json() {
  local url="$1"
  if [[ -n "${GITHUB_TOKEN:-}" ]]; then
    curl -fsSL -H "Authorization: Bearer ${GITHUB_TOKEN}" -H 'Accept: application/vnd.github+json' "$url"
  else
    curl -fsSL -H 'Accept: application/vnd.github+json' "$url"
  fi
}

_thr_json_asset_url() {
  local asset_name="$1"
  python3 -c '
import json, sys
name = sys.argv[1]
data = json.load(sys.stdin)
for asset in data.get("assets", []):
    if asset.get("name") == name:
        print(asset.get("browser_download_url", ""))
        break
' "$asset_name"
}

download_thr_from_github_release() {
  local goos goarch triple archive asset_url checksums_url expected actual json

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

  if ! json="$(_thr_curl_github_json "$(_thr_release_api_url "$THR_VERSION")")"; then
    warn "No GitHub release found for THR_VERSION=$THR_VERSION."
    return 1
  fi

  asset_url="$(printf '%s' "$json" | _thr_json_asset_url "$archive")"
  checksums_url="$(printf '%s' "$json" | _thr_json_asset_url 'checksums.txt')"
  if [[ -z "$asset_url" ]] || [[ -z "$checksums_url" ]]; then
    warn "Release is missing $archive or checksums.txt."
    return 1
  fi

  THR_RELEASE_TMPDIR="$(mktemp -d "${TMPDIR:-/tmp}/thr-install.XXXXXX")"

  log "Downloading $archive from GitHub Releases (THR_VERSION=$THR_VERSION)..."
  curl -fsSL "$asset_url" -o "$THR_RELEASE_TMPDIR/$archive"
  curl -fsSL "$checksums_url" -o "$THR_RELEASE_TMPDIR/checksums.txt"

  expected="$(awk -v name="$archive" '$2 == name {print $1; exit}' "$THR_RELEASE_TMPDIR/checksums.txt")"
  if [[ -z "$expected" ]]; then
    warn "Could not find checksum line for $archive."
    return 1
  fi

  actual="$(_thr_sha256_file "$THR_RELEASE_TMPDIR/$archive")"
  if [[ "$actual" != "$expected" ]]; then
    warn "Checksum mismatch for $archive (expected $expected, got $actual)."
    return 1
  fi

  tar -xzf "$THR_RELEASE_TMPDIR/$archive" -C "$THR_RELEASE_TMPDIR"
  if [[ ! -f "$THR_RELEASE_TMPDIR/thr" ]]; then
    warn "Archive did not contain a thr binary."
    return 1
  fi
  chmod +x "$THR_RELEASE_TMPDIR/thr"
  THR_INSTALLED_BIN="$THR_RELEASE_TMPDIR/thr"
}

build_thr_from_source() {
  local archive_url top dirs ec

  _thr_validate_install_ref "$INSTALL_REF" || return 1
  if ! need_cmd curl || ! need_cmd tar || ! need_cmd go; then
    warn "Source install needs curl, tar, and Go."
    return 1
  fi

  archive_url="$(_thr_archive_url "$INSTALL_REF")"
  THR_BUILD_TMPDIR="$(mktemp -d "${TMPDIR:-/tmp}/thr-install.XXXXXX")"

  log "Building thr from source..."
  log "Archive: $archive_url"
  curl -fsSL "$archive_url" | tar -xz -C "$THR_BUILD_TMPDIR"

  dirs=("$THR_BUILD_TMPDIR"/*/)
  if [[ ! -e "${dirs[0]:-}" ]]; then
    warn "Archive layout unexpected after extract (no top-level directory)."
    return 1
  fi
  top="${dirs[0]%/}"
  if [[ ! -f "$top/go.mod" ]]; then
    warn "Archive layout unexpected after extract (missing go.mod)."
    return 1
  fi

  start_install_spinner 'still working (go build)...'
  set +e
  (cd "$top" && CGO_ENABLED=1 go build -trimpath -tags "$GO_TAGS" -o "$THR_BUILD_TMPDIR/thr" ./cmd/thr)
  ec=$?
  set -e
  stop_install_spinner

  if [[ "$ec" -ne 0 ]]; then
    return "$ec"
  fi
  if [[ ! -f "$THR_BUILD_TMPDIR/thr" ]]; then
    warn "Source build completed without producing a thr binary."
    return 1
  fi
  chmod +x "$THR_BUILD_TMPDIR/thr"
  THR_INSTALLED_BIN="$THR_BUILD_TMPDIR/thr"
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

strip_thr_path_blocks() {
  local file="$1"
  local tmp

  [[ -f "$file" ]] || return 0
  tmp="$(mktemp "${TMPDIR:-/tmp}/thr-install.XXXXXX")"
  awk -v m1="$THR_PATH_MARKER" -v m2="$THR_OLD_PATH_MARKER" '
    $0 == m1 || $0 == m2 { skip = 1; next }
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

  rc="$(_shell_rc_file)"
  mkdir -p "$(dirname "$rc")"
  strip_thr_path_blocks "$rc"

  {
    printf '\n%s\n' "$THR_PATH_MARKER"
    printf 'export PATH="%s:$PATH"\n' "$dir"
  } >>"$rc"

  THR_UPDATED_SHELL_RC=1
  log "Added $dir to PATH in $rc"
}

ensure_install_dir_exists() {
  local dir="$1"

  if [[ -d "$dir" ]]; then
    return 0
  fi

  if mkdir -p "$dir" 2>/dev/null; then
    return 0
  fi

  log "Creating $dir (you may be prompted for sudo)..."
  sudo mkdir -p "$dir"
}

install_thr_to_dir() {
  local src="$1"
  local dir="$2"
  local dst

  ensure_install_dir_exists "$dir"
  dst="$dir/thr"

  if [[ -w "$dir" ]]; then
    install -m 0755 "$src" "$dst"
  else
    log "Installing thr to $dst (you may be prompted for sudo)..."
    sudo install -m 0755 "$src" "$dst"
  fi

  THR_INSTALLED_BIN="$dst"
  THR_INSTALLED_DIR="$dir"
  log "Installed thr to $dst"
}

resolve_install_dir() {
  local os="$1"

  if [[ -n "$THR_INSTALL_DIR" ]]; then
    printf '%s' "$THR_INSTALL_DIR"
    return 0
  fi

  case "$os" in
    Darwin)
      if need_cmd brew; then
        printf '%s' "$(brew --prefix)/bin"
        return 0
      fi
      if [[ -d /opt/homebrew/bin ]]; then
        printf '%s' '/opt/homebrew/bin'
      else
        printf '%s' '/usr/local/bin'
      fi
      ;;
    Linux)
      printf '%s' "${THR_USER_BIN:-$HOME/.local/bin}"
      ;;
    *)
      return 1
      ;;
  esac
}

apply_install_dir_to_path_in_this_process() {
  local dir="$1"
  if [[ ":$PATH:" != *":$dir:"* ]]; then
    export PATH="$dir:$PATH"
  fi
}

install_prepared_binary() {
  local os="$1"
  local dir

  if [[ ! -f "$THR_INSTALLED_BIN" ]]; then
    warn "Internal error: no prepared thr binary to install."
    return 1
  fi

  dir="$(resolve_install_dir "$os")" || return 1
  install_thr_to_dir "$THR_INSTALLED_BIN" "$dir"
  ensure_dir_on_path "$dir"
  apply_install_dir_to_path_in_this_process "$dir"
}

prefetch_embedding_model() {
  if [[ -n "$THR_INSTALLED_BIN" ]] && [[ -x "$THR_INSTALLED_BIN" ]]; then
    "$THR_INSTALLED_BIN" prefetch
    return $?
  fi

  if command -v thr >/dev/null 2>&1; then
    thr prefetch
    return $?
  fi

  return 1
}

finalize_messages() {
  log "Ready: $(command -v thr 2>/dev/null || printf '%s' "$THR_INSTALLED_BIN")"
  log "Ensuring the embedding model is in cache (first install may take a minute)..."
  if prefetch_embedding_model; then
    log "Embedding model is available."
  else
    warn "Could not run thr prefetch. The model will download on the first add, ask, or edit."
  fi

  if [[ "$THR_UPDATED_SHELL_RC" -eq 1 ]]; then
    log "If thr is not found in new shells yet: source $(_shell_rc_file)  (or open a new terminal)"
  fi
  log "Verify: thr --help"
}

run_source_install_flow() {
  local os="$1"

  case "$os" in
    Darwin)
      ensure_go_macos
      ensure_go_version
      ensure_build_tools_macos
      ensure_onnx_macos
      ;;
    Linux)
      ensure_go_linux
      ensure_go_version
      ensure_build_tools_linux
      ensure_onnx_linux
      ;;
    *)
      warn "Unsupported OS: $os"
      return 1
      ;;
  esac

  build_thr_from_source
  install_prepared_binary "$os"
  finalize_messages
}

run_binary_install_flow() {
  local os="$1"

  case "$os" in
    Darwin) ensure_onnx_macos ;;
    Linux) ensure_onnx_linux ;;
    *)
      warn "Unsupported OS: $os"
      return 1
      ;;
  esac

  download_thr_from_github_release
  install_prepared_binary "$os"
  finalize_messages
}

main() {
  local os
  os="$(uname -s)"

  if [[ "${THR_USE_SOURCE:-}" == '1' ]]; then
    run_source_install_flow "$os"
    return 0
  fi

  if run_binary_install_flow "$os"; then
    return 0
  fi

  warn "Prebuilt install unavailable; falling back to building from source."
  run_source_install_flow "$os"
}

main "$@"
