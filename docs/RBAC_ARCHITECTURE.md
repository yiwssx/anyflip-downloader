# Google Workspace RBAC — Security design and API contract

**Issues:** #69 parent, #70 policy, #71 persistence. **Status:** code merged to `main` via PRs #79–#85; NOT production-enabled. Google SSO operator cutover #68 remains a separate mandatory gate.

## Identity and privilege boundaries

- Browser authentication remains **Google OIDC only**. Existing verifier checks RS256 JWKS, issuer, audience, subject, nonce, expiry, `hd=rcat.ac.th`, verified email and `@rcat.ac.th`. Authorization binds to the existing `g-<SHA256(canonical_issuer|sub)>` identity reference; email is display-only.
- `user` is the default for every newly verified Workspace identity. Neither the first sign-in nor an email claim can grant administrator access. `super_admin` is an application role, not a Google Workspace domain-admin role.
- Token mode stays available for pre-existing installations; its bearer token never inherits Google RBAC. Only the Google mode can enable RBAC, and only after operator-controlled initialization.
- RBAC is **opt-in**, `ANYFLIP_RBAC_ENABLED=1`. The default is disabled to preserve current running containers. Without an initialized, valid private store, enabled RBAC refuses startup rather than resetting privileges.
- Every protected request must recompute the caller's current role from the server registry, not the browser or an old session role. A role change therefore takes effect for existing Google sessions without logging out.

## Permission matrix (v1)

| Operation | user | admin | super_admin |
| --- | --- | --- | --- |
| Google SSO login / logout / own identity | yes | yes | yes |
| Create single/batch jobs | own | own | own |
| View job listing, status, archived history | own | own | own |
| Retry, cancel, download | own | own | own |
| Aggregate metrics and redacted cross-user job status | no | yes | yes |
| Cancel another user's active job | no | audited | audited |
| List verified users and role (minimal fields) | no | read-only | yes |
| Change application role | no | no | yes |
| Read privileged audit | no | no | yes |
| Download another user's PDF | **never** | **never** | **never** |
| Manage Workspace, DNS, Tunnel, Docker, secrets or host | never | never | never |

All privileged operations use dedicated `/api/admin/*` endpoints. Normal job routes stay owner-scoped even for super_admin. Cross-account status/retry/download on normal routes must return 404 to avoid ID enumeration. Unauthenticated APIs return 401; authenticated insufficient role returns 403; unknown roles fail closed.

**Implemented API contracts** (PRs #80, #84 and #85): `GET /api/admin/summary`; `GET /api/admin/jobs?limit=50&offset=0`; `POST /api/admin/jobs/{id}/cancel` with bounded reason category; `GET /api/admin/users`; `PUT /api/admin/users/{identityRef}/role` with validated role; `GET /api/admin/audit` with a bounded, redacted result. All mutations require same-origin protections and reliable audit before committing the privileged action.

## Store architecture and security

- Private `/data/jobs/.anyflip-rbac.json` in the **existing durable job volume**. No new volume, database, network service, reverse proxy, DNS setting, image-data migration or hard quota mount alteration.
- The store contains only versioned, bounded account metadata: irreversible identity reference, display email, role, disabled flag and last login. Never Google tokens, OAuth code, cookies or book URLs. JSON is validated for known roles, duplicate IDs and the existence of an active super_admin.
- Enforce private directory permissions (no group/other access) and regular `0600` store; reject symlinks and malformed records. Save using a same-directory temporary file, fsync, atomic rename and directory sync. The previous complete image is stored as a private `.bak` file. No silent recovery from a corrupt store.
- Updates are serialized with a mutex and tested for concurrency. The previous known good file is backed up on registration and role-store updates. Operator backups must remain off-volume and encrypted; `.bak` is only a convenient local rollback snapshot and may be in the same failure domain.
- Scope limitation: the current registry is intentionally in-process. **Do not run multiple writing Web replicas** until a cross-process transactional store/locking solution is introduced.

## Operator bootstrap (one-time, code-ready, NOT yet production)

1. Finish Google Workspace SSO rollout and verify a designated person's account via Google OIDC (issue #68). With RBAC disabled, the verified user opens `/auth/me` in their signed-in browser and obtains `identityRef`. Cross-check the returned institutional email and the intended Workspace account. Never use the displayed email itself as an RBAC key.
2. Ensure the existing job volume is backed up, persistent, and owned by the non-root app process. Confirm its directory is private (`0700`) before enabling RBAC; don't change mounts or delete volumes.
3. During an authorized maintenance window, execute the application binary **offline** with `-rbac-bootstrap-owner <verified_identityRef>` and `ANYFLIP_JOB_DIR=/data/jobs` against the existing mounted volume. This creates the first role store once and refuses overwrites. It is not available over HTTP.
4. Confirm the file is regular and `0600`, with exactly one expected super_admin. Only then set `ANYFLIP_RBAC_ENABLED=1` and restart the Web process using the host's existing Compose **plus quota overlay**. Avoid `down -v`. Newly authenticated, non-admin Workspace accounts remain `user`.
5. Recovery: restore a reviewed, operator-kept private backup with the app stopped, verify file ownership/mode and restart. Never rerun bootstrap over an existing store; do not expose a break-glass HTTP endpoint.

**Caution:** `identityRef` is evidence derived from a verified session, not an OAuth token. The offline bootstrap program validates its format and relies on the operator to authenticate the recipient out-of-band; never copy an arbitrary value from an untrusted user.

## Threat model and acceptance gates

- **IDOR:** cross-user PDF, archived history, batch, retry and cancellation cannot be retrieved through owner routes.
- **Privilege escalation:** no automatic first-user administrator, client-side role header, email alias promotion, self-promotion or last-super-admin demotion. No use of legacy service tokens in Google mode.
- **Request forgery and replay:** verify origin/fetch metadata on mutations, one-time Google state/nonce, canonical HTTPS host. The origin proxy must overwrite forwarding headers rather than pass client-supplied ones.
- **Store corruption / races:** missing, symlinked, world-readable, malformed or unknown-role record fails closed, with concurrency and restart tests.
- **Privacy:** privileged audit logs (RBAC-6) may include event type, actor/target identity reference and time, but must omit passwords, secrets, OAuth codes, source URLs and PDF content.
- **Release:** require Go build/vet/unit/race/security, Docker/Compose and desktop/mobile RBAC E2E, two real institutional accounts and negative Gmail checks, operator backup/restore drill, last-admin recovery, quota persistence, preserved `10.10.1.100:8094`, Cloudflare Tunnel and `www.rcat.ac.th`. None of these host checks can be proven by code review alone.

**Release gate:** #68 must be accepted before RBAC is turned on in production. No host deployment, `.env` mutation, quota migration or Google credential changes were performed by the RBAC repository implementation.
