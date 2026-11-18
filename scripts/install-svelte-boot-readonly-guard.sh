#!/usr/bin/env bash
# Emergency host safety patch: replace the previously approved mutating
# Svelte boot unit with a read-only guard, KEEPING IT DISABLED.
# Does not start/stop Docker, recreate containers, alter quotas or credentials.
set -Eeuo pipefail

die() { printf 'SVELTE_BOOT_SAFETY=BLOCKED reason=%s\n' "$*" >&2; exit 1; }
[[ "${EUID}" -eq 0 ]] || die "root-required"
[[ "$#" -eq 1 ]] || die "usage: $0 /opt/anyflip-downloader"
host="$(cd "$1" && pwd -P)"
[[ "$host" == "/opt/anyflip-downloader" ]] || die "unexpected-host"
src="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
[[ "$src" != "$host" ]] || die "must-use-detached-release-worktree"
unit="/etc/systemd/system/rcat-anyflip-svelte-boot.service"
new="$src/deploy/systemd/rcat-anyflip-svelte-boot.service"
[[ -f "$unit" && -f "$new" ]] || die "systemd-unit-missing"
for cmd in systemctl git cmp install; do
  command -v "$cmd" >/dev/null 2>&1 || die "missing-$cmd"
done
[[ -z "$(git -C "$src" status --porcelain --untracked-files=no -- deploy/systemd/rcat-anyflip-svelte-boot.service scripts/install-svelte-boot-readonly-guard.sh)" ]] ||
  die "unreviewed-release-changes"
grep -Fq 'ExecStart=/usr/local/libexec/rcat-anyflip-svelte-boot.sh check' "$new" ||
  die "new-unit-not-readonly"
if grep -Fq 'ExecStart=/usr/local/libexec/rcat-anyflip-svelte-boot.sh ensure' "$new"; then
  die "new-unit-unsafe"
fi

# Only replace the exact old reviewed file (PR #137/#138), or the exact
# new reviewed file during a repeated installation. Never overwrite custom
# Host systemd changes. Git history is local; no network needed.
if ! cmp -s "$unit" "$new"; then
  cmp -s "$unit" <(git -C "$src" show \
    7b3e8213fd9ff1ad934573b0e998d9ba4e45aedf:deploy/systemd/rcat-anyflip-svelte-boot.service) ||
    die "existing-unit-has-unreviewed-changes"
fi

# Disable the old force-recreate unit; this does not touch Docker or its jobs.
systemctl disable --now rcat-anyflip-svelte-boot.service
install -m 0644 "$new" "$unit"
systemctl daemon-reload
# systemd may have already unloaded this disabled unit, so there is no failed
# state to clear. This cleanup is optional; the required disabled/read-only
# assertions below still fail closed.
systemctl reset-failed rcat-anyflip-svelte-boot.service 2>/dev/null || true
enabled="$(systemctl is-enabled rcat-anyflip-svelte-boot.service || true)"
[[ "$enabled" == disabled ]] || die "unit-must-remain-disabled"
grep -Fq 'ExecStart=/usr/local/libexec/rcat-anyflip-svelte-boot.sh check' "$unit" ||
  die "installed-unit-not-readonly"
echo "SVELTE_BOOT_SAFETY=PASS"
echo "SVELTE_BOOT_UNIT=disabled"
echo "SVELTE_BOOT_EXEC=read-only-check"
echo "DOCKER_CONTAINER_CHANGES=none"
echo "BOOT_SOURCE_OF_TRUTH=PENDING_QUOTA_UNIT_REVIEW"
