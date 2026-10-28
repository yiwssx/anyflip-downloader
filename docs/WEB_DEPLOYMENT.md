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

The app uses in-memory job state (maximum two active downloads, twelve pending jobs), a 20-minute per-job timeout and a one-hour terminal-job cleanup timer. Uploaded/downloaded page image responses are limited to 32 MiB per page. For memory safety, the image header must also declare no more than 40 million decoded pixels or 12,000 pixels along either dimension; a typical 7680×4320 (8K UHD) image is inside this guard. A page exceeding these limits fails the job rather than silently downsampling. JPEGs requiring EXIF normalization additionally have a 25-million-pixel transformation limit. These bounds do not replace a validated job-level disk quota or production load testing. This is an administrator-token MVP, **not a multi-tenant service**. Jobs in memory are not recovered across restarts; named-volume PDFs may remain if the server stops before the cleanup timer runs.

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
