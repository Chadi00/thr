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

runtime_library_name() {
  case "$1" in
    darwin) printf '%s' 'libonnxruntime.dylib' ;;
    linux) printf '%s' 'libonnxruntime.so' ;;
    *) fail "unsupported fixture os: $1" ;;
  esac
}

create_runtime_asset() {
  local os="$1"
  local arch="$2"
  local target="${os}-${arch}"
  local lib_name
  local stage="$WORK_DIR/runtime-stage-${target}"
  local archive="$WORK_DIR/thr-onnxruntime_1.25.1_${os}_${arch}.tar.gz"

  lib_name="$(runtime_library_name "$os")"
  mkdir -p "$stage/lib"
  printf 'fixture runtime for %s\n' "$target" >"$stage/lib/$lib_name"
  printf 'fixture license\n' >"$stage/LICENSE"
  cat >"$stage/manifest.json" <<EOF
{"schema_version":1,"target":"${target}"}
EOF
  tar -czf "$archive" -C "$stage" manifest.json lib LICENSE
  printf '%s' "$archive"
}

write_lock() {
  local os="$1"
  local arch="$2"
  local archive="$3"
  local archive_sha="$4"
  local lib_sha="$5"
  local lock="$WORK_DIR/onnxruntime-${os}-${arch}.lock"
  local target="${os}-${arch}"
  local lib_name runner

  lib_name="$(runtime_library_name "$os")"
  case "$os" in
    darwin) runner="macos-latest" ;;
    linux) runner="ubuntu-latest" ;;
    *) fail "unsupported fixture os: $os" ;;
  esac

  cat >"$lock" <<EOF
{
  "schema_version": 2,
  "onnxruntime_version": "1.25.1",
  "native_release_tag": "thr-native-onnxruntime-v1.25.1",
  "targets": [
    {
      "target": "${target}",
      "status": "shipping",
      "os": "${os}",
      "arch": "${arch}",
      "runner": "${runner}",
      "installer": "unix",
      "source": "official-release-asset",
      "source_url": "https://example.invalid/source.tgz",
      "source_archive_sha256": "source-sha",
      "source_library_path": "lib/${lib_name}",
      "runtime_asset_name": "$(basename "$archive")",
      "runtime_asset_url": "file://${archive}",
      "runtime_archive_sha256": "${archive_sha}",
      "runtime_library_path": "lib/${lib_name}",
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

assert_packaged_target() {
  local os="$1"
  local arch="$2"
  local expected_lib="$3"
  local runtime_archive runtime_archive_sha runtime_lib_sha lock out_dir product_archive

  runtime_archive="$(create_runtime_asset "$os" "$arch")"
  runtime_archive_sha="$(sha256_file "$runtime_archive")"
  runtime_lib_sha="$(sha256_file "$WORK_DIR/runtime-stage-${os}-${arch}/lib/$expected_lib")"
  lock="$(write_lock "$os" "$arch" "$runtime_archive" "$runtime_archive_sha" "$runtime_lib_sha")"
  out_dir="$WORK_DIR/dist-${os}-${arch}"

  THR_ONNXRUNTIME_LOCK="$lock" \
    THR_PACKAGE_BINARY="$binary" \
    THR_PACKAGE_OUT_DIR="$out_dir" \
    GOOS="$os" \
    GOARCH="$arch" \
    bash "$ROOT_DIR/scripts/package_release.sh" >/dev/null

  product_archive="$out_dir/thr_${os}_${arch}.tar.gz"
  [[ -f "$product_archive" ]] || fail "product archive was not created for ${os}-${arch}"
  assert_archive_contains "$product_archive" "bin/thr"
  assert_archive_contains "$product_archive" "manifest.json"
  assert_archive_contains "$product_archive" "lib/thr/onnxruntime/1.25.1/${os}-${arch}/${expected_lib}"
}

main() {
  local runtime_archive runtime_lib_sha lock binary

  binary="$WORK_DIR/thr"
  create_stub_binary "$binary"

  assert_packaged_target darwin arm64 libonnxruntime.dylib
  assert_packaged_target linux amd64 libonnxruntime.so

  runtime_archive="$(create_runtime_asset darwin arm64)"
  runtime_lib_sha="$(sha256_file "$WORK_DIR/runtime-stage-darwin-arm64/lib/libonnxruntime.dylib")"

  lock="$(write_lock darwin arm64 "$runtime_archive" "bad-sha" "$runtime_lib_sha")"
  if THR_ONNXRUNTIME_LOCK="$lock" THR_PACKAGE_BINARY="$binary" THR_PACKAGE_OUT_DIR="$WORK_DIR/dist-bad" GOOS=darwin GOARCH=arm64 bash "$ROOT_DIR/scripts/package_release.sh" >/dev/null 2>&1; then
    fail "expected packaging to reject tampered runtime archive checksum"
  fi

  printf '[package-release-fixture-test] ok\n'
}

main "$@"
