#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ASSET_DIR="${ROOT_DIR}/internal/embed/model_assets"
TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/thr-model-assets.XXXXXX")"

MODEL_ID="Qdrant/bge-base-en-v1.5-onnx-Q"
MODEL_REVISION="738cad1c108e2f23649db9e44b2eab988626493b"
CHUNK_SIZE="45m"

cleanup() {
  rm -rf "$TMP_DIR"
}

trap cleanup EXIT

log() {
  printf '[thr-model-assets] %s\n' "$*"
}

need_cmd() {
  command -v "$1" >/dev/null 2>&1
}

sha256_file() {
  if need_cmd sha256sum; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

expected_sha256() {
  case "$1" in
    config.json) printf '%s' '86f84a5285de7f1ee673f712387219ef1e261ec27dcd870e793a80f9da1aaa3b' ;;
    model_optimized.onnx) printf '%s' '4e556722bc4f65716c544c8a931f1e90fb3f866e5741fd93a96f051d673339c7' ;;
    special_tokens_map.json) printf '%s' '5d5b662e421ea9fac075174bb0688ee0d9431699900b90662acd44b2a350503a' ;;
    tokenizer.json) printf '%s' 'd241a60d5e8f04cc1b2b3e9ef7a4921b27bf526d9f6050ab90f9267a1f9e5c66' ;;
    tokenizer_config.json) printf '%s' '0b29c7bfc889e53b36d9dd3e686dd4300f6525110eaa98c76a5dafceb2029f53' ;;
    vocab.txt) printf '%s' '07eced375cec144d27c900241f3e339478dec958f92fddbc551f295c992038a3' ;;
    *)
      printf 'unknown model file: %s\n' "$1" >&2
      return 1
      ;;
  esac
}

download_and_verify() {
  local name="$1"
  local url="https://huggingface.co/${MODEL_ID}/resolve/${MODEL_REVISION}/${name}"
  local path="${TMP_DIR}/${name}"
  local expected actual

  log "Downloading ${name}"
  curl -fL --retry 3 --retry-delay 2 "$url" -o "$path"

  expected="$(expected_sha256 "$name")"
  actual="$(sha256_file "$path")"
  if [[ "$actual" != "$expected" ]]; then
    printf 'checksum mismatch for %s: got %s want %s\n' "$name" "$actual" "$expected" >&2
    return 1
  fi
}

main() {
  local name assembled expected actual
  local files=(
    config.json
    model_optimized.onnx
    special_tokens_map.json
    tokenizer.json
    tokenizer_config.json
    vocab.txt
  )

  need_cmd curl || {
    printf 'curl is required\n' >&2
    return 1
  }
  need_cmd split || {
    printf 'split is required\n' >&2
    return 1
  }

  for name in "${files[@]}"; do
    download_and_verify "$name"
  done

  rm -rf "$ASSET_DIR"
  mkdir -p "$ASSET_DIR"

  for name in "${files[@]}"; do
    if [[ "$name" == "model_optimized.onnx" ]]; then
      continue
    fi
    install -m 0644 "${TMP_DIR}/${name}" "${ASSET_DIR}/${name}"
  done

  split -d -b "$CHUNK_SIZE" -a 3 "${TMP_DIR}/model_optimized.onnx" "${ASSET_DIR}/model_optimized.onnx.part-"
  chmod 0644 "${ASSET_DIR}"/model_optimized.onnx.part-*

  assembled="${TMP_DIR}/model_optimized.onnx.assembled"
  cat "${ASSET_DIR}"/model_optimized.onnx.part-* >"$assembled"
  expected="$(expected_sha256 model_optimized.onnx)"
  actual="$(sha256_file "$assembled")"
  if [[ "$actual" != "$expected" ]]; then
    printf 'assembled ONNX checksum mismatch: got %s want %s\n' "$actual" "$expected" >&2
    return 1
  fi

  log "Wrote bundled model assets to ${ASSET_DIR}"
}

main "$@"
