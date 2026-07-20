# Manual verified install

Use this path if you prefer to download, verify, and extract the release archive yourself instead of running:

```bash
curl -fsSL https://raw.githubusercontent.com/Chadi00/thr/master/install.sh | bash
```

The one-line installer performs the same core checks: it downloads the release archive, verifies the OpenSSH signature on `checksums.txt`, checks the archive SHA-256, validates the archive layout, and installs the binary plus packaged runtime files.

## 1. Choose your archive

Download the archive for your OS and architecture from [Releases](https://github.com/Chadi00/thr/releases/latest):

| Platform | Archive |
|----------|---------|
| macOS arm64 | `thr_darwin_arm64.tar.gz` |
| macOS x86_64 | `thr_darwin_amd64.tar.gz` |
| Linux arm64 | `thr_linux_arm64.tar.gz` |
| Linux x86_64 | `thr_linux_amd64.tar.gz` |

Set the archive name:

```bash
archive="thr_darwin_arm64.tar.gz" # change this to your target
```

## 2. Download release assets

```bash
curl -LO "https://github.com/Chadi00/thr/releases/latest/download/${archive}"
curl -LO https://github.com/Chadi00/thr/releases/latest/download/checksums.txt
curl -LO https://github.com/Chadi00/thr/releases/latest/download/checksums.txt.sig
```

## 3. Verify signed checksums

```bash
cat > allowed_signers <<'EOF'
thr-release ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIAXr9HFt+bOkFt6Hx9xC5z/KpwBL0Y5RDonM1eqErPKl thr-release
EOF

ssh-keygen -Y verify \
  -f allowed_signers \
  -I thr-release \
  -n thr-release \
  -s checksums.txt.sig \
  < checksums.txt
```

## 4. Verify the archive checksum

```bash
expected="$(awk -v name="$archive" '$2 == name {print $1}' checksums.txt)"
if command -v sha256sum >/dev/null; then
  actual="$(sha256sum "$archive" | awk '{print $1}')"
else
  actual="$(shasum -a 256 "$archive" | awk '{print $1}')"
fi
test "$actual" = "$expected"
```

## 5. Extract and install

The default install prefix is `~/.local`, matching the installer default.

```bash
tar -xzf "$archive"
mkdir -p ~/.local/bin ~/.local/lib/thr
install -m 0755 bin/thr ~/.local/bin/thr
cp -R lib/thr/. ~/.local/lib/thr/
install -m 0644 manifest.json ~/.local/lib/thr/manifest.json
```

Make sure `~/.local/bin` is on your `PATH`:

```bash
export PATH="$HOME/.local/bin:$PATH"
```

Then check the install:

```bash
thr version
thr prefetch
```

## 6. Migrate existing data

The one-line installer automatically runs `thr migrate` when its selected default database (`THR_DB` or `~/.thr/thr.db`) exists. After a manual install, run it explicitly if you want the upgrade before normal use:

```bash
thr migrate
thr --db /path/to/another.db migrate
```

Otherwise, the first operational access migrates each legacy database automatically, including custom `--db` databases. `thr context` is read-only and only reports `migration_required`.

Migration creates and verifies a private timestamped backup before changing the database. Existing memories keep their IDs and are assigned to `user` with `scope_assignment: legacy_default`. Older binaries cannot use the scoped database; restore the reported backup to downgrade.
