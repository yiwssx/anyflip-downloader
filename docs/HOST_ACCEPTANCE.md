# P3 — Ubuntu LAN host acceptance and safe handoff

**Status: OPERATOR PHYSICAL ACCEPTANCE REQUIRED.** GitHub CI cannot certify the existing Ubuntu host, a disk hard quota, authorized content, browser/device fidelity, public HTTPS or multi-user security. This runbook does not change the running server.

## 1. Inspect the existing deployment

The operator-confirmed LAN URL is http://10.10.1.100:8094 and the host has a customized compose.web.yaml, private .env and a persistent Docker named volume. The repository template instead binds 127.0.0.1:8094. **Never overwrite the host-local Compose, .env, port bindings or volume.** Do not run docker compose down -v. Do not enable public routing as part of acceptance.

From /opt/anyflip-downloader on the actual Ubuntu host:

    cd /opt/anyflip-downloader
    git status --short
    git rev-parse HEAD
    docker compose -f compose.web.yaml ps
    docker compose -f compose.web.yaml config --quiet

Protect host-local modifications and data before any update. Do not force-reset or overwrite the customized Compose.

## 2. Read-only host preflight (no token required)

Run from the deployment directory on the host:

    BASE_URL=http://10.10.1.100:8094 ALLOW_LAN_HTTP_READONLY=1 sh scripts/verify-host-readiness.sh

This explicit LAN HTTP exception sends **no authorization credential** and creates **no download job**. It checks Compose syntax, nonroot/read-only container, named volume, bind interface, HTTP health, missing-token denial and strict CSP. Expected final message:

    HOST_PREFLIGHT_PASS — read-only checks passed; release/production acceptance still OPEN

Use loopback when the host actually binds 127.0.0.1. Never pass a real token over plain LAN HTTP. The existing verify-web-deployment.sh intentionally refuses authenticated non-loopback HTTP.

This preflight neither enforces nor certifies an OS hard quota.

## 3. Identify and enforce actual disk hard quota

Read-only inspection commands. Host filesystem paths must remain private:

    container_id=$(docker compose -f compose.web.yaml ps -q anyflip-web)
    volume_name=$(docker inspect --format '{{range .Mounts}}{{if eq .Destination "/data/jobs"}}{{.Name}}{{end}}{{end}}' "$container_id")
    test -n "$volume_name" || exit 1
    docker volume inspect --format '{{.Mountpoint}}' "$volume_name"

Review the filesystem, mount options and quota support at the resulting path using findmnt -T and df -h as an administrator. The volume name, application-level 4 GiB soft ceiling, 512 MiB job budget and Docker memory limit are **not** OS disk-quota evidence.

Enforce a hard limit only on an operator-approved dedicated filesystem or filesystem/project quota after confirming platform support. If migration is required, use a verified backup, data-preserving migration and rollback. Never format a live filesystem, remount over a named volume, or fill a shared root filesystem to provoke ENOSPC.

Before checking off this gate, record the exact filesystem/quota configuration and a controlled enforcement test **on that host in an approved maintenance window** with data-integrity and rollback proof. Allow for staging bursts and reserved free space.

## 4. Docker crash and retention acceptance

CI tests the disposable runner's synthetic stale PDF staging cleanup after a forced SIGKILL, fresh-data preservation, restart, health and authorization. This is not a test of the real host.

On the target Ubuntu host, after backup and confirming no valuable job is running, use an operator-approved disposable job. Record a redacted job ID, size before/after, cleanup after the documented one-hour retention, restart recovery, failure codes and rollback. Avoid creating symlinks or large synthetic files in production. Do not delete or reset the named volume.

## 5. Authorized book and browser checks

- Process one *publisher-authorized complete book*; verify source and PDF page counts, first/middle/last pages, Thai legibility, image clarity, crop and orientation in **two independent PDF viewers**. Privately record counts and SHA-256, not access tokens, internal paths or copyrighted source pages in GitHub.
- Check desktop and mobile browsers: authentication, create, progress, cancel, failure, batch, retry, history after restart, PDF download and CSP console warnings. GitHub Chromium E2E uses mock API responses and does not replace these physical checks.
- Prefer SSH tunneling or a private HTTPS endpoint for authenticated access. A Bearer token sent over ordinary LAN HTTP is not encrypted.

## 6. Physical acceptance record

| Evidence | Status |
| --- | --- |
| Deployed commit SHA, container/Compose identity | Pending |
| Token-free read-only host preflight | Pending |
| OS-enforced disk hard quota with controlled proof | Pending |
| Interruption, restart, low-space, retention, rollback | Pending |
| Complete authorized book, page counts and two viewers | Pending |
| Desktop/mobile browser/token/CSP walkthrough | Pending |
| Operator signature, date and release decision | Pending |

**Release decision: NOT APPROVED** until all above items have evidence and operator authorization. OCR, desktop packaging and tuning to host specs remain separate optional milestones.
