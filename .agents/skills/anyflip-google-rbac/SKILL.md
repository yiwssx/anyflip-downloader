---
name: anyflip-google-rbac
description: Use for Google Workspace OAuth login, session cookies, admin console permissions, RBAC role persistence, super-admin bootstrap, per-user job ownership and privileged audit changes.
---

# anyflip-google-rbac

## Security invariants
- Production auth is Google Workspace only: verified @rcat.ac.th email AND verified hosted-domain hd claim, plus valid Google issuer, audience, signature, expiry, nonce/state/PKCE. A login hint or email suffix by itself is insufficient.
- Default role is user. Privileged roles are enforced server-side and are not equivalent to Workspace domain administrator rights.
- First super_admin is created by verified offline bootstrap, not by an unauthenticated API or automatic email match. Role store permissions are strict (0700 directory, 0600 regular file).
- Admin oversight is not permission to download another user's PDF. Validate user ownership on every job, cancellation, history, download and administrative operation.
- Keep authorization and audit fail-closed for unsafe persisted state. Do not log or publish secrets, OAuth codes, tokens, session cookies or identityRef values.

## Workflow
1. Review docs/AUTHENTICATION.md, docs/RBAC_ARCHITECTURE.md, docs/RBAC_OPERATIONS.md and docs/GOOGLE_CUTOVER.md.
2. Trace handlers and stores in web_google*.go, web_admin*.go, web_rbac*.go, web.go and related tests.
3. Add negative/privilege-boundary tests: unverified accounts, non-RCAT domain, cross-user access, stale session, role changes, store corruption, audit failure and concurrent mutations.
4. Run Go build/vet/unit/race and the Google browser E2E workflow where impacted. Confirm exact-commit CI before release.
5. Treat operator-reported November 2026 acceptance as attestation, not permission for automated live OAuth or production role-store changes.
