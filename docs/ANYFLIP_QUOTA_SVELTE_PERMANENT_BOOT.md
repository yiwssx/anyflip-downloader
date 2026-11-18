# Permanent RCAT AnyFlip Svelte startup through the original quota service

## Confirmed root cause (host evidence, 2026-11-18)

The production operator shared the exact `/etc/systemd/system/anyflip-quota.service`:

```ini
[Unit]
Description=AnyFlip Downloader (quota filesystem required)
Requires=docker.service
BindsTo=mnt-anyflip\x2dquota\x2dstage.mount
After=docker.service mnt-anyflip\x2dquota\x2dstage.mount
RequiresMountsFor=/mnt/anyflip-quota-stage

[Service]
Type=simple
WorkingDirectory=/opt/anyflip-downloader
ExecStartPre=/usr/local/sbin/anyflip-quota-ready
ExecStart=/usr/bin/docker compose -f compose.web.yaml -f compose.quota.yaml up --no-build --no-color anyflip-web
ExecStop=/usr/bin/docker compose -f compose.web.yaml -f compose.quota.yaml stop anyflip-web
Restart=on-failure
RestartSec=10s
TimeoutStopSec=70s

[Install]
WantedBy=multi-user.target
```

The runtime Docker image ID matched the previously approved Svelte image
`sha256:80056ad0eb1908fedd70eb5f86857b583dc2385a6e2e1f91e2d19f93bc9223d3`
(source `36b2539f1aaaf1eb0acb8fe04d800577a9a3a81d`), but the current
container did not set `ANYFLIP_WEB_UI=svelte` (nor `=legacy`).
The Go server defaults to legacy mode when the environment variable is
missing. Thus **the image is already correct**, but the operator-owned
quota-managed startup is missing the Svelte Compose overlay. Recreating
the service after reboot can select the old frontend despite the correct
image. This is a verified Host configuration issue, not a browser cache or
a GitHub source rollback.

The previous separate `rcat-anyflip-svelte-boot.service` tried
`docker compose up --force-recreate` during Docker container restoration,
resulting in a name conflict. It was disabled. Do **not** re-enable it to
solve this problem.

## Permanent design

Use the **existing** quota-owned systemd service as the only authority
that starts and stops production AnyFlip. Keep its
`ExecStartPre=/usr/local/sbin/anyflip-quota-ready`, hard quota mount
`BindsTo`, `After`, `RequiresMountsFor`, original `.env`, original
`compose.web.yaml` and host-private `compose.quota.yaml`.
A narrow, reversible drop-in changes only the Compose invocation and the
local Svelte image selection:

```text
/etc/systemd/system/anyflip-quota.service
└── anyflip-quota.service.d/90-svelte-pinned.conf
       Environment=RCAT_SVELTE_IMAGE=rcat-anyflip-svelte:boot-<sha>-<image>
       ExecStart= (reset original command)
       ExecStart=/usr/bin/docker compose --project-directory ... -p ...
         --env-file /opt/anyflip-downloader/.env
         -f /opt/anyflip-downloader/compose.web.yaml
         -f /opt/anyflip-downloader/compose.quota.yaml
         -f /etc/rcat-anyflip-svelte/compose.pinned.yaml
         up --no-build --no-color anyflip-web
       ExecStop= (reset original command)
       ExecStop=<matching Compose set> stop anyflip-web
```

The existing operator-approved
`/etc/rcat-anyflip-svelte/compose.pinned.yaml` sets:
`image: ${RCAT_SVELTE_IMAGE:?...}`,
`build: !reset null`, `restart: "no"`, and
`ANYFLIP_WEB_UI=svelte`. Docker Compose does **not** rebuild,
download, switch Google Workspace authentication, change ports, unmount
the quota filesystem, delete jobs or modify RBAC.

The drop-in is the **single source of truth for both boot and normal
service restart**. No additional reconciliation service, polling task or
forced-recreate step exists on the boot path.

## Installation: two explicit phases

Run the verified release on the authorized Ubuntu host.

