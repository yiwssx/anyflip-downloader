# Google Workspace production cutover — 2026-11-03

**Goal:** RCAT AnyFlip Downloader at `https://anyflip.rcat.ac.th` must accept only verified Google Workspace users whose ID token has both `hd=rcat.ac.th` and a verified `@rcat.ac.th` email. Users must **never** enter the old AnyFlip Access Token.

**Current state (2026-11-04):** Production Google Workspace cutover and RBAC enablement were **accepted by the Ubuntu host operator**, who confirmed that all applicable checks passed and requested closure. This is an operator attestation, not independent live-host verification by GitHub CI or the assistant. No OAuth credentials are present in source control. The commands below are retained as a reference for future redeployments and rollback.

## Step 1 — Provision Google OAuth Web client (historical setup / future reconfiguration)

1. Open [Google Cloud Auth Platform](https://console.cloud.google.com/auth/overview). Create/select a Google Cloud project owned by RCAT; configure OAuth consent/branding. Set **Internal** audience if the project belongs to the RCAT Google Workspace organization and this option is available.
2. Create **OAuth Client ID → Web application**. Authorized redirect URI must be **exactly**:

   `https://anyflip.rcat.ac.th/auth/google/callback`

   Add **Authorized JavaScript origin** `https://anyflip.rcat.ac.th` if the client UI requires it.
3. Retain the Client ID and Client Secret privately. Do **not** post the secret, authorization code, state, session cookie, service token, or `.env` contents in ChatGPT, GitHub, issue comments or deployment logs. Do not use the Cloudflare Tunnel token as the Google OAuth client secret.
4. Ensure the organization's Workspace policies allow the OAuth client. The backend independently verifies the signed ID token signature, issuer, audience, nonce, expiration, hosted-domain claim, verified email, and email suffix; the Google `hd` sign-in URL parameter alone does not authorize access.

## Step 2 — Prepare the existing Ubuntu host without touching storage

The actual running host is `10.10.1.100`. AnyFlip Docker binds **10.10.1.100:8094**, not loopback. Existing hard quota is a host-managed filesystem mount, supervised by `anyflip-quota.service`. **Keep the existing local `compose.quota.yaml` and any other operator-managed Compose overrides.** Do not replace the live configuration with the stock `compose.web.yaml` alone, and do not run `down -v`.

Run on Ubuntu:

```bash
cd /opt/anyflip-downloader
sudo git pull --ff-only origin main
# Edit the existing PRIVATE .env, preserving unrelated settings.
sudoedit .env
```

Keep or add the following three Google values (replace placeholders privately):

```dotenv
ANYFLIP_AUTH_MODE=google
ANYFLIP_HOST_BIND_IP=10.10.1.100
GOOGLE_CLIENT_ID=YOUR_WEB_CLIENT_ID.apps.googleusercontent.com
GOOGLE_CLIENT_SECRET=YOUR_PRIVATE_GOOGLE_CLIENT_SECRET
GOOGLE_REDIRECT_URI=https://anyflip.rcat.ac.th/auth/google/callback
```

`ANYFLIP_WEB_TOKEN` is no longer required and is **ignored/rejected as a Bearer credential** in Google mode. It may be removed once the Google rollout is accepted, but keep a secure rollback record until then. Protect `.env` with `chmod 600`; it must not enter the Git repository.

**Before any rebuild**, run the read-only guard on the Ubuntu host:

```bash
sudo bash scripts/preflight-google-cutover.sh
```

The preflight checks the OAuth configuration without printing secrets, exact callback URL, current vs proposed host port `10.10.1.100:8094`, one and the same `/data/jobs` mount, active `anyflip-quota.service`, and HTTP+HTTPS health. It **does not restart or modify** anything. A `BLOCK:` result means **do not deploy**. If the original host uses more Compose overlays, include them in the reviewed operator command; update the preflight to check all overlays first. No bypass of storage checks in production.

## Step 3 — Only after preflight passes: deploy the existing service in place

For the confirmed deployment that uses `compose.web.yaml` plus the local `compose.quota.yaml`:

```bash
cd /opt/anyflip-downloader
sudo docker compose -f compose.web.yaml -f compose.quota.yaml up -d --build --no-deps anyflip-web
```

This rebuilds **only the application service**, so in-flight jobs and in-memory sessions can be interrupted. Do this during a maintenance window, and verify the actual config still uses the existing hard-quota mount and port. It does **not** stop or redeploy the separate `rcat-anyflip-ingress` Tunnel.

## Step 4 — Public verification and acceptance

```bash
curl -fsS https://anyflip.rcat.ac.th/healthz
curl -sS -o /dev/null -w 'anonymous jobs HTTP %{http_code}\n' https://anyflip.rcat.ac.th/api/jobs
curl -sS -o /dev/null -w 'Google sign-in HTTP %{http_code}\n' https://anyflip.rcat.ac.th/auth/google
curl -fsS https://anyflip.rcat.ac.th/ | grep -o 'mode-google' | head -1
```

Expected: `/healthz=200`, anonymous API `401`, `/auth/google=303` (when checked without following redirects), canonical HTML `mode-google`. The alternate `https://anyflip.rcat.dpdns.org/` should redirect to `https://anyflip.rcat.ac.th/`, while its `/healthz` remains HTTP 200. Also verify `https://www.rcat.ac.th/` HTTP 200.

**Mandatory real-browser acceptance:** Google login with a legitimate `@rcat.ac.th` Workspace account, denial of a personal Gmail or other domain, logout/session expiry, one authorized document download, history ownership isolation in two authorized accounts, and a stable hard quota. CI's synthetic claims/tests do not prove that Google's live OAuth client has been configured or approved.

## Rollback

Before starting, privately back up the current `.env` and note the currently deployed image/commit and Compose config; never send secrets to chat. If Google OAuth login does not work, restore the prior `ANYFLIP_AUTH_MODE=token` `.env` values and **recreate only** `anyflip-web` using the exact same two or more Compose files and live quota mount. Do **not** remove or recreate `anyflip-quota.service`, the job volume, DNS records, Cloudflare Tunnel or www.

OAuth client configuration and physical Production deployment must be performed by the account/host operator: repository changes cannot create the organization's Google client credentials.
