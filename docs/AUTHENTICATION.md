# Google Workspace sign-in — AnyFlip Downloader

**Target access:** Only a Google account verified as a member of the Google Workspace domain `rcat.ac.th` may use the Web GUI.

## What changes

- `ANYFLIP_AUTH_MODE=google`: Browser shows **Sign in with Google**; no local usernames, passwords, access tokens, user provisioning, admin dashboard, or password-reset database.
- Google OAuth 2.0 Authorization Code Flow with PKCE, random state and nonce. Google ID token's RSA signature (JWKS) and `iss`, `aud`, `nonce`, `exp`, `iat`, `hd=rcat.ac.th`, `email_verified=true` and `@rcat.ac.th` email address are verified **on the server**. Google `hd` authorization parameter is only a UI hint and is never the sole access check.
- Each user receives a server-managed, HttpOnly, SameSite=Strict session lasting at most 12 hours. Jobs and archived history are assigned by a one-way hash derived from the stable Google account `sub` value; users cannot access other users' jobs.
- Google tokens are not sent to the browser as session credentials and no Google refresh token is stored. Google mode rejects legacy machine Bearer credentials: every Web/API action requires Google login.
- `ANYFLIP_AUTH_MODE=token` remains the **default only to avoid breaking the currently running deployment**. In token mode the original API/GUI work unchanged. In Google mode, `ANYFLIP_WEB_TOKEN` is **not required**, and any legacy Bearer credential is **rejected**. Switch only after HTTPS and OAuth settings are configured. An unknown auth mode fails closed.

## Setup (operator-owned Google Cloud project)

1. In Google Cloud Console, configure the OAuth consent screen / Google Auth Platform. Where possible select **Internal** for the organization's Google Workspace project. Create an OAuth 2.0 Client ID of type **Web application**.
2. Configure the real production **Authorized Redirect URI** exactly: `https://anyflip.rcat.ac.th/auth/google/callback`. Also set the **Authorized JavaScript Origin** to `https://anyflip.rcat.ac.th` if the Google OAuth client UI requires one. The protocol, host, port and path must match exactly. A raw LAN IP such as `http://10.10.1.100:8094` is **not** a suitable Google OAuth production redirect.
3. Keep the client secret only on the server in its existing private `.env` or secret manager, never in GitHub or a frontend variable:
   
   ```dotenv
   ANYFLIP_AUTH_MODE=google
   GOOGLE_CLIENT_ID=<Google Web OAuth client ID>
   GOOGLE_CLIENT_SECRET=<Google Web OAuth client secret>
   GOOGLE_REDIRECT_URI=https://anyflip.rcat.ac.th/auth/google/callback
   ```
   
4. The current production ingress is **Cloudflare for SaaS O2O → Cloudflare Tunnel → `10.10.1.100:8094`**, not a direct Apache or loopback origin. Preserve the public Host `anyflip.rcat.ac.th` and have the trusted edge overwrite, rather than trust client-provided, `X-Forwarded-Proto: https`. Keep the backend bound to its existing LAN interface and maintain the hard-quota Compose overlay. The app requires matching callback host and HTTPS indication in this mode. Google Identity and OAuth servers are contacted over HTTPS.
5. Before deployment, verify both successful sign-in using a real `@rcat.ac.th` Workspace account and rejection of a Gmail/personal/outside-domain account. Verify mobile/tablet UX, logout, job isolation, and persistence of archived owner metadata. The existing server's quota-aware Compose overlays, volumes, mounts and `anyflip-quota.service` must be preserved.

**No credentials supplied to ChatGPT:** The owner must create the Google OAuth Client and configure its settings in the operator-owned environment. Do not send Google client secrets, OAuth codes, session cookies, or any legacy tokens in chat, GitHub or log files. CI uses synthetic signed tokens with locally generated RSA keys; it never uses production Google credentials.

**Production checklist:** See [`GOOGLE_CUTOVER.md`](GOOGLE_CUTOVER.md) and the read-only `scripts/preflight-google-cutover.sh` before rebuilding the existing container. Never use the stock Compose file alone on the customized server; doing so can change the LAN bind or discard the quota mount.

## Notes and limitations

- **Google SSO is the only interactive authentication provider.** The earlier local account/Admin/recovery modules were removed from the application in favor of Google's account lifecycle and recovery facilities. Old `.anyflip-accounts.json` and authentication audit files, if present on the host, are **not automatically deleted** or migrated; back up and handle them separately.
- The application session lives in server memory and is invalidated on restart or logout. When an organization revokes a user's Google Workspace account, an already-issued app session may remain usable until expiry; consider a shorter session duration or a directory integration if immediate revocation becomes a requirement.
- Existing token-authenticated jobs have no Google owner; they are not exposed to regular Google users, though old metadata remains within the original retention rules.
- The output remains an image-based PDF created from permitted source pages. Google login does not confer rights to download third-party copyrighted documents.
- Production setup/Google Cloud organization ownership, DNS/HTTPS, host cutover and acceptance are **not performed by repository code changes**.

Sources: [Google OAuth Web Server](https://developers.google.com/identity/protocols/oauth2/web-server) and [Google OpenID Connect validation](https://developers.google.com/identity/openid-connect/openid-connect).
