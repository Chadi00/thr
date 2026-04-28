#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ONNXRUNTIME_VERSION="1.25.1"
ONNXRUNTIME_OSX_ARM64_SHA256="18987ec3187b5f29ba798109750f6135060560ad4e0a52678fcc753ee8fb3091"
TMP_DIR=""

log() {
  printf '[thr-package] %s\n' "$*"
}

fail() {
  printf '[thr-package] %s\n' "$*" >&2
  exit 1
}

cleanup() {
  if [[ -n "$TMP_DIR" ]]; then
    rm -rf "$TMP_DIR"
  fi
}

trap cleanup EXIT

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

runtime_library_name() {
  case "${GOOS:-darwin}" in
    darwin) printf '%s' 'libonnxruntime.dylib' ;;
    windows) printf '%s' 'onnxruntime.dll' ;;
    *) printf '%s' 'onnxruntime.so' ;;
  esac
}

download_official_onnxruntime() {
  local target="$1"
  local archive archive_sha expected_sha url extracted

  case "$target" in
    darwin-arm64)
      archive="onnxruntime-osx-arm64-${ONNXRUNTIME_VERSION}.tgz"
      expected_sha="$ONNXRUNTIME_OSX_ARM64_SHA256"
      ;;
    *)
      return 1
      ;;
  esac

  url="https://github.com/microsoft/onnxruntime/releases/download/v${ONNXRUNTIME_VERSION}/${archive}"
  log "Downloading ${archive}"
  curl -fsSL "$url" -o "${TMP_DIR}/${archive}"
  archive_sha="$(sha256_file "${TMP_DIR}/${archive}")"
  if [[ "$archive_sha" != "$expected_sha" ]]; then
    fail "ONNX Runtime archive checksum mismatch for ${archive}"
  fi

  tar -xzf "${TMP_DIR}/${archive}" -C "$TMP_DIR"
  extracted="${TMP_DIR}/${archive%.tgz}"
  THR_ONNXRUNTIME_LIB="${extracted}/lib/$(runtime_library_name)"
  THR_ONNXRUNTIME_LICENSE_DIR="$extracted"
}

copy_license_if_present() {
  local source_dir="$1"
  local dest_dir="$2"
  local name

  for name in LICENSE ThirdPartyNotices.txt Privacy.md VERSION_NUMBER GIT_COMMIT_ID; do
    if [[ -f "${source_dir}/${name}" ]]; then
      install -m 0644 "${source_dir}/${name}" "${dest_dir}/${name}"
    fi
  done
}

model_const() {
  local name="$1"

  awk -F '"' -v name="$name" '$0 ~ name { print $2; exit }' "$ROOT_DIR/internal/embed/bge.go"
}

model_manifest_sha256() {
  local model_id="$1"
  local model_revision="$2"
  local model_dimension="$3"

  {
    printf 'model_id=%s\nrevision=%s\ndimension=%s\n' "$model_id" "$model_revision" "$model_dimension"
    awk -F '"' '/Name: / { print $2 "=" $4 }' "$ROOT_DIR/internal/embed/bge.go" | sort
  } | shasum -a 256 | awk '{print $1}'
}

main() {
  local binary="${THR_PACKAGE_BINARY:-thr}"
  local out_dir="${THR_PACKAGE_OUT_DIR:-dist}"
  local version="${THR_RELEASE_VERSION:-dev}"
  local commit="${THR_RELEASE_COMMIT:-unknown}"
  local goos="${GOOS:-darwin}"
  local goarch="${GOARCH:-$(go env GOARCH)}"
  local target="${goos}-${goarch}"
  local archive="thr_${goos}_${goarch}.tar.gz"
  local stage runtime_dir runtime_lib rel_runtime_lib thr_sha runtime_sha
  local runtime_source="${THR_ONNXRUNTIME_SOURCE:-official}"
  local model_id model_revision model_manifest_sha model_dimension

  need_cmd tar || fail "tar is required"
  need_cmd shasum || need_cmd sha256sum || fail "shasum or sha256sum is required"
  [[ -x "$binary" ]] || fail "thr binary not found or not executable: ${binary}"

  TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/thr-package.XXXXXX")"
  stage="${TMP_DIR}/stage"
  runtime_dir="${stage}/lib/thr/onnxruntime/${ONNXRUNTIME_VERSION}/${target}"
  mkdir -p "${stage}/bin" "$runtime_dir" "$out_dir"

  install -m 0755 "$binary" "${stage}/bin/thr"

  if [[ -z "${THR_ONNXRUNTIME_LIB:-}" ]]; then
    download_official_onnxruntime "$target" || fail "No official ONNX Runtime asset configured for ${target}; set THR_ONNXRUNTIME_LIB to a CI-built library"
  fi
  [[ -f "$THR_ONNXRUNTIME_LIB" ]] || fail "ONNX Runtime library not found: ${THR_ONNXRUNTIME_LIB}"
  runtime_lib="${runtime_dir}/$(runtime_library_name)"
  install -m 0755 "$THR_ONNXRUNTIME_LIB" "$runtime_lib"
  if [[ -n "${THR_ONNXRUNTIME_LICENSE_DIR:-}" ]]; then
    copy_license_if_present "$THR_ONNXRUNTIME_LICENSE_DIR" "$runtime_dir"
  fi

  rel_runtime_lib="lib/thr/onnxruntime/${ONNXRUNTIME_VERSION}/${target}/$(runtime_library_name)"
  thr_sha="$(sha256_file "${stage}/bin/thr")"
  runtime_sha="$(sha256_file "$runtime_lib")"
  model_id="$(model_const ActiveModelID)"
  model_revision="$(model_const ActiveModelRevision)"
  model_dimension="$(awk '/ActiveModelDimension/ { print $3; exit }' "$ROOT_DIR/internal/embed/bge.go")"
  model_manifest_sha="$(model_manifest_sha256 "$model_id" "$model_revision" "$model_dimension")"

  cat >"${stage}/manifest.json" <<EOF
{
  "schema_version": 1,
  "target": "${target}",
  "thr": {
    "version": "${version}",
    "commit": "${commit}",
    "path": "bin/thr",
    "sha256": "${thr_sha}"
  },
  "onnxruntime": {
    "version": "${ONNXRUNTIME_VERSION}",
    "source": "${runtime_source}",
    "library_path": "${rel_runtime_lib}",
    "library_sha256": "${runtime_sha}"
  },
  "model": {
    "model_id": "${model_id}",
    "model_revision": "${model_revision}",
    "manifest_sha256": "${model_manifest_sha}",
    "dimension": ${model_dimension}
  }
}
EOF

  tar -czf "${out_dir}/${archive}" -C "$stage" bin lib manifest.json
  log "Wrote ${out_dir}/${archive}"
}

main "$@"
