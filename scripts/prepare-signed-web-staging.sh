#!/bin/sh
# Prepare a private Docker build context from a preverified AnyFlip Linux binary.
# Does not build, start, update, deploy, or connect to any RCAT Host.
set -eu

if [ "$#" -ne 3 ]; then
  echo 'Usage: sh prepare-signed-web-staging.sh <linux-amd64-binary> <40hex-source-sha> <sha256:64hex>' >&2
  exit 2
fi
binary=$1
source_sha=$2
expected_digest=$3
case "$(basename -- "$binary")" in
  anyflip-downloader_v*_linux_amd64) ;;
  *) echo 'REFUSED: expected signed Linux AMD64 Web/CLI binary' >&2; exit 2 ;;
esac
if ! printf '%s\n' "$source_sha" | LC_ALL=C grep -Eq '^[0-9a-f]{40}$'; then
  echo 'REFUSED: exact immutable source SHA required' >&2
  exit 2
fi
if ! printf '%s\n' "$expected_digest" | LC_ALL=C grep -Eq '^sha256:[0-9a-f]{64}$'; then
  echo 'REFUSED: exact binary SHA-256 required' >&2
  exit 2
fi
[ -f "$binary" ] || { echo 'REFUSED: binary missing' >&2; exit 2; }
command -v gh >/dev/null 2>&1 || { echo 'REFUSED: GitHub attestation verifier unavailable' >&2; exit 2; }
command -v sha256sum >/dev/null 2>&1 || { echo 'REFUSED: sha256sum unavailable' >&2; exit 2; }
tmp_root=$(printenv TMPDIR 2>/dev/null || true)
[ -n "$tmp_root" ] || tmp_root=/tmp
context=$(mktemp -d "$tmp_root/anyflip-web-stage.XXXXXXXX")
cleanup() { if [ -n "$context" ]; then rm -rf -- "$context"; fi; }
trap cleanup EXIT HUP INT TERM
chmod 0700 "$context"
# Freeze the candidate in a private context BEFORE attestation to prevent
# validating one set of bytes then copying a changed source path.
install -m 0555 "$binary" "$context/anyflip-downloader"
actual_digest=$(sha256sum "$context/anyflip-downloader" | cut -d ' ' -f 1)
if [ "sha256:$actual_digest" != "$expected_digest" ]; then
  echo 'REFUSED: candidate bytes do not match the reviewed SHA-256' >&2
  exit 2
fi
# Authenticate the exact frozen bytes against the trusted source identity.
gh attestation verify "$context/anyflip-downloader" \
  --repo yiwssx/anyflip-downloader \
  --signer-workflow yiwssx/anyflip-downloader/.github/workflows/release.yml \
  --source-ref refs/heads/main \
  --source-digest "$source_sha" >/dev/null || {
    echo 'REFUSED: GitHub source-bound attestation not verified' >&2
    exit 2
  }
printf '%s\n' "$source_sha" > "$context/SOURCE_COMMIT"
printf '%s\n' "$expected_digest" > "$context/BINARY_SHA256"
# Only return an isolated build context. Docker/image operations require
# a separate authorized Staging executor; Production is never enabled here.
printf '%s\n' "$context"
context=
