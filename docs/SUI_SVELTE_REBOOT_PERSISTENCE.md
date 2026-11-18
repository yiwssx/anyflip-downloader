# Svelte UI persists after Ubuntu reboot (no automatic legacy fallback)

**Scope:** Fix the **runtime boot configuration drift** that can restore the Vanilla/legacy UI after the RCAT Ubuntu Host reboot. The operator reported the Svelte UI working again after a redeploy, but there is **no original host boot journal here** to prove which service recreated it. This is a preventive repair, not a claim that the original cause is forensically established.

## Why a reboot may have exposed the legacy UI

- The original Go server intentionally defaults to `ANYFLIP_WEB_UI=legacy` when no frontend mode is supplied, and the original `compose.web.yaml` builds the **legacy Dockerfile**.
- The Svelte cutover deployed with an extra `compose.svelte.yaml`; an old quota/host startup command using only base + quota Compose can recreate a legacy container.
- A plain `docker restart` or daemon reboot **does not normally swap the image/Env** of an existing container. Look for a Compose `up --build`, `up --force-recreate`, a systemd startup script, or an image replacement instead.
- The Host's `anyflip-quota.service` and `compose.quota.yaml` are operator-owned and not committed to the repository. They must not be overwritten or inferred.

## Durable design

The operator installs **one** small root-only, `systemd` oneshot service **after** confirming the actual currently working production Svelte UI. This creates:

- `/etc/rcat-anyflip-svelte/active.env` (0600, metadata only: original immutable image ID, source SHA, Compose project, mount identity and bind).
- `/etc/rcat-anyflip-svelte/compose.pinned.yaml` (0600, immutable-local-image reference with `build: !reset null`, `restart: "no"`, `ANYFLIP_WEB_UI: svelte`).
- Local image tag `rcat-anyflip-svelte:boot-<source>-<image>` pointing to the **existing running image**; no download, rebuild or pull.
- `/usr/local/libexec/rcat-anyflip-svelte-boot.sh` and a local copy of the previously accepted `verify-svelte-ui.sh`.
- `rcat-anyflip-svelte-boot.service`, enabled at `multi-user.target`, with `Requires/After=docker.service anyflip-quota.service`.

During installation the script **does not restart the application**. It does set the running container's Docker restart policy to `no` if required: the quota-aware systemd unit (not the bare Docker daemon) is responsible for safe boot ordering.

At the next boot:

- **If the expected Svelte image is running and Healthy:** read-only health/auth/CSP checks, report `SVELTE_BOOT_ACTION=NOOP`, no recreation.
- **If quota-aware boot recreated the old image or stopped the app:** compare the exact mounted storage, LAN bind, project, Google mode and allowlisted local pinned image; then recreate only `anyflip-web` using **base + quota + pinned** Compose with `--no-build --no-deps`. Smoke-check HTTP, assets, auth denial, CSP and Docker health.
- **If a mount, project, bind, source image or Google authentication is unexpected:** fail closed, do not auto-recreate. Reconciliation failures attempt to restore the previous container image and UI mode, retaining the named job volume and existing OAuth environment.

This does **not** run a background watcher, change the Go server's fallback semantics for existing CLI/CI consumers, modify the quota service, DNS/Tunnel, OAuth credentials, or add a second Docker project.

## One-time Host installation (after Svelte is confirmed healthy)

The source below is only used as an installer. It pins the **actual existing container image** rather than assuming the Git checkout is the running image.

```bash
sudo git -C /opt/anyflip-downloader fetch --no-tags origin main
BOOT_SHA="$(sudo git -C /opt/anyflip-downloader rev-parse origin/main)"
BOOT_RELEASE="/opt/anyflip-boot-guard-$(date -u +%Y%m%dT%H%M%SZ)"
sudo git -C /opt/anyflip-downloader worktree add --detach "$BOOT_RELEASE" "$BOOT_SHA"
sudo bash "$BOOT_RELEASE/scripts/install-svelte-boot.sh" /opt/anyflip-downloader
```

Expected:

```text
SVELTE_BOOT_INSTALL=PASS
SVELTE_BOOT_UNIT=enabled
SVELTE_BOOT_CONTAINER_RESTARTED=no
SVELTE_BOOT_REBOOT_PROOF=PENDING_OPERATOR
```

If the existing release is still legacy, the quota unit is inactive, or the mounted job volume/port differs, the installer refuses and makes no runtime recreate. **Do not bypass the preflight by editing this script or changing Docker mounts.** The only allowed image is a previously verified Svelte image with an exact source revision label.

