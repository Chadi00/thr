#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# shellcheck source=../install.sh disable=SC1091
THR_INSTALL_LIB_ONLY=1 source "$ROOT_DIR/install.sh"

fail() {
  printf '[installer-prompt-test] %s\n' "$*" >&2
  exit 1
}

assert_parse() {
  local input="$1"
  local expected="$2"
  local got status

  if got="$(parse_agent_skill_selection "$input")"; then
    status=0
  else
    status="$?"
  fi

  if [[ "$status" -ne 0 ]]; then
    fail "expected parse success for '${input}', got status ${status}"
  fi
  if [[ "$got" != "$expected" ]]; then
    fail "expected '${input}' to parse as '${expected}', got '${got}'"
  fi
}

assert_parse_skip() {
  local input="$1"
  local got status

  if got="$(parse_agent_skill_selection "$input")"; then
    status=0
  else
    status="$?"
  fi

  if [[ "$status" -ne 1 ]]; then
    fail "expected '${input}' to skip with status 1, got status ${status} and output '${got}'"
  fi
}

assert_parse_invalid() {
  local input="$1"
  local got status

  if got="$(parse_agent_skill_selection "$input")"; then
    status=0
  else
    status="$?"
  fi

  if [[ "$status" -ne 2 ]]; then
    fail "expected '${input}' to be invalid with status 2, got status ${status} and output '${got}'"
  fi
}

assert_confirm_yes() {
  local reply="$1"

  if ! agent_skill_confirmation_reply_is_yes "$reply"; then
    fail "expected confirmation reply '${reply}' to proceed"
  fi
}

assert_confirm_no() {
  local reply="$1"

  if agent_skill_confirmation_reply_is_yes "$reply"; then
    fail "expected confirmation reply '${reply}' to cancel"
  fi
}

assert_key() {
  local bytes="$1"
  local expected="$2"
  local got

  exec 3< <(printf '%b' "$bytes")
  got="$(read_agent_skill_selector_key)"
  exec 3<&-

  if [[ "$got" != "$expected" ]]; then
    fail "expected key bytes '${bytes}' to read as '${expected}', got '${got}'"
  fi
}

assert_toggle() {
  local selected_claude=0
  local selected_opencode=0
  local selected_codex=0
  local selected_other=0

  toggle_agent_skill_selection 0
  toggle_agent_skill_selection 2
  if [[ "$selected_claude" -ne 1 || "$selected_opencode" -ne 0 || "$selected_codex" -ne 1 || "$selected_other" -ne 0 ]]; then
    fail "expected toggle to select Claude Code and Codex"
  fi

  toggle_agent_skill_selection 0
  if [[ "$selected_claude" -ne 0 || "$selected_codex" -ne 1 ]]; then
    fail "expected second toggle to clear Claude Code only"
  fi
}

main() {
  assert_parse 'claude' 'claude-code'
  assert_parse 'claude,codex' 'claude-code codex'
  assert_parse 'codex claude' 'claude-code codex'
  assert_parse 'Claude Code, OpenCode, Other' 'claude-code opencode other'
  assert_parse '1 2 3 4' 'claude-code opencode codex other'
  assert_parse 'claude claude codex' 'claude-code codex'

  assert_parse_skip ''
  assert_parse_skip 'q'
  assert_parse_skip 'skip'

  assert_parse_invalid 'vim'
  assert_parse_invalid 'claude q'

  assert_confirm_yes 'y'
  assert_confirm_yes 'Y'
  assert_confirm_yes 'yes'
  assert_confirm_yes 'YES'
  assert_confirm_no ''
  assert_confirm_no 'n'
  assert_confirm_no 'no'
  assert_key '\033[A' 'up'
  assert_key '\033[B' 'down'
  assert_key ' ' 'space'
  assert_key '\n' 'enter'
  assert_key 'q' 'quit'
  assert_toggle

  printf '[installer-prompt-test] ok\n'
}

main "$@"
