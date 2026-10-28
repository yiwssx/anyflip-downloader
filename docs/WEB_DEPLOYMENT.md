# Deploy AnyFlip Downloader Web GUI on Ubuntu Server

This is a **self-hosted deployment recipe**. A working **LAN-only** Web GUI was operator-confirmed on 2026-10-28 after the pdfcpu HOME/config volume fix (PR #18); this is not confirmation of any configured public domain, Apache HTTPS service, or approved production release. It retains the existing CLI while adding an authenticated browser interface. Output is an **image-based PDF**, not a recovered original text/vector PDF. Use it only for books you are authorized to download or reproduce.

## Requirements

- Ubuntu Server with Docker Engine and Docker Compose v2 installed
- Apache2 with `proxy`, `proxy_http`, `ssl`, `headers` and `rewrite` modules; a domain with DNS A/AAAA records pointing to the server
- Valid HTTPS certificate (e.g. issued using Certbot)
- Sufficient free space and memory; this example has 2 vCPU / 2 GiB as a **container limit**, not a validated concurrency benchmark

## Build and launch on localhost only

```sh
git clone https://github.com/yiwssx/anyflip-downloader.git
cd anyflip-downloader
git switch main

# Create a secret .env that must never be committed:
umask 077
printf 'ANYFLIP_WEB_TOKEN=%s\n' "$(openssl rand -hex 32)" > .env
chmod 600 .env

docker compose -f compose.web.yaml config --quiet
docker compose -f compose.web.yaml up -d --build
docker compose -f compose.web.yaml ps
curl --fail --silent --show-error http://127.0.0.1:8094/healthz
```

The Compose file binds **127.0.0.1:8094** on the host, routes it to port 8080 in a Docker container and keeps temporary/finished PDFs in the `anyflip_jobs` named volume. The image runs with UID 10001, no extra Linux capabilities and a read-only root filesystem. The application is **not** exposed directly on a public host network interface.

To obtain the token later from your local `.env`, consult that file privately on the server; **never** post it in GitHub, logs, chat messages or URLs.

## Apache HTTPS reverse proxy

1. Create the DNS record for the actual domain and obtain a valid HTTPS certificate. If using Certbot, complete certificate issuance **before** enabling the example TLS virtual host (otherwise Apache will fail to start).
2. Enable Apache modules: `sudo a2enmod proxy proxy_http ssl headers rewrite`.
3. Copy `deploy/apache/anyflip-web.conf.example` to an Apache site file, replace every occurrence of `downloader.example.com`, and check paths to the issued certificate.
4. Run `sudo apachectl configtest`, enable your new site with `sudo a2ensite <your-site-file>`, reload Apache, and verify HTTPS with a browser.

The Apache config proxies traffic only to `http://127.0.0.1:8094/`; do not configure a public bind to port 8080/8094. Use the DNS/hostname of your own choice. **No DNS or certificate has been created in this PR.**

## Web use and checks

Visit your own HTTPS domain in Chrome, Edge or Safari. Enter the strong administrator token, paste an authorized AnyFlip book URL, then start/monitor/cancel the job. The download button becomes active when the verified PDF is ready.

The completed CLI message and Web GUI explicitly call the result **Image-based PDF**, not the original editable publisher PDF. Authenticated download responses use `Content-Type: application/pdf` plus `X-AnyFlip-Output-Type: image-based-pdf` for API clients; this custom header is informational and does not replace MIME type or document authenticity checks.

Check authorization enforcement without leaking the real token:

```sh
curl -i http://127.0.0.1:8094/healthz
curl -i http://127.0.0.1:8094/api/jobs/00000000000000000000000000000000
# The protected request should return HTTP 401 without a token.
```

The Docker Compose deployment allows a 35-second stop grace period, while the Go HTTP server cancels running jobs and attempts to drain active requests on SIGTERM. Files from interrupted jobs are subject to retention cleanup after a restart.

The Web API also throttles repeated **failed** Bearer-token checks from the same TCP peer: five failures within one minute trigger a five-minute cooldown (HTTP 429 and `Retry-After`), with an in-memory cap of 1,024 tracked peers. Successful authentication clears that peer's failures. This is a local, best-effort safety limit, **not** comprehensive DDoS protection, public authorization, or a guarantee against distributed attacks. The application deliberately ignores untrusted `X-Forwarded-For` headers. When accessed through Apache, all requests may appear to originate from the proxy IP and a failed-client lockout may temporarily affect other clients; any public deployment requires its own trusted-proxy-aware rate limits, HTTPS and access restrictions. Throttle state resets on restart. 

### Web volume capacity admission (P3-J2 code gate)

Before a queued Web job begins, the Linux service counts only files with the strict generated job-ID names (`*.pdf`, `*.images`) and its own PDF staging directories. It then atomically reserves **512 MiB per active Web job**, refuses to start if managed retained/staging files plus active reservations exceed **4 GiB**, and requires at least **512 MiB additional free space** beyond running/new job reservations (using non-root-available filesystem blocks). It does not open, count or remove unrelated host files; unrelated files still reduce the real reported free space. A refused job finishes with a safe `storage_capacity_exceeded` error. CLI behavior and the existing LAN/Apache bindings are unchanged.

This is **fail-closed admission only, not an active-write hard quota**. Existing writers can exceed their checkpointed limits transiently, or other host processes may fill the filesystem. If the service runs outside Linux, Web admission refuses execution because usable disk space cannot be verified; the CLI remains usable. A hard limit must be separately enforced by a dedicated filesystem, filesystem project quota or other host-supported storage mechanism, tested on the actual Ubuntu Docker host. Do not switch an existing Docker named volume to a bind mount without an independently verified data migration/rollback plan. Test low-free-space, two simultaneous jobs, restart/retention and failed PDF output before enabling any public-facing endpoint.

### Web job storage budget (P3-J1)

The Web GUI enforces an application-level **512-page limit and a 512 MiB per-job storage budget** (image workspace plus isolated PDF staging). The original CLI has no new default limits. After every completed image and PDF import chunk, the service checks on-disk files and fails closed with safe `page_limit_exceeded` or `storage_quota_exceeded` errors rather than publishing incomplete PDFs. Quota failures remove only that job's exclusively owned image workspace immediately.

**This is a checkpointed soft budget, not a filesystem/Docker-volume hard quota.** A concurrent image request (up to 32 MiB per page), multiple workers, JPEG orientation preprocessing and a PDF import chunk can temporarily grow beyond this per-job cap before the next check; completed/failed other jobs still consume the same named volume. Do not claim that these checks prevent a full disk or guarantee a global storage ceiling. For production, separately configure host filesystem/volume quotas, minimum-free-space monitoring, retention tests and capacity verification on the actual Ubuntu server. Existing `10.10.1.100:8094` LAN bindings remain operator-controlled; this code does not change port mappings or enable public access.

The app uses in-memory job state (maximum two active downloads, twelve pending jobs), a 20-minute per-job timeout and a one-hour terminal-job cleanup timer. Uploaded/downloaded page image responses are limited to 32 MiB per page. For memory safety, the image header must also declare no more than 40 million decoded pixels or 12,000 pixels along either dimension; a typical 7680×4320 (8K UHD) image is inside this guard. A page exceeding these limits fails the job rather than silently downsampling. JPEGs requiring EXIF normalization additionally have a 25-million-pixel transformation limit. These bounds do not replace a validated job-level disk quota or production load testing. This is an administrator-token MVP, **not a multi-tenant service**. Jobs in memory are not recovered across restarts; named-volume PDFs may remain if the server stops before the cleanup timer runs.

## Operator batch queue and temporary history (P4)

The same administrator token grants access to the authenticated **POST `/api/jobs/batch`** endpoint (JSON `{"jobs":[{"url":"https://anyflip.com/authorized/book","threads":3,"bestQuality":false}]}`), capped at eight URLs per request and the shared 12-active-or-queued job limit. The server validates every member and checks queue space before scheduling any of them. Jobs receive separate unpredictable IDs, temporary folders, cancellation contexts and PDF files. No publisher permissions or network allowlist are bypassed.

Authenticated **GET `/api/jobs`** returns at most 50 current in-memory job statuses, without original URLs, titles, tokens or server paths. Authenticated **POST `/api/jobs/{id}/retry`** schedules a *new job ID* with the prior failed/cancelled job's internal settings; running or completed jobs cannot be retried. The Thai Web interface has a batch URL field, status selector and manual retry controls.

**Important:** The current API is an administrator-only service with a shared token; it is not multi-user and provides no per-person job ownership. Recent terminal job status is persisted in a small 0600 atomic JSON metadata file (`.anyflip-history.json`) in the job volume: at most 50 records, expiring after seven days, survives normal container restarts. Only job ID, terminal stage, page counts, classified safe errors and finish time are stored; source URLs, titles, tokens, file paths and retry settings are never persisted. PDF/image artifacts still expire after one hour; archived history does not allow PDF download, cancellation or retry after restart. Running jobs do not resume after restart. This is a single-admin metadata history, **not** a database or an audited multi-user history. Separate user accounts are not implemented. Do not expose this API to untrusted users or call it a persistent queue.

## Post-deployment smoke test (safe)

The repository includes `scripts/verify-web-deployment.sh`. It checks health,
rejects unauthenticated API calls and (when the token is set) tests a
nonexistent job and a deliberately invalid AnyFlip URL. **It never submits a
valid download job**, so no book data is retrieved. It also validates Docker
Compose configuration when the Docker CLI is available.

From the deployment directory on the server, with the existing private
`.env` file:

```sh
set -a
. ./.env
set +a
sh scripts/verify-web-deployment.sh
unset ANYFLIP_WEB_TOKEN
```

For HTTPS reverse proxy verification, supply `BASE_URL=https://your-real-domain`
and keep certificate verification enabled. The script refuses ordinary HTTP
to non-loopback hosts. Expected result: `SMOKE_CHECK_PASS` with any optional
Docker checks explicitly marked `SKIP` if unavailable.

This does not replace a real browser/mobile walkthrough, TLS/certificate
check, authorized real-book test or Docker restart/recovery test.

## Diagnosing a URL that works in CLI but fails in Web GUI

CLI and Web GUI share the same Go download engine. If the same authorized
AnyFlip reader URL works locally but fails in a deployed Web GUI:

1. Confirm that the Web UI accepts your administrator token and creates a Job ID.
   A 401 from `/api/jobs` indicates incorrect Web authentication; it is not
   a book/download permission issue.
2. Check the Job ID and displayed error code (`network_error`,
   `source_access_denied`, `metadata_invalid`, `storage_error`,
   `job_timeout`, etc.). Codes are deliberately safe to share; do not
   share administrator tokens, source credentials, or full raw server logs.
3. Review logs privately on the Docker host:
   `docker compose -f compose.web.yaml logs --tail=100 anyflip-web`.
   Find the generated Job ID and classified `stage`/`code` fields.
   Web job logs deliberately **do not include raw source URLs, URL query
   credentials, temporary filesystem paths, or transport error strings**.
   For DNS/egress errors, check the container's approved outbound HTTPS
   connectivity; for storage errors, inspect the named volume's ownership and
   available disk space. Never re-enable raw errors in persistent logs.
4. When code has been updated, redeploy using
   `docker compose -f compose.web.yaml up -d --build`, then rerun
   `scripts/verify-web-deployment.sh` and retest with an authorized document.
   Do not weaken TLS verification, AnyFlip hostname checks or network
   restrictions to make a failing book work.

A test of an available first page is not proof that an entire book or the
production server will succeed. Web jobs have a 20-minute maximum duration.

## Maintenance, rollback and security limitations

```sh
docker compose -f compose.web.yaml logs --tail=100 anyflip-web
docker compose -f compose.web.yaml restart anyflip-web
docker compose -f compose.web.yaml stop
docker compose -f compose.web.yaml up -d --build
```

To roll back, deploy a vetted Git commit or release tag and rebuild, preserving the named volume unless you have intentionally decided to delete its contents. Do not run `docker compose down -v` unless you want to remove all retained job PDFs/images. For security, rotate tokens whenever needed and restrict read access to the `.env` file and Docker socket. Consider an additional Apache access allowlist/VPN for administrative use.

**Production gates not validated yet:** Apache TLS integration, OS firewall/NAT review, physical browser/device QA, complete publisher-authorized book PDF acceptance, production volume restart/retention recovery, monitoring, capacity and security testing. The operator reported a working LAN-only GUI, but no complete-book evidence has been captured in this repository. GitHub Actions Go Build/Vet/Unit/Race and Docker Compose smoke passed on [PR #19's last head (run 40631954574)](https://github.com/yiwssx/anyflip-downloader/actions/runs/40631954574). These tests use a GitHub-hosted runner, not the user's Ubuntu host. Never claim public deployment readiness from CI alone.

**Existing LAN deployments:** The stock example intentionally binds `127.0.0.1:8094:8080`; preserve an already approved host-specific bind (currently `10.10.1.100:8094`) when editing its Compose. Do not replace the host's customized file with the stock template. Keep the token in a private `.env` and use loopback/SSH tunneling or HTTPS when sending administrator credentials, because HTTP on a LAN does not encrypt the Bearer token. No change to the host's reverse proxy is required for the currently operating LAN-only service.
