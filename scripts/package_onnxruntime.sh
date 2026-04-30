#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR=""

log() {
  printf '[thr-runtime-package] %s\n' "$*"
}

fail() {
  printf '[thr-runtime-package] %s\n' "$*" >&2
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

validate_tar_paths() {
  local archive="$1"
  local entry

  while IFS= read -r entry; do
    case "$entry" in
      "" | /* | *"/../"* | "../"* | *"/.." | "..")
        fail "Archive contains an unsafe path: ${entry}"
        ;;
    esac
  done < <(tar -tzf "$archive")
}

safe_extract_tar() {
  local archive="$1"
  local dest="$2"

  validate_tar_paths "$archive"
  mkdir -p "$dest"
  tar -xzf "$archive" -C "$dest"
}

copy_license_files() {
  local source_dir="$1"
  local dest_dir="$2"
  local name

  for name in ${THR_RUNTIME_LICENSE_FILES:-}; do
    if [[ -f "${source_dir}/${name}" ]]; then
      install -m 0644 "${source_dir}/${name}" "${dest_dir}/${name}"
    fi
  done
}

find_source_library() {
  local source_root="$1"
  local source_library_path="$2"
  local found

  found="$(find "$source_root" \( -type f -o -type l \) -path "*/${source_library_path}" | head -n 1)"
  [[ -n "$found" ]] || fail "Could not find ONNX Runtime library path ${source_library_path}"
  printf '%s' "$found"
}

source_license_root() {
  local source_lib="$1"
  local source_library_path="$2"
  local source_root="$3"
  local suffix="/${source_library_path}"

  case "$source_lib" in
    *"$suffix")
      printf '%s' "${source_lib%"$suffix"}"
      ;;
    *)
      printf '%s' "$source_root"
      ;;
  esac
}

prepare_official_source() {
  local source_archive="${TMP_DIR}/source.tgz"
  local source_root="${TMP_DIR}/source"
  local source_sha source_lib license_root

  [[ -n "${THR_SOURCE_URL:-}" ]] || fail "THR_SOURCE_URL is required for official runtime packaging"
  [[ -n "${THR_SOURCE_ARCHIVE_SHA256:-}" ]] || fail "THR_SOURCE_ARCHIVE_SHA256 is required for official runtime packaging"
  [[ -n "${THR_SOURCE_LIBRARY_PATH:-}" ]] || fail "THR_SOURCE_LIBRARY_PATH is required for official runtime packaging"

  log "Downloading ${THR_SOURCE_URL}"
  curl -fsSL "$THR_SOURCE_URL" -o "$source_archive"
  source_sha="$(sha256_file "$source_archive")"
  if [[ "$source_sha" != "$THR_SOURCE_ARCHIVE_SHA256" ]]; then
    fail "Source runtime archive checksum mismatch"
  fi

  safe_extract_tar "$source_archive" "$source_root"
  source_lib="$(find_source_library "$source_root" "$THR_SOURCE_LIBRARY_PATH")"
  license_root="$(source_license_root "$source_lib" "$THR_SOURCE_LIBRARY_PATH" "$source_root")"
  THR_ONNXRUNTIME_LIB="$source_lib"
  THR_ONNXRUNTIME_LICENSE_DIR="$license_root"
}

prepare_source_build() {
  [[ -n "${THR_ONNXRUNTIME_LIB:-}" ]] || fail "THR_ONNXRUNTIME_LIB is required for source-build runtime packaging"
  [[ -f "$THR_ONNXRUNTIME_LIB" ]] || fail "ONNX Runtime library not found: ${THR_ONNXRUNTIME_LIB}"
  [[ -n "${THR_ONNXRUNTIME_LICENSE_DIR:-}" ]] || fail "THR_ONNXRUNTIME_LICENSE_DIR is required for source-build runtime packaging"
}

main() {
  local lock_path="${THR_ONNXRUNTIME_LOCK:-$ROOT_DIR/native/onnxruntime.lock}"
  local out_dir="${THR_PACKAGE_OUT_DIR:-dist-native}"
  local goos="${GOOS:-darwin}"
  local goarch="${GOARCH:-$(go env GOARCH)}"
  local target="${goos}-${goarch}"
  local stage runtime_lib normalized_lib archive library_sha archive_sha
  local -a tar_entries

  need_cmd curl || fail "curl is required"
  need_cmd tar || fail "tar is required"
  need_cmd go || fail "go is required"
  need_cmd shasum || need_cmd sha256sum || fail "shasum or sha256sum is required"

  eval "$(env GOOS= GOARCH= go run "$ROOT_DIR/scripts/release_targets.go" native-env --lock "$lock_path" --target "$target")"

  TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/thr-runtime-package.XXXXXX")"
  stage="${TMP_DIR}/stage"
  mkdir -p "$stage" "$out_dir"

  case "$THR_RUNTIME_SOURCE" in
    official-release-asset)
      prepare_official_source
      ;;
    source-build)
      prepare_source_build
      ;;
    *)
      fail "Unsupported runtime source: ${THR_RUNTIME_SOURCE}"
      ;;
  esac

  normalized_lib="${stage}/${THR_RUNTIME_LIBRARY_PATH}"
  mkdir -p "$(dirname "$normalized_lib")"
  install -m 0755 "$THR_ONNXRUNTIME_LIB" "$normalized_lib"
  copy_license_files "$THR_ONNXRUNTIME_LICENSE_DIR" "$stage"

  runtime_lib="${THR_RUNTIME_LIBRARY_PATH}"
  library_sha="$(sha256_file "$normalized_lib")"
  cat >"${stage}/manifest.json" <<EOF
{
  "schema_version": 1,
  "target": "${target}",
  "onnxruntime": {
    "version": "${THR_ONNXRUNTIME_VERSION}",
    "source": "${THR_RUNTIME_SOURCE}",
    "source_url": "${THR_SOURCE_URL:-}",
    "source_repo": "${THR_SOURCE_REPO:-}",
    "source_tag": "${THR_SOURCE_TAG:-}",
    "library_path": "${runtime_lib}",
    "library_sha256": "${library_sha}"
  }
}
EOF

  find "$stage" -exec touch -t 202001010000 {} +
  archive="${out_dir}/${THR_RUNTIME_ASSET_NAME}"
  tar_entries=(manifest.json lib)
  for name in ${THR_RUNTIME_LICENSE_FILES:-}; do
    if [[ -f "${stage}/${name}" ]]; then
      tar_entries+=("$name")
    fi
  done
  COPYFILE_DISABLE=1 tar -cf - -C "$stage" "${tar_entries[@]}" | gzip -n >"$archive"
  archive_sha="$(sha256_file "$archive")"

  printf '%s  %s\n' "$archive_sha" "$(basename "$archive")" >"${archive}.sha256"
  printf '%s  %s\n' "$library_sha" "$runtime_lib" >"${archive}.lib.sha256"
  log "Wrote ${archive}"
  log "archive_sha256=${archive_sha}"
  log "library_sha256=${library_sha}"
}

main "$@"
