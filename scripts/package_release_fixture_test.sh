#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WORK_DIR="$(mktemp -d "${TMPDIR:-/tmp}/thr-package-test.XXXXXX")"

cleanup() {
  rm -rf "$WORK_DIR"
}

trap cleanup EXIT

fail() {
  printf '[package-release-fixture-test] %s\n' "$*" >&2
  exit 1
}

sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

create_stub_binary() {
  local path="$1"

  cat >"$path" <<'EOF'
#!/usr/bin/env bash
printf 'thr fixture\n'
EOF
  chmod +x "$path"
}

create_runtime_asset() {
  local stage="$WORK_DIR/runtime-stage"
  local archive="$WORK_DIR/thr-onnxruntime_1.25.1_darwin_arm64.tar.gz"

  mkdir -p "$stage/lib"
  printf 'fixture runtime\n' >"$stage/lib/libonnxruntime.dylib"
  printf 'fixture license\n' >"$stage/LICENSE"
  cat >"$stage/manifest.json" <<'EOF'
{"schema_version":1,"target":"darwin-arm64"}
EOF
  tar -czf "$archive" -C "$stage" manifest.json lib LICENSE
  printf '%s' "$archive"
}

write_lock() {
  local archive="$1"
  local archive_sha="$2"
  local lib_sha="$3"
  local lock="$WORK_DIR/onnxruntime.lock"

  cat >"$lock" <<EOF
{
  "schema_version": 2,
  "onnxruntime_version": "1.25.1",
  "native_release_tag": "thr-native-onnxruntime-v1.25.1",
  "targets": [
    {
      "target": "darwin-arm64",
      "status": "shipping",
      "os": "darwin",
      "arch": "arm64",
      "runner": "macos-latest",
      "installer": "unix",
      "source": "official-release-asset",
      "source_url": "https://example.invalid/source.tgz",
      "source_archive_sha256": "source-sha",
      "source_library_path": "lib/libonnxruntime.dylib",
      "runtime_asset_name": "$(basename "$archive")",
      "runtime_asset_url": "file://${archive}",
      "runtime_archive_sha256": "${archive_sha}",
      "runtime_library_path": "lib/libonnxruntime.dylib",
      "runtime_library_sha256": "${lib_sha}",
      "license_files": ["LICENSE"]
    }
  ]
}
EOF
  printf '%s' "$lock"
}

assert_archive_contains() {
  local archive="$1"
  local entry="$2"

  if ! tar -tzf "$archive" | grep -qxF "$entry"; then
    fail "expected ${archive} to contain ${entry}"
  fi
}

main() {
  local runtime_archive runtime_archive_sha runtime_lib_sha lock binary out_dir product_archive

  runtime_archive="$(create_runtime_asset)"
  runtime_archive_sha="$(sha256_file "$runtime_archive")"
  runtime_lib_sha="$(sha256_file "$WORK_DIR/runtime-stage/lib/libonnxruntime.dylib")"
  lock="$(write_lock "$runtime_archive" "$runtime_archive_sha" "$runtime_lib_sha")"
  binary="$WORK_DIR/thr"
  out_dir="$WORK_DIR/dist"
  create_stub_binary "$binary"

  THR_ONNXRUNTIME_LOCK="$lock" \
    THR_PACKAGE_BINARY="$binary" \
    THR_PACKAGE_OUT_DIR="$out_dir" \
    GOOS=darwin \
    GOARCH=arm64 \
    bash "$ROOT_DIR/scripts/package_release.sh" >/dev/null

  product_archive="$out_dir/thr_darwin_arm64.tar.gz"
  [[ -f "$product_archive" ]] || fail "product archive was not created"
  assert_archive_contains "$product_archive" "bin/thr"
  assert_archive_contains "$product_archive" "manifest.json"
  assert_archive_contains "$product_archive" "lib/thr/onnxruntime/1.25.1/darwin-arm64/libonnxruntime.dylib"

  lock="$(write_lock "$runtime_archive" "bad-sha" "$runtime_lib_sha")"
  if THR_ONNXRUNTIME_LOCK="$lock" THR_PACKAGE_BINARY="$binary" THR_PACKAGE_OUT_DIR="$out_dir/bad" GOOS=darwin GOARCH=arm64 bash "$ROOT_DIR/scripts/package_release.sh" >/dev/null 2>&1; then
    fail "expected packaging to reject tampered runtime archive checksum"
  fi

  printf '[package-release-fixture-test] ok\n'
}

main "$@"
