# Deploy AnyFlip Downloader Web GUI on Ubuntu Server

This is a **self-hosted deployment recipe**, not a confirmation that any Ubuntu host or DNS has been configured. It retains the existing CLI while adding an authenticated browser interface. Output is an **image-based PDF**, not a recovered original text/vector PDF. Use it only for books you are authorized to download or reproduce.

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

Check authorization enforcement without leaking the real token:

```sh
curl -i http://127.0.0.1:8094/healthz
curl -i http://127.0.0.1:8094/api/jobs/00000000000000000000000000000000
# The protected request should return HTTP 401 without a token.
```

The Docker Compose deployment allows a 35-second stop grace period, while the Go HTTP server cancels running jobs and attempts to drain active requests on SIGTERM. Files from interrupted jobs are subject to retention cleanup after a restart.\n\nThe app uses in-memory job state (maximum two active downloads, twelve pending jobs), a 20-minute per-job timeout and a one-hour terminal-job cleanup timer. This is an administrator-token MVP, **not a multi-tenant service**. Jobs in memory are not recovered across restarts; named-volume PDFs may remain if the server stops before the cleanup timer runs.

## Maintenance, rollback and security limitations

```sh
docker compose -f compose.web.yaml logs --tail=100 anyflip-web
docker compose -f compose.web.yaml restart anyflip-web
docker compose -f compose.web.yaml stop
docker compose -f compose.web.yaml up -d --build
```

To roll back, deploy a vetted Git commit or release tag and rebuild, preserving the named volume unless you have intentionally decided to delete its contents. Do not run `docker compose down -v` unless you want to remove all retained job PDFs/images. For security, rotate tokens whenever needed and restrict read access to the `.env` file and Docker socket. Consider an additional Apache access allowlist/VPN for administrative use.

**Production gates not validated yet:** a real Docker build/run on the target Ubuntu server, `docker compose config`, Apache TLS integration, OS firewall, browser/device UI checks, real authorized book retrieval, post-restart orphan-file cleanup and operational capacity testing. GitHub Actions currently reports no workflow runs; the Go tests were run independently in a sandbox. Complete these gates before claiming public deployment readiness.
