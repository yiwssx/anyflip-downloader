# UIH-00 — Frontend risk register & ownership (2026-11-08)

Tracking: [Epic #93](https://github.com/yiwssx/anyflip-downloader/issues/93), [UIH-00 #94](https://github.com/yiwssx/anyflip-downloader/issues/94). Code and synthetic CI are **not** live-host verification.

| Risk and evidence | Severity | Remediation owner and disposition |
| --- | --- | --- |
| Owner-scoped DOM/form/history can survive Google logout; delayed job response can reopen prior job (`web/workspace.js`) | High, confidentiality | Frontend owner: reset on logout and reject responses from a previous generation; browser CI test. Backend owner continues enforcing real job ownership. |
| Client-side Admin can be mistaken for authorization authority (`web/admin-api.js`, Go RBAC) | High, authorization | Backend/security owner: role and job ownership checked in Go. Frontend treats 401/403 as authoritative; negative API and browser tests. |
| Slow/failed/offline responses, 429/503, malformed stages, stale jobs (`web/workspace.js`) | Medium–High, reliability | Frontend owner: AbortController, single-flight polling, bounded backoff and generation checks. QA owner: synthetic browser fault injection. Actual Wi-Fi/offline under #101. |
| Legacy CSS cascades and responsive drift (`web/style.css`, `web/tokens.css`) | Medium, UX | Design-system/QA owner: canonical tokens and layered CSS; review screenshot diffs and touch targets. Visual approval still open #95/#100. |
| Playwright WebKit screenshots inject CSP-blocked styles | Medium, QA tooling | Browser QA owner: WebKit functional/axe tests remain mandatory, screenshot capture limited to Chromium/Firefox until compatible. Do not relax CSP. |
| Automated axe and synthetic keyboard checks alone cannot certify WCAG 2.2 AA | Medium, accessibility | Accessibility owner: screen reader and physical phone/keyboard checks #97/#101. |
| Host-local `compose.web.yaml` edits block `git pull`; replacing overlay could lose filesystem hard quota | Critical, operations | RCAT host operator: review/stash host override, preserve `.env`, `compose.quota.yaml` and service; run preflight before deploy. See `docs/UIH_DEPLOY_OPERATOR.md`. |
| Actual Google SSO, TLS/cookies, RBAC two-account isolation, quota-under-load and rollback rehearsal unverified for latest UIH build | Critical, release blocker | RCAT host operator: controlled acceptance and signed release gate #101. **No code merge can close this gate.** |

## Architecture / baseline references

- `docs/FRONTEND_ARCHITECTURE.md` records mode-specific routes, trust boundaries, state and responsive matrix, ADR and CI scope.
- [PR #102](https://github.com/yiwssx/anyflip-downloader/pull/102) through [PR #109](https://github.com/yiwssx/anyflip-downloader/pull/109) contain corresponding frontend, admin, accessibility, reliability and CSS changes.
- Source gates: `go test ./...`, `go test -race ./...`, `go vet ./...`, `node scripts/check-frontend-policy.mjs`.
- UI E2E in `.github/workflows/browser-e2e.yml` covers Chromium/Firefox/WebKit, token + Google, Admin role cases, WCAG automated subset and responsive testing.
- Live host evidence and approvals remain in `docs/UIH_RELEASE_ACCEPTANCE.md` and UIH-07 #101.
