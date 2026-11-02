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

3. Only make the browser available through a **trusted HTTPS reverse proxy with preserved Host** or an isolated local loopback for development. The Go container may see HTTP from a terminating proxy; the proxy must **overwrite** `X-Forwarded-Proto: https` and preserve `Host`. The account login form is refused on direct HTTP when HTTPS mode is enabled. Its Secure cookie and HTTPS Origin check ensure actual browsers use HTTPS. Only trust the forwarded header if the backend is unreachable except through that trusted proxy. Restrict direct host/LAN access, firewall and forward-proxy behavior. A plain-HTTP LAN host endpoint must not be used for password login. Do not enable this mode at `http://10.10.1.100:8094` without adding a trusted HTTPS path.

4. Preserve all host-local Compose overlays, quota mounts, existing named volumes and the quota-aware systemd workflow. Do **not** deploy by replacing the operator's customized `compose.web.yaml` with the stock repository template. Confirm TLS, storage backups, account migration and rollback before cutover.

### Account lifecycle (PR #58)

- Authenticated users can submit `POST /auth/change-password` with `currentPassword` and `newPassword`. New passwords must be 12–128 bytes and differ from the existing password. Successful changes revoke every active session for that account; users must sign in again.
- Human Administrators can list account status, suspend/reactivate users via `PATCH /auth/users/{username}/status` with `{"disabled":true|false}`, and reset **another** account's password via `POST /auth/users/{username}/reset-password` with `{"password":"..."}`.
- Disabled accounts cannot sign in or use any old Session Cookie. Reactivation never restores revoked cookies. Suspension cancels outstanding queued/running downloads and blocks new admission on a second server-side account check. Existing job metadata/PDF retention remain available to authorized Admin; disabling is not deletion.
- An Administrator cannot suspend themselves, and the last active Administrator cannot be disabled. To change an Admin's own password, the existing password must be supplied. API machine Bearer tokens retain job access but cannot administer accounts or change passwords.
- `.anyflip-accounts.json` remains version 1, with a backward-compatible optional `disabled` flag, and remains `0600`. Credential changes write atomically before session revocation; persistence errors roll back the mutation. Operators must back up and protect this file.

The account store is written atomically with restricted permissions. Back it up privately; never commit it. Account sessions are process-local and will be revoked on restart. Users cannot self-register; Admin creates accounts in the authenticated GUI. The application provides password reset and session revocation; operators must still establish a secure out-of-band credential delivery, backup/restore, audit logging and emergency administrator recovery procedure before production rollout.

## Endpoints

| Endpoint | Method | Access |
| --- | --- | --- |
| `/auth/login` | POST | Anonymous credentials, same origin; rate-limited |
| `/auth/me` | GET | Valid account session |
| `/auth/logout` | POST | Same-origin session, revokes cookie |
| `/auth/users` | GET/POST | Admin (username, role; POST provisions password) |
| `/auth/change-password` | POST | Signed-in account; current password required; revokes sessions |
| `/auth/users/{username}/status` | PATCH | Human Admin; suspend/reenable |
| `/auth/users/{username}/reset-password` | POST | Human Admin; target cannot be self; revokes target sessions |
| `/api/jobs*` | Existing methods | Account session and job owner; Admin access; server-side Bearer service token |

Cookies never grant a session merely by visiting the website. Passwords are checked during a Login request. For requests using the session cookie, mutating operations require matching Origin or same-origin browser metadata. Authentication is necessary but not sufficient for public access: rate limits, HTTPS, audit logs, account recovery, session persistence choices and security assessment must be accepted separately.

## Private audit trail and offline administrator recovery (P4-AUTH-3)

### Structured account security events

Only when `ANYFLIP_AUTH_MODE=accounts` is enabled, the Go server writes JSON Lines to `/data/jobs/.anyflip-auth-audit.jsonl` (permission 0600). Each entry contains a UTC timestamp, a fixed event name, sanitized actor/target account names and a result (`intent`, `ok`, `denied` or `failed`). The events include login success/failure, logout, user creation, password change/reset, suspend/enable and offline admin recovery. Requests, passwords, hashes, cookies, bearer secrets, source book URLs and network addresses are **not** written to this file. Malformed account names become `unknown`.

The active audit log is limited to **512 KiB** and rotates to exactly one `.1` archive (an additional at-most-512-KiB file). Audit history is therefore deliberately bounded and **not permanent**. Export logs privately before rotation if retention is required. There is no HTTP route to read the audit file. This is an **operational** log, not a tamper-evident/cryptographically signed or transactional audit system. For account changes and successful login, the `intent` record must be successfully written before execution; failure returns HTTP 503. A post-commit `ok` record is best-effort and a write failure is reported in the server log; it cannot undo an already committed account change. Operators should review audit availability and implement external append-only retention before public release.

### Emergency offline recovery of an existing administrator

There is **no browser or HTTP recovery endpoint** and no unauthenticated backdoor. Recovery requires privileged, out-of-band shell access to the private account store on the actual host.

1. Arrange a maintenance window; **stop the Web application and any other account-store writers first**. The account file and CLI do not support concurrent processes editing the same store. Preserve operator-specific Compose overlays, ext4 hard quota, volume mappings and systemd service.
2. Back up the existing `/data/jobs/.anyflip-accounts.json`, private audit logs and storage mount metadata separately with permission 0600, and ensure the backup can be restored. Confirm the intended `/data/jobs` is the mounted quota-aware data directory before any recovery. Never publish the account store.
3. Use the reviewed Go binary inside a controlled maintenance context with the same mounted data directory. Example command (only after stopping the Web process and confirming path access):

   ```sh
   read -rsp 'New admin password (12+ characters): ' RECOVERY_PASSWORD
   printf '\n'
   printf '%s' "$RECOVERY_PASSWORD" | ./anyflip-downloader -recover-admin admin -recovery-dir /data/jobs
   unset RECOVERY_PASSWORD
   ```

   The password enters through stdin, not command-line arguments, .env, GitHub or URLs. Replace `admin` only with the username of an **existing** administrator. The command refuses an absent account database or regular user. It makes an exclusive, private `/data/jobs/.anyflip-accounts.pre-recovery.bak` snapshot, resets that admin's Argon2id hash and enables the existing account. If a prior recovery snapshot exists, the command refuses to overwrite it; move it into independently protected backup storage **after verifying it**, before a second recovery. It also records offline recovery intent and outcome in the local audit log.
4. Verify permissions/backup, restart the Web service using the host's existing deployment workflow, sign in with the recovered password **only via HTTPS**, change the password again using the authenticated UI if required, and review the audit trail. A restart revokes all old process-local sessions.

**Limitations:** Offline recovery cannot repair a lost/corrupt account store, replace host backups, recover a missing machine token, or prevent an attacker with filesystem administrative access. Do not run against a live Docker service. Emergency console steps and audit backups remain operator acceptance gates.

## Testing and release gates

- Go tests: hash, persistence, login/logout, role denial, per-user listing, IDOR rejection, Admin access, archived history and CSRF.
- Playwright: token-only mode remains covered; isolated account-mode browser test exercises login, admin creates user, logout and user UI.
- Physical acceptance **OPEN:** real desktop/mobile HTTPS browser, password rotation, user deactivation/reset, reverse proxy / Host handling, account store backup, real authorized book and quota-aware Docker restart. No deployment to operator machine has been performed by this PR.
