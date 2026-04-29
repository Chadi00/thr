#!/usr/bin/env bash
set -euo pipefail

REPO_SLUG="Chadi00/thr"
THR_PATH_MARKER="# thr install: add thr bin to PATH (https://github.com/Chadi00/thr)"
THR_LEGACY_HOMEBREW_PATH_MARKER="# thr install: add Homebrew bin to PATH (https://github.com/Chadi00/thr)"
THR_OLD_PATH_MARKER="# thr install: add thr bin dir to PATH (https://github.com/Chadi00/thr)"
THR_OLD_GO_PATH_MARKER="# thr install: add Go bin to PATH (https://github.com/Chadi00/thr)"
THR_DOWNLOAD_BASE_URL="${THR_INSTALL_TEST_BASE_URL:-https://github.com/${REPO_SLUG}/releases/latest/download}"
THR_ONNXRUNTIME_VERSION="1.25.1"
THR_SSH_SIGNING_NAMESPACE="thr-release"
THR_SSH_SIGNING_IDENTITY="thr-release"
# Release jobs must sign checksums.txt with the private key matching this allowed signer.
THR_SSH_ALLOWED_SIGNERS="${THR_INSTALL_ALLOWED_SIGNERS:-thr-release ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIAXr9HFt+bOkFt6Hx9xC5z/KpwBL0Y5RDonM1eqErPKl thr-release}"

THR_TMPDIR=""
THR_EXTRACT_DIR=""
THR_INSTALLED_BIN=""
THR_UPDATED_SHELL_RC=0
THR_AGENT_SKILL_NAMES=("Claude Code" "OpenCode" "Codex" "Other")

log() {
  printf '[thr-install] %s\n' "$*"
}

warn() {
  printf '[thr-install] warning: %s\n' "$*" >&2
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

cleanup() {
  if [[ -n "$THR_TMPDIR" ]]; then
    rm -rf "$THR_TMPDIR"
  fi
}

trap cleanup EXIT

system_name() {
  printf '%s' "${THR_INSTALL_TEST_UNAME_S:-$(uname -s)}"
}

machine_name() {
  printf '%s' "${THR_INSTALL_TEST_UNAME_M:-$(uname -m)}"
}

normalize_os() {
  case "$(system_name)" in
    Darwin | darwin) printf '%s' 'darwin' ;;
    Linux | linux) printf '%s' 'linux' ;;
    *)
      warn "Unsupported operating system: $(system_name)"
      return 1
      ;;
  esac
}

normalize_arch() {
  case "$(machine_name)" in
    arm64 | aarch64) printf '%s' 'arm64' ;;
    x86_64 | amd64) printf '%s' 'amd64' ;;
    *)
      warn "Unsupported architecture: $(machine_name)"
      return 1
      ;;
  esac
}

runtime_library_name() {
  case "$1" in
    darwin) printf '%s' 'libonnxruntime.dylib' ;;
    linux) printf '%s' 'libonnxruntime.so' ;;
    *)
      warn "Unsupported runtime operating system: $1"
      return 1
      ;;
  esac
}

current_target() {
  local os arch

  os="$(normalize_os)" || return 1
  arch="$(normalize_arch)" || return 1
  printf '%s-%s' "$os" "$arch"
}

