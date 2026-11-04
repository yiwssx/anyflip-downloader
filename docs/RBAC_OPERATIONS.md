# RCAT AnyFlip — RBAC operator enablement, recovery and rollback

**Status: CODE REVIEW / QA ONLY. No production activation is authorized.** See #68 for Google SSO cutover, #69 for the epic, and #77 for host acceptance.

## Feature-gate behavior

| Deployment mode | RBAC flag | Expected behavior |
| --- | --- | --- |
| `ANYFLIP_AUTH_MODE=token` | `0` (default) | Existing Web token UI and API, no Admin API |
| `ANYFLIP_AUTH_MODE=google` | `0` (default) | Google SSO only, existing per-user jobs, no Admin API |
| `ANYFLIP_AUTH_MODE=google` | `1` | Google SSO plus durable roles, Admin API and Admin Console after operator bootstrap |
| Google + RBAC 1 but missing/unsafe store | `1` | Application refuses startup; no automatic promotion or reset |

The Compose file forwards the flag, defaulting to `0`. It does not modify the existing job-volume mount or hard quota overlay.

## Prerequisite checks — non-destructive

1. Complete the real HTTPS Workspace sign-in, logout and Gmail rejection tests under #68, using **two distinct verified institutional accounts**. Avoid initial Admin testing while the app is still in token mode.
2. Inspect the actual host's `/opt/anyflip-downloader/.env`, branch/commit, quota-aware Compose overlay and storage service. Preserve local changes to `compose.web.yaml`: Git `pull --ff-only` will refuse to overwrite them. Reconcile differences manually before any deployment.
3. Confirm `systemctl is-active anyflip-quota.service`, `findmnt -M /mnt/anyflip-quota-stage`, the expected bound `10.10.1.100:8094`, service UID ownership, and that the same quota-backed `/data/jobs` volume is mounted in the container. Run `scripts/preflight-google-cutover.sh` as the read-only host preflight.
4. Back up the private RBAC file and the job volume to an access-controlled operator location, encrypt off-host if retained. Do not put this backup or Google credentials into GitHub.
5. Confirm the job directory is mode `0700` to the service user, and `.anyflip-rbac.json` (when present) is a regular mode `0600` file. Do not `chmod` arbitrary paths without checking the mounted volume and UID mapping.

## One-time verified Super Admin bootstrap

While Google SSO works and `ANYFLIP_RBAC_ENABLED=0`, the designated Super Admin signs in through the **actual** Workspace Google account. In the signed-in browser, `GET /auth/me` contains `email`, `role=user` and `identityRef`. The operator verifies the address and Workspace account, and copies **only** the irreversible `g-...` identity reference for the next offline command. No Google token or cookie is copied.

With the correct server volume and quota overlay available, the operator uses the image's built-in CLI:

```sh
cd /opt/anyflip-downloader
# Operator enters the verified identityRef locally; do not paste it into issues.
read -r -p 'Verified Workspace identityRef (g-...): ' RBAC_OWNER
docker compose -f compose.web.yaml -f compose.quota.yaml exec -T \
  -e ANYFLIP_JOB_DIR=/data/jobs anyflip-web \
  /usr/local/bin/anyflip-downloader -rbac-bootstrap-owner "$RBAC_OWNER"
unset RBAC_OWNER
```

This creates the store **only if it does not exist**. It will not replace a malformed or existing store, and it does not grant Admin automatically to other logged-in users. If the command fails, stop and investigate without deleting the file.

After validating the store and backup, set the private environment `ANYFLIP_RBAC_ENABLED=1`. The operator then plans an approved maintenance window to rebuild/recreate only the Web container with **both existing Compose files**, preserving its quota data and ports. Never use `docker compose down -v` or deploy a stock Compose file on this customized host.

## Admin utilization metrics (privacy and correctness)

The role-protected `GET /api/admin/summary` reports queued/running job counts, current reserved running-job slots and their concurrency ceiling. On Linux it also reports the total, used and available bytes of the filesystem containing `ANYFLIP_JOB_DIR` using `statfs`; it never includes device names, host filesystem paths or users' source URLs. If the filesystem cannot be read, `storage` is omitted and the UI explicitly shows unavailable information. On non-Linux builds, storage is unavailable rather than guessed.

**Do not confuse reported filesystem capacity with an independently validated hard quota.** It represents the actual filesystem that the running Web process sees. The operator must confirm that the mounted `/data/jobs` is the intended dedicated approximately 6 GiB hard-quota volume before interpreting these totals as the quota. Verify mount and volume mapping under #77 before production acceptance.

## Minimum production acceptance matrix

- Login: two verified `@rcat.ac.th` accounts, one Gmail/non-Workspace negative case, expired/invalid session, logout.
- Roles: user cannot access `/api/admin/*`; admin can see aggregate metadata but cannot change roles; super_admin can grant/revoke; revoked roles take effect on old sessions; self-demotion and last-super-admin loss are blocked.
- Objects: ordinary list/status/history/retry/cancel/batch/download remain owner-scoped for **all** roles; guessing another owner's PDF/job ID returns 404.
- Admin jobs: bounded paging, cancellation confirmation, reason classification, no raw book URL/source path in responses; audit is created or the privileged mutation fails closed.
- Persistence: role assignments survive container restart and backup/restore; malformed/missing/unreadable stores and compromised audit permissions reject privileges.
- Browser: keyboard, desktop, mobile; no horizontal overflow; user/admin/super admin views; denied network requests do not reveal sensitive data.
- Infrastructure: `anyflip-quota.service` active, 6 GiB mount unchanged, existing PDFs preserved, LAN `10.10.1.100:8094` healthy, Tunnel canonical HTTPS host healthy, `www.rcat.ac.th` still responds successfully.

These checks are operator-owned and cannot be asserted PASS from GitHub CI alone.

## Rollback / incidents

For faulty Admin authorization or a broken role store, **disable the feature gate** (`ANYFLIP_RBAC_ENABLED=0`) in the private Compose environment and safely recreate only the Web container with the existing quota overlay. Retain the store, backup and audit files for investigation and recovery. Never automatically bootstrap a new super_admin over an existing file. Session state resets on process restart; jobs may be cancelled by shutdown, so use a scheduled window.

For approved last-admin recovery, stop privileged writes, recover a reviewed private role-store backup, validate file ownership, permissions and continuity, then perform sign-in and authorization drills before re-enabling Admin. Document the operator decision separately from app audit events. Do not add a public emergency override.

**Important limitation:** Application sessions expire after up to 12 hours and Google Workspace account suspension is not actively polled by this project. Disabling a Workspace account may not immediately invalidate a previously issued in-memory session; application-level disabled records and role changes are enforced at request time.

## No automatic deployment

These instructions are a handoff. Neither the RBAC feature branches nor the Github CI have modified the real Ubuntu host, DNS/Tunnel, Google OAuth credentials, local `.env` or production RBAC roles. Production activation remains blocked until #68 and #77 are explicitly accepted.
