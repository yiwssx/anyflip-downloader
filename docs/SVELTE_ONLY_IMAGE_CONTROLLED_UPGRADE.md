# RCAT AnyFlip — controlled production Svelte-only image upgrade

## Entry conditions

The operator has already completed the **permanent source-of-truth repair** in PRs #140–#141 and shown real Host output:

```text
SVELTE_BOOT_SAFETY=PASS
SVELTE_BOOT_UNIT=disabled
QUOTA_SVELTE_COMPOSE=PASS
QUOTA_SVELTE_STARTUP=CONFIGURED
QUOTA_SVELTE_APPLY=PASS
SUI05_SVELTE_PRIMARY=PASS
PINNED_SOURCE=36b2539f1aaaf1eb0acb8fe04d800577a9a3a81d
QUOTA_SVELTE_REBOOT_PROOF=PENDING_NORMAL_REBOOT
```

Production is **already visibly Svelte**. This last optional release step replaces the pre-policy image with a binary built from PR #142 that cannot load or serve the retired HTML/JS/CSS frontend even if `ANYFLIP_WEB_UI` were unset.

Use the existing `anyflip-quota.service` as the **only owner of application stop/start**. Leave `rcat-anyflip-svelte-boot.service` **disabled** permanently. Do not run `scripts/deploy-svelte-ui.sh` while the quota unit owns the runtime: that older cutover script uses a separate Compose overlay and automatic force-recreate/rollback that can race the systemd quota owner.

## Phase 1 — prepare immutable new image without service interruption

Create one clean detached worktree at the **accepted main merge SHA** containing this procedure; do not reset the existing Host checkout or copy over its private `.env` and quota overlay.

```bash
sudo git -C /opt/anyflip-downloader fetch --no-tags origin main
RELEASE="/opt/anyflip-svelte-only-release-$(date -u +%Y%m%dT%H%M%SZ)"
SOURCE_SHA=REPLACE_WITH_THIS_PR_ACCEPTED_MERGE_SHA
sudo git -C /opt/anyflip-downloader worktree add --detach "$RELEASE" "$SOURCE_SHA"
sudo bash "$RELEASE/scripts/upgrade-svelte-only-image.sh" \
  /opt/anyflip-downloader --prepare
```

The script verifies the production container is currently healthy, with approved Google auth, correct source-pinned image, quota mount identity, LAN bind and disabled competing boot recovery; then builds the **new** Svelte-only Docker image from the approved source with `Dockerfile.svelte`. It checks OCI revision and the `th.ac.rcat.anyflip.frontend=svelte-only` label, pins a local immutable `rcat-anyflip-svelte:boot-<source>-<image>` tag and stores a private candidate receipt under `/etc/rcat-anyflip-svelte`.

Expected: `SVELTE_ONLY_PREPARE=PASS`, `DOCKER_CONTAINERS_CHANGED=no`. **No running container, systemd unit or persistent volume changes in Phase 1**. Source build outputs and image tag exist only locally on Host. GitHub CI proves code behavior, not Host build success.

## Phase 2 — one controlled cutover, only after all downloads finish

```bash
sudo bash "$RELEASE/scripts/upgrade-svelte-only-image.sh" \
  /opt/anyflip-downloader --apply --jobs-idle
```

The `--jobs-idle` flag is the operator's **explicit acknowledgment** that there are no active downloads; the script does not have Google user credentials to inspect private jobs.

The script checks the candidate tag/immutable ID/revision/frontend label and verifies effective merged Compose preserves Google auth, LAN port and jobs volume. It then:

1. Makes a private **root-only backup** of the exact old `active.env` and `90-svelte-pinned.conf`.
2. Updates only `BOOT_IMAGE_ID`, `BOOT_IMAGE_TAG`, `BOOT_SOURCE_SHA` in the pinned receipt, and the existing quota unit's `RCAT_SVELTE_IMAGE` drop-in environment.
3. Calls `systemctl daemon-reload` and performs **exactly one** `systemctl restart anyflip-quota.service`. That service handles the existing mounted-quota lifecycle; `docker compose up --no-build` detects the changed tagged image.
4. Checks actual container image identity, Docker health, `ANYFLIP_WEB_UI=svelte`, Google auth, exact prior mount and port, and HTTP/CSP/anonymous API denial using the accepted smoke checks.

Expected: `SVELTE_ONLY_UPGRADE=PASS`, `SVELTE_ONLY_PRODUCTION_BINARY=PASS`, new source SHA, and `REBOOT_PROOF=PENDING_NEXT_NORMAL_REBOOT`.

**No** docker engine restart, name-force-recreate, `docker compose down`, volume delete, use of alternative project or re-enabling the competing boot service. The previous image remains available, and its root-only configuration backup path is reported. If the cutover fails, stop; review `journalctl -u anyflip-quota.service -b` and `docker ps -a` before any explicit manual rollback. Never repeat cutover blindly while container names or running jobs may be in transition.

## Important exclusions

- Does **not** delete legacy frontend historical files in Git or erase history. Those remain for plain Go/CLI tests, while the production image excludes and refuses them.
- Does **not** establish successful reboot behavior until the next **normally scheduled** server reboot. The separate `containerd.service` timeout that delayed all RCAT Docker services on 2026-11-18 is unrelated and still needs independent analysis.
- Does **not** change PDF HUB, POS, Radio, production Tunnel/DNS, Google OAuth secrets, RBAC, or AnyFlip mount/volume.
