# Account authentication (P4-AUTH)

**Status:** Code and CI gate only. Not yet deployed on the Ubuntu operator host; not approved for public exposure.

## Architecture

- `ANYFLIP_AUTH_MODE=token` (default) preserves the existing GUI and API Bearer-token behavior.
- `ANYFLIP_AUTH_MODE=accounts` serves a distinct Login-based GUI with **no Access Token field**. Account credentials are verified on the Go server; users receive an opaque, random, 12-hour, HttpOnly SameSite=Strict session cookie. Session IDs are stored server-side only (hashed in memory). Logout revokes the session and a process restart invalidates all sessions.
- Argon2id salted password hashes are persisted in `/data/jobs/.anyflip-accounts.json` with 0600 permissions. Passwords, original download URLs and machine API secrets are not stored in browser localStorage or JS.
- Admin can create user/admin accounts. Users see only their own in-memory jobs and archived job metadata. Admin may see all. Old jobs without an owner are Admin-only; regular Users cannot access them even with a known Job ID. Shared machine API Bearer credentials continue to act as a privileged service account; keep the secret server-side and restrict API clients.
- Job files and the existing one-hour retention are unchanged. This is not SSO, MFA, email recovery or public multi-tenancy.

## First setup (only after HTTPS is configured)

1. Build the Go binary from reviewed `main` (or use the vetted container CLI), then create an admin hash without putting the password on a command line:

   ```sh
   read -rsp 'Admin password (12+ characters): ' ADMIN_PASSWORD
   printf '\n'
   ADMIN_HASH="$(printf '%s' "$ADMIN_PASSWORD" | ./anyflip-downloader -hash-password)"
   unset ADMIN_PASSWORD
   printf 'Password hash generated (do not print or share it).\n'
   ```

2. In an operator-owned, mode-0600 `.env` place `ANYFLIP_AUTH_MODE=accounts`, `ANYFLIP_AUTH_BOOTSTRAP_USERNAME=admin`, `ANYFLIP_AUTH_BOOTSTRAP_HASH=<generated hash>`, and `ANYFLIP_AUTH_COOKIE_SECURE=true`. Keep the existing strong `ANYFLIP_WEB_TOKEN` **server-side**. The service bootstraps the first account only if the private account file does not yet exist; subsequent launches load the file and do not reset the administrator.

3. Only make the browser available through a **trusted HTTPS reverse proxy with preserved Host** or an isolated local loopback for development. The Go container may see HTTP from a terminating proxy; its Secure cookie and HTTPS Origin check ensure actual browsers use HTTPS. Restrict direct host/LAN access, firewall and forward-proxy behavior. A plain-HTTP LAN host endpoint must not be used for password login. Do not enable this mode at `http://10.10.1.100:8094` without adding a trusted HTTPS path.

4. Preserve all host-local Compose overlays, quota mounts, existing named volumes and the quota-aware systemd workflow. Do **not** deploy by replacing the operator's customized `compose.web.yaml` with the stock repository template. Confirm TLS, storage backups, account migration and rollback before cutover.

The account store is written atomically with restricted permissions. Back it up privately; never commit it. Account sessions are process-local and will be revoked on restart. Users cannot self-register; Admin creates accounts in the authenticated GUI. An administrator must explicitly provision password reset/revocation procedures before production rollout.

## Endpoints

| Endpoint | Method | Access |
| --- | --- | --- |
| `/auth/login` | POST | Anonymous credentials, same origin; rate-limited |
| `/auth/me` | GET | Valid account session |
| `/auth/logout` | POST | Same-origin session, revokes cookie |
| `/auth/users` | GET/POST | Admin (username, role; POST provisions password) |
| `/api/jobs*` | Existing methods | Account session and job owner; Admin access; server-side Bearer service token |

Cookies never grant a session merely by visiting the website. Passwords are checked during a Login request. For requests using the session cookie, mutating operations require matching Origin or same-origin browser metadata. Authentication is necessary but not sufficient for public access: rate limits, HTTPS, audit logs, account recovery, session persistence choices and security assessment must be accepted separately.

## Testing and release gates

- Go tests: hash, persistence, login/logout, role denial, per-user listing, IDOR rejection, Admin access, archived history and CSRF.
- Playwright: token-only mode remains covered; isolated account-mode browser test exercises login, admin creates user, logout and user UI.
- Physical acceptance **OPEN:** real desktop/mobile HTTPS browser, password rotation, user deactivation/reset, reverse proxy / Host handling, account store backup, real authorized book and quota-aware Docker restart. No deployment to operator machine has been performed by this PR.
