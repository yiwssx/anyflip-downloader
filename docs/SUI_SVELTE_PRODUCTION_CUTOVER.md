# SUI-05 — Svelte primary-UI production cutover

Tracking: [AnyFlip migration #130](https://github.com/yiwssx/anyflip-downloader/issues/130).
The designated operator explicitly requested **Svelte as the live UI and production deployment** on 2026-11-16. This is authorization to prepare/execute a controlled cutover, not evidence that a specific Host command succeeded.

## Final application decision

- **Primary UI**: Svelte 5 + shadcn-svelte design system, Google Workspace, Admin/RBAC, responsive iOS-inspired layout.
- **Backend**: unchanged Go OIDC, API, owner-scoped jobs/PDF and quota enforcement. No second frontend server.
- **Deployment image**: source-pinned `Dockerfile.svelte`, Node 22 only at **build time**, Go static-asset embed at runtime, non-root image. `compose.svelte.yaml` is appended **last** and always sets `ANYFLIP_WEB_UI=svelte`, so the legacy UI is never served during successful operation.
- **Legacy UI**: retained as an offline rollback option only; not the selected production UI. Do not delete its code until real Host acceptance.
- **Host configs**: never replace the host-local `compose.web.yaml`, `.env`, `compose.quota.yaml`, `anyflip-quota.service`, persisted jobs/RBAC store, DNS, Tunnel, Google OAuth settings or existing port.
- Fail closed on missing quota overlay, non-Google existing auth, missing live service, changed volume/bind, missing Svelte assets or failed HTTP/CSP tests. Do not start an unrelated second Compose project or delete volumes.

## One controlled Host execution

Execute these three operations **on the existing RCAT Ubuntu Host** as an authorized operator. They intentionally use `git fetch` + an isolated worktree, **not `git pull`**, so locally modified host files are preserved. The worktree is kept to tie the deployed image to immutable source; do not auto-remove it.

```bash
sudo git -C /opt/anyflip-downloader fetch --no-tags origin main
release="/opt/anyflip-svelte-$(date -u +%Y%m%dT%H%M%SZ)"
sudo git -C /opt/anyflip-downloader worktree add --detach "$release" origin/main
sudo bash "$release/scripts/deploy-svelte-ui.sh" /opt/anyflip-downloader
```

The script reads **existing** Host Compose/.env/quota settings, discovers the existing Compose project/container, tags the previous image under a new local rollback tag, writes a mode-600 receipt under `/root/anyflip-svelte-rollback-<timestamp>/`, validates the merged Compose configuration, builds from the **worktree only**, recreates **only** the AnyFlip Web service with `--no-deps --no-build --force-recreate`, verifies the pinned source revision, unchanged mounts/binding, Docker health, Google Svelte HTML, JS/CSS MIME, strict CSP/no-store, and anonymous /auth/me + jobs API 401.

If anything after recreation fails, it attempts to restore the prior image in **legacy** mode with the original Host base/quota files; it preserves the receipt for inspection. On failure to identify the original live project or Host quota overlay, it exits without deploying. **No `docker compose down -v`, no reset/stash/overwrite of host-local changes, no DNS/Tunnel/credential modification.**

**Downtime boundary:** the Web service is recreated, so currently active jobs may be cancelled or interrupted. Plan a quiet window and inform users. This script does not claim to know whether anonymous/unreadable live jobs are active.

## Verify user-visible production behavior

After the command reports `SUI05_CODE_CUTOVER=PASS`, inspect `https://anyflip.rcat.ac.th` using authorized RCAT accounts:

1. Normal `@rcat.ac.th` Google Workspace login/logout; non-institutional Gmail is refused.
2. Regular User, Admin and Super Admin see correct tabs/operations; cross-account job/PDF and Admin permission denials remain server enforced.
3. Single and batch **authorized documents**: create, progress, cancel, retry, history, PDF; verify Image-based PDF label.
4. Desktop and physical iOS/Android: focus/keyboard, light/dark, readable Thai text, bottom navigation, zero horizontal overflow; compare before/after.
5. HTTPS proxy/CSP, `/healthz`, quota usage/retained history/volume persistence, and recovery/rollback evidence.

The deployment script's anonymous smoke does **not** substitute for real-account/device verification. Report separately as **operator-attested** or **not tested**, never invent PASS. Only close #130 after that evidence is accepted.

## Rollback by operator

The script automatically attempts rollback on a post-recreation error. For a later incident, use the saved `/root/anyflip-svelte-rollback-<timestamp>/rollback.yaml` and the `project` value inside its private `receipt.txt`:

```bash
sudo docker compose \
  --project-directory /opt/anyflip-downloader \
  -p EXISTING_PROJECT_NAME \
  --env-file /opt/anyflip-downloader/.env \
  -f /opt/anyflip-downloader/compose.web.yaml \
  -f /opt/anyflip-downloader/compose.quota.yaml \
  -f /root/anyflip-svelte-rollback-TIMESTAMP/rollback.yaml \
  up -d --no-build --no-deps --force-recreate anyflip-web
```

Rollback reuses the previous pinned runtime image and existing volume; it does not erase authentication, jobs or quota configuration. The receipt includes no secrets. After rollback, verify Google login, Admin/role separation, jobs/history, and service health. Do not delete the rollback tag or backup/volume.

## Evidence boundaries

PR checks prove source build, container startup and synthetic security/browser assertions, **not** a real SSH operation, local data restore, HTTPS proxy correctness, physical device UAT or an independently signed OCI release artifact. Neither GitHub merge nor artifact CI changes a running Host by itself.