**Phase A — configure only; no downtime**:

```bash
sudo git -C /opt/anyflip-downloader fetch --no-tags origin main
RELEASE_SHA=REPLACE_WITH_APPROVED_MERGE_SHA
RELEASE="/opt/anyflip-quota-svelte-$(date -u +%Y%m%dT%H%M%SZ)"
sudo git -C /opt/anyflip-downloader worktree add --detach "$RELEASE" "$RELEASE_SHA"

# Ensure previously unsafe boot-time re-creation is disabled; this
# does not stop the running application.
sudo systemctl disable --now rcat-anyflip-svelte-boot.service
sudo systemctl reset-failed rcat-anyflip-svelte-boot.service

sudo bash "$RELEASE/scripts/install-svelte-quota-startup.sh" /opt/anyflip-downloader
```

Expected `QUOTA_SVELTE_STARTUP=CONFIGURED`,
`QUOTA_SVELTE_COMPOSE=PASS`,
`DOCKER_CONTAINERS_CHANGED=no`,
`APPLY=OPERATOR_CONTROLLED_SINGLE_SERVICE_RESTART_PENDING`.

The installer fails closed if the quota mount is not ext4, the
existing unit is not precisely the operator-provided quota manager,
an unexpected drop-in exists, the Svelte image is missing, the active
Google auth/port/mount/project differs, or the separate boot guard is
enabled. It does not overwrite the original unit, secret files or
private Compose files.

**Phase B — one explicit controlled service restart, after all downloads
finish**:

```bash
sudo bash "$RELEASE/scripts/apply-svelte-quota-startup.sh" \
  /opt/anyflip-downloader --apply
```

Expected `QUOTA_SVELTE_APPLY=PASS`,
`SUI05_SVELTE_PRIMARY=PASS`,
`QUOTA_SVELTE_REBOOT_PROOF=PENDING_NORMAL_REBOOT`.

The existing quota service retains its normal behavior. Its own
`ExecStop` stops **only** `anyflip-web` (not PDF HUB/POS/Radio); the
same service restarts it with **base + quota + pinned Svelte**.
This is a single operator-authorized cutover; no automatic rollback or
concurrent Compose repair is attempted.

If phase B fails, **do not** repeatedly restart or delete container
names. Inspect `journalctl -u anyflip-quota.service -b` and
`docker ps -a` before an authorized manual rollback.

## Reversible change

The original quota unit and all host-owned files remain unchanged.
The new drop-in is
`/etc/systemd/system/anyflip-quota.service.d/90-svelte-pinned.conf`.
If rollback is explicitly authorized, first save that file in a
private root-owned evidence folder, then remove only this exact
verified drop-in, run `systemctl daemon-reload` and perform **one**
controlled `systemctl restart anyflip-quota.service`. This returns
the original base+quota command and therefore the legacy fallback UI.
This is an emergency rollback path, **not** recommended as the steady
state.

## Subsequent reboot and future releases

No immediate production reboot is necessary to validate this change:
the actual one-service systemd restart and Svelte HTTP/CSP/Google entry
smoke prove the selected release works via the same Compose command.
At the **next normally scheduled reboot**, verify
`systemctl is-active anyflip-quota.service`, current
`ANYFLIP_WEB_UI=svelte`, pinned image ID and health. Record boot
evidence; until then, reboot persistence is operator-pending, not PASS.

For any future approved Svelte image update, explicitly re-pin the
image/source and update the managed drop-in through a reviewed
maintenance procedure. The installer deliberately refuses to overwrite
an existing drop-in with a different image to avoid silently
reverting or upgrading production.

**Separate server-wide incident:** `containerd.service` timed out
during the 2026-11-18 boot and Docker required several minutes to
recover. This Svelte startup correction does **not** fix or claim to fix
containerd startup timeouts affecting PDF HUB, POS or Radio. Handle that
issue separately after inspecting the exact initial containerd log,
slow restoration of old drill containers and systemd startup policy.
Do not increase global daemon timeouts or delete containers without
evidence.