Read-only check afterward:

```bash
sudo systemctl is-enabled rcat-anyflip-svelte-boot.service
sudo /usr/local/libexec/rcat-anyflip-svelte-boot.sh check
sudo systemctl cat rcat-anyflip-svelte-boot.service
```

Expected `SVELTE_BOOT=PASS` and `SVELTE_BOOT_ACTION=NOOP`. **Do not reboot solely for a proof test while downloads are active.** At the next operator-scheduled reboot, check:

```bash
sudo systemctl status rcat-anyflip-svelte-boot.service --no-pager
sudo journalctl -u rcat-anyflip-svelte-boot.service -b --no-pager -n 50
sudo /usr/local/libexec/rcat-anyflip-svelte-boot.sh check
```

On failure do **not** run `docker compose down -v`, reset the checkout, delete a volume, unmount the quota, or switch off Google/RBAC. Review the classified `SVELTE_BOOT=BLOCKED reason=...` and inspect the operator-owned startup units privately.

## Future application deployments

An intentional new Svelte image release must be **re-pinned after it is deployed and accepted**, otherwise this boot service will restore the older pinned Svelte image at the next boot:

```bash
sudo bash "$NEW_APPROVED_RELEASE/scripts/install-svelte-boot.sh" /opt/anyflip-downloader --re-pin
```

`--re-pin` refuses unknown/edit-modified service files and only accepts the **current healthy Svelte** image; it does not build or restart the running app. Use an authorized maintenance window for the underlying image deployment. No new systemd boot service should be added for each release.

## Acceptance / limitations

GitHub CI validates shell scripts, the shared Compose template with real Docker Compose merge semantics, and existing Go/Docker checks. **Only Host observations** can prove that the existing custom quota unit runs before the boot recovery and that no unknown third-party startup unit subsequently recreates the app. Keep the owner/operator's reboot journal as the final evidence. The new guard does not watch for accidental manual Compose re-deploys while the Host is continuously running; use the quota-aware pinned Compose command for every authorized recreation.


## Recovery from the 2026-11-18 partial installer run

The first Host execution of `scripts/install-svelte-boot.sh` failed during its installed `check` step with:

```text
/usr/local/libexec/rcat-anyflip-svelte-boot.sh: line 68: --filter: command not found
SVELTE_BOOT=BLOCKED reason=expected-one-existing-container
SVELTE_BOOT_INSTALL=BLOCKED reason=installed-configuration-did-not-verify
```

**Root cause:** the repository's `reconcile-svelte-boot.sh` separated `docker ps -aq` from its `--filter` arguments with an unescaped newline inside a process substitution. Shell parsing ran `--filter` as another command. This was a coding bug; it is not evidence of an OAuth, Quota, Svelte or Docker crash.

The failed installer wrote a private `active.env`, a pinned Compose overlay and a systemd *unit file* before the check, but **had not yet called `systemctl enable` or `docker update --restart=no`**. The running application container was therefore not recreated by the installer. Keep the partial state; do not delete private `/etc/rcat-anyflip-svelte` or re-deploy the web app.

After the **corrected source** is merged with green CI, use a fresh detached worktree at the exact accepted commit and run:

```bash
sudo git -C /opt/anyflip-downloader fetch --no-tags origin main
FIX_SHA=REPLACE_WITH_VERIFIED_MERGED_SHA
FIX_RELEASE="/opt/anyflip-boot-fix-$(date -u +%Y%m%dT%H%M%SZ)"
sudo git -C /opt/anyflip-downloader worktree add --detach "$FIX_RELEASE" "$FIX_SHA"
sudo bash "$FIX_RELEASE/scripts/install-svelte-boot.sh" /opt/anyflip-downloader --re-pin
sudo /usr/local/libexec/rcat-anyflip-svelte-boot.sh check
sudo systemctl is-enabled rcat-anyflip-svelte-boot.service
```

The `--re-pin` flag is required because `active.env` and `compose.pinned.yaml` were written during the failed installation. It retains the existing healthy Svelte image identity. This corrected installer does not restart the running web container. Do **not** reboot the Host merely to test this repair: wait for a scheduled maintenance window and record boot journal/health evidence then.

The fixed `scripts/test-svelte-boot-config.sh` now executes the selected `docker ps` expression in an isolated Bash test with a synthetic Docker function, rather than depending only on `bash -n` which could not detect the bug.