sha256_file() {
  if need_cmd sha256sum; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

verify_signed_checksums() {
  local allowed_signers

  allowed_signers="${THR_TMPDIR}/allowed_signers"
  printf '%s\n' "$THR_SSH_ALLOWED_SIGNERS" >"$allowed_signers"
  if ! ssh-keygen -Y verify \
    -f "$allowed_signers" \
    -I "$THR_SSH_SIGNING_IDENTITY" \
    -n "$THR_SSH_SIGNING_NAMESPACE" \
    -s "${THR_TMPDIR}/checksums.txt.sig" \
    <"${THR_TMPDIR}/checksums.txt" >/dev/null; then
    warn "Could not verify signed release checksums."
    return 1
  fi
}

validate_archive_layout() {
  local archive="$1"
  local target="$2"
  local os="${target%%-*}"
  local entry has_bin=0 has_manifest=0 has_runtime=0
  local runtime_lib

  runtime_lib="lib/thr/onnxruntime/${THR_ONNXRUNTIME_VERSION}/${target}/$(runtime_library_name "$os")"

  while IFS= read -r entry; do
    case "$entry" in
      "" | /* | ./* | *"/../"* | "../"* | *"/.." | "..")
        warn "Archive contains an unsafe path: ${entry}"
        return 1
        ;;
      bin/thr)
        has_bin=1
        ;;
      bin/ | lib/ | lib/thr/ | lib/thr/*/)
        ;;
      manifest.json)
        has_manifest=1
        ;;
      "$runtime_lib")
        has_runtime=1
        ;;
      lib/thr/*)
        ;;
      *)
        warn "Archive contains an unexpected path: ${entry}"
        return 1
        ;;
    esac
  done < <(tar -tzf "$archive")

  if [[ "$has_bin" -ne 1 || "$has_manifest" -ne 1 || "$has_runtime" -ne 1 ]]; then
    warn "Archive is missing bin/thr, manifest.json, or the packaged ONNX Runtime library."
    return 1
  fi
}

download_release_archive() {
  local target os arch archive expected actual

  if ! need_cmd curl || ! need_cmd tar || ! need_cmd ssh-keygen; then
    warn "Install requires curl, tar, and ssh-keygen."
    return 1
  fi

  target="$(current_target)" || return 1
  os="${target%%-*}"
  arch="${target#*-}"
  archive="thr_${os}_${arch}.tar.gz"
  THR_TMPDIR="$(mktemp -d "${TMPDIR:-/tmp}/thr-install.XXXXXX")"
  THR_EXTRACT_DIR="${THR_TMPDIR}/extract"
  mkdir -p "$THR_EXTRACT_DIR"

  log "Downloading ${archive}..."
  curl -fsSL "${THR_DOWNLOAD_BASE_URL}/${archive}" -o "${THR_TMPDIR}/${archive}"
  curl -fsSL "${THR_DOWNLOAD_BASE_URL}/checksums.txt" -o "${THR_TMPDIR}/checksums.txt"
  curl -fsSL "${THR_DOWNLOAD_BASE_URL}/checksums.txt.sig" -o "${THR_TMPDIR}/checksums.txt.sig"

  verify_signed_checksums

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

  validate_archive_layout "${THR_TMPDIR}/${archive}" "$target"

  tar -xzf "${THR_TMPDIR}/${archive}" -C "$THR_EXTRACT_DIR"
  if [[ ! -f "${THR_EXTRACT_DIR}/bin/thr" ]]; then
    warn "Archive did not contain a thr binary."
    return 1
  fi

  chmod +x "${THR_EXTRACT_DIR}/bin/thr"
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
  awk -v m1="$THR_PATH_MARKER" -v m2="$THR_LEGACY_HOMEBREW_PATH_MARKER" -v m3="$THR_OLD_PATH_MARKER" -v m4="$THR_OLD_GO_PATH_MARKER" '
    $0 == m1 || $0 == m2 || $0 == m3 || $0 == m4 { skip = 1; next }
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

  if ! confirm "Add ${dir} to PATH in ${rc}?"; then
    warn "Skipped PATH update. Run ${dir}/thr directly or add ${dir} to PATH later."
    return 0
  fi

  strip_thr_path_blocks "$rc"
  {
    printf '\n%s\n' "$THR_PATH_MARKER"
    printf "export PATH=\"%s:\\$PATH\"\n" "$dir"
  } >>"$rc"

  export PATH="$dir:$PATH"
  THR_UPDATED_SHELL_RC=1
  log "Added ${dir} to PATH in ${rc}"
}

install_prefix() {
  printf '%s' "${THR_INSTALL_PREFIX:-$HOME/.local}"
}

install_bin_dir() {
  printf '%s/bin' "$(install_prefix)"
}

install_lib_dir() {
  printf '%s/lib/thr' "$(install_prefix)"
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
  local dir lib_dir dst

  dir="$(install_bin_dir)"
  lib_dir="$(install_lib_dir)"
  dst="${dir}/thr"
  ensure_install_dir_exists "$dir"
  ensure_install_dir_exists "$lib_dir"

  if [[ -w "$dir" ]]; then
    install -m 0755 "${THR_EXTRACT_DIR}/bin/thr" "$dst"
  else
    log "Installing thr to ${dst} (you may be prompted for sudo)..."
    sudo install -m 0755 "${THR_EXTRACT_DIR}/bin/thr" "$dst"
  fi

  if [[ -w "$lib_dir" ]]; then
    rm -rf "${lib_dir}/onnxruntime/${THR_ONNXRUNTIME_VERSION}"
    mkdir -p "$lib_dir"
    cp -R "${THR_EXTRACT_DIR}/lib/thr/." "$lib_dir/"
    install -m 0644 "${THR_EXTRACT_DIR}/manifest.json" "${lib_dir}/manifest.json"
  else
    log "Installing thr runtime files to ${lib_dir} (you may be prompted for sudo)..."
    sudo rm -rf "${lib_dir}/onnxruntime/${THR_ONNXRUNTIME_VERSION}"
    sudo mkdir -p "$lib_dir"
    sudo cp -R "${THR_EXTRACT_DIR}/lib/thr/." "$lib_dir/"
    sudo install -m 0644 "${THR_EXTRACT_DIR}/manifest.json" "${lib_dir}/manifest.json"
  fi

  THR_INSTALLED_BIN="$dst"
  ensure_dir_on_path "$dir"
  log "Installed thr to ${dst}"
}

prefetch_model() {
  log "Preparing the bundled embedding model..."
  if "$THR_INSTALLED_BIN" prefetch; then
    log "Embedding model is ready."
    return 0
  fi

  warn "Could not run thr prefetch. The bundled model will be prepared on the first add, ask, or edit."
  return 0
}

install_agent_skill() {
  local target="$1"

  if "$THR_INSTALLED_BIN" setup "$target"; then
    return 0
  fi

  warn "Could not install the thr skill for ${target}. You can retry later with: thr setup ${target}"
  return 0
}

print_other_skill_guidance() {
  log "Install the thr skill manually from:"
  log "https://github.com/${REPO_SLUG}/tree/master/skills/thr"
}

agent_skill_display_name() {
  case "$1" in
    claude-code) printf '%s' 'Claude Code' ;;
    opencode) printf '%s' 'OpenCode' ;;
    codex) printf '%s' 'Codex' ;;
    other) printf '%s' 'Other' ;;
    *) printf '%s' "$1" ;;
  esac
}

agent_skill_choice_target() {
  case "$1" in
    1 | c | cc | claude | claude-code | claudecode)
      printf '%s' 'claude-code'
      ;;
    2 | o | opencode | open-code | open_code)
      printf '%s' 'opencode'
      ;;
    3 | codex)
      printf '%s' 'codex'
      ;;
    4 | other | manual)
      printf '%s' 'other'
      ;;
    *)
      return 1
      ;;
  esac
}

agent_skill_targets_from_flags() {
  local claude="$1"
  local opencode="$2"
  local codex="$3"
  local other="$4"
  local sep=""

  if [[ "$claude" -eq 1 ]]; then
    printf '%s%s' "$sep" 'claude-code'
    sep=" "
  fi
  if [[ "$opencode" -eq 1 ]]; then
    printf '%s%s' "$sep" 'opencode'
    sep=" "
  fi
  if [[ "$codex" -eq 1 ]]; then
    printf '%s%s' "$sep" 'codex'
    sep=" "
  fi
  if [[ "$other" -eq 1 ]]; then
    printf '%s%s' "$sep" 'other'
  fi
}

parse_agent_skill_selection() {
  local input="$1"
  local normalized token target
  local -a tokens=()
  local i=0
  local selected_claude=0
  local selected_opencode=0
  local selected_codex=0
  local selected_other=0
  local skip_requested=0

  normalized="$(printf '%s' "$input" | tr '[:upper:]' '[:lower:]')"
  normalized="${normalized//,/ }"
  normalized="${normalized//;/ }"

  if [[ -z "${normalized//[[:space:]]/}" ]]; then
    return 1
  fi

  read -r -a tokens <<<"$normalized"
  while [[ "$i" -lt "${#tokens[@]}" ]]; do
    token="${tokens[$i]}"
    if [[ "$token" == "claude" && $((i + 1)) -lt "${#tokens[@]}" && "${tokens[$((i + 1))]}" == "code" ]]; then
      token="claude-code"
      i=$((i + 1))
    elif [[ "$token" == "open" && $((i + 1)) -lt "${#tokens[@]}" && "${tokens[$((i + 1))]}" == "code" ]]; then
      token="opencode"
      i=$((i + 1))
    fi

    case "$token" in
      q | quit | skip | none | no)
        skip_requested=1
        i=$((i + 1))
        continue
        ;;
    esac

    if ! target="$(agent_skill_choice_target "$token")"; then
      return 2
    fi

    case "$target" in
      claude-code) selected_claude=1 ;;
      opencode) selected_opencode=1 ;;
      codex) selected_codex=1 ;;
      other) selected_other=1 ;;
    esac
    i=$((i + 1))
  done

  if [[ "$skip_requested" -eq 1 ]]; then
    if [[ "$selected_claude" -eq 1 || "$selected_opencode" -eq 1 || "$selected_codex" -eq 1 || "$selected_other" -eq 1 ]]; then
      return 2
    fi
    return 1
  fi

  agent_skill_targets_from_flags "$selected_claude" "$selected_opencode" "$selected_codex" "$selected_other"
}

agent_skill_confirmation_reply_is_yes() {
  case "$1" in
    y | Y | yes | YES | Yes) return 0 ;;
    *) return 1 ;;
  esac
}

confirm_agent_skill_targets() {
  local targets="$1"
  local target reply

  printf '\nSelected coding agents:\n' >&3
  for target in $targets; do
    printf '  - %s\n' "$(agent_skill_display_name "$target")" >&3
  done
  printf 'Install thr skill for these selections? [y/N] ' >&3

  if ! IFS= read -r reply <&3; then
    return 1
  fi

  if agent_skill_confirmation_reply_is_yes "$reply"; then
    return 0
  fi

  printf 'Skipped coding agent skill setup.\n' >&3
  return 1
}

agent_skill_selector_supported() {
  [[ -t 3 ]] || return 1
  [[ -n "${TERM:-}" && "${TERM:-}" != "dumb" ]] || return 1
}

render_agent_skill_selector() {
  local cursor="$1"
  local selected_claude="$2"
  local selected_opencode="$3"
  local selected_codex="$4"
  local selected_other="$5"
  local i marker checked selected

  printf '\033[2K\rSelect coding agents. Space toggles, Enter reviews, q skips.\n' >&3
  for i in 0 1 2 3; do
    selected=0
    case "$i" in
      0) selected="$selected_claude" ;;
      1) selected="$selected_opencode" ;;
      2) selected="$selected_codex" ;;
      3) selected="$selected_other" ;;
    esac

    marker=" "
    if [[ "$i" -eq "$cursor" ]]; then
      marker=">"
    fi

    checked=" "
    if [[ "$selected" -eq 1 ]]; then
      checked="x"
    fi

    printf '\033[2K\r  %s [%s] %s\n' "$marker" "$checked" "${THR_AGENT_SKILL_NAMES[$i]}" >&3
  done
  printf '\033[2K\rUse arrow keys or j/k to move.\n' >&3
}

read_agent_skill_selector_key() {
  local key rest

  if ! IFS= read -rsn1 key <&3; then
    return 1
  fi

  case "$key" in
    $'\033')
      if IFS= read -rsn2 -t 1 rest <&3; then
        case "$rest" in
          '[A') printf '%s' 'up' ;;
          '[B') printf '%s' 'down' ;;
          *) printf '%s' 'escape' ;;
        esac
      else
        printf '%s' 'escape'
      fi
      ;;
    "")
      printf '%s' 'enter'
      ;;
    " ")
      printf '%s' 'space'
      ;;
    j | J)
      printf '%s' 'down'
      ;;
    k | K)
      printf '%s' 'up'
      ;;
    q | Q)
      printf '%s' 'quit'
      ;;
    *)
      printf '%s' 'other'
      ;;
  esac
}

toggle_agent_skill_selection() {
  case "$1" in
    0)
      if [[ "$selected_claude" -eq 1 ]]; then selected_claude=0; else selected_claude=1; fi
      ;;
    1)
      if [[ "$selected_opencode" -eq 1 ]]; then selected_opencode=0; else selected_opencode=1; fi
      ;;
    2)
      if [[ "$selected_codex" -eq 1 ]]; then selected_codex=0; else selected_codex=1; fi
      ;;
    3)
      if [[ "$selected_other" -eq 1 ]]; then selected_other=0; else selected_other=1; fi
      ;;
  esac
}

select_agent_skills_interactive() {
  local cursor=0
  local selected_claude=0
  local selected_opencode=0
  local selected_codex=0
  local selected_other=0
  local key targets
  local selector_lines=6
  local rendered=0

  if ! agent_skill_selector_supported; then
    return 2
  fi

  printf '\n' >&3
  while true; do
    if [[ "$rendered" -eq 1 ]]; then
      printf '\033[%dA' "$selector_lines" >&3
    fi
    render_agent_skill_selector "$cursor" "$selected_claude" "$selected_opencode" "$selected_codex" "$selected_other"
    rendered=1

    if ! key="$(read_agent_skill_selector_key)"; then
      printf '\nSkipped coding agent skill setup.\n' >&3
      return 1
    fi

    case "$key" in
      up)
        if [[ "$cursor" -eq 0 ]]; then cursor=3; else cursor=$((cursor - 1)); fi
        ;;
      down)
        if [[ "$cursor" -eq 3 ]]; then cursor=0; else cursor=$((cursor + 1)); fi
        ;;
      space)
        toggle_agent_skill_selection "$cursor"
        ;;
      enter)
        targets="$(agent_skill_targets_from_flags "$selected_claude" "$selected_opencode" "$selected_codex" "$selected_other")"
        if [[ -z "$targets" ]]; then
          printf '\nNo coding agents selected; skipping skill setup.\n' >&3
          return 1
        fi

        if confirm_agent_skill_targets "$targets"; then
          printf '%s\n' "$targets"
          return 0
        fi
        return 1
        ;;
      quit | escape)
        printf '\nSkipped coding agent skill setup.\n' >&3
        return 1
        ;;
    esac
  done
}

select_agent_skills_fallback() {
  local input targets parse_status

  printf '\nTerminal does not support the interactive selector.\n' >&3
  while true; do
    printf 'Enter coding agents to install (claude-code, opencode, codex, other; comma/space separated; blank to skip): ' >&3
    if ! IFS= read -r input <&3; then
      return 1
    fi

    if targets="$(parse_agent_skill_selection "$input")"; then
      if confirm_agent_skill_targets "$targets"; then
        printf '%s\n' "$targets"
        return 0
      fi
      return 1
    else
      parse_status="$?"
    fi

    case "$parse_status" in
      1)
        printf 'Skipped coding agent skill setup.\n' >&3
        return 1
        ;;
      *)
        printf 'Please enter one or more of: claude-code, opencode, codex, other; or q to skip.\n' >&3
        ;;
    esac
  done
}

install_selected_agent_skills() {
  local targets="$1"
  local target install_other=0

  for target in $targets; do
    case "$target" in
      other)
        install_other=1
        ;;
      *)
        install_agent_skill "$target"
        ;;
    esac
  done

  if [[ "$install_other" -eq 1 ]]; then
    print_other_skill_guidance
  fi
}

offer_agent_skill_setup() {
  local reply selected_targets selection_status

  if [[ "${THR_INSTALL_SKIP_SKILL_PROMPT:-0}" == "1" ]]; then
    return 0
  fi

  if ! { exec 3<>/dev/tty; } 2>/dev/null; then
    return 0
  fi

  printf 'Install the thr skill for a coding agent? [y/N] ' >&3
  if ! IFS= read -r reply <&3; then
    exec 3>&-
    return 0
  fi
  case "$reply" in
    y | Y | yes | YES | Yes) ;;
    *)
      exec 3>&-
      return 0
      ;;
  esac

  if selected_targets="$(select_agent_skills_interactive)"; then
    exec 3>&-
    install_selected_agent_skills "$selected_targets"
    return 0
  else
    selection_status="$?"
  fi

  if [[ "$selection_status" -eq 2 ]] && selected_targets="$(select_agent_skills_fallback)"; then
    exec 3>&-
    install_selected_agent_skills "$selected_targets"
    return 0
  fi

  exec 3>&-
  return 0
}

main() {
  download_release_archive
  install_binary
  prefetch_model
  offer_agent_skill_setup

  log "Ready: ${THR_INSTALLED_BIN}"
  if [[ "$THR_UPDATED_SHELL_RC" -eq 1 ]]; then
    log "If thr is not found in new shells yet: source $(shell_rc_file)  (or open a new terminal)"
  fi
  log "Verify: thr --help"
}

if [[ "${THR_INSTALL_LIB_ONLY:-0}" != "1" ]]; then
  main "$@"
fi
