# UIH Frontend Hardening — baseline and architecture
_Tracking: [Epic #93](https://github.com/yiwssx/anyflip-downloader/issues/93); [UIH-00 #94](https://github.com/yiwssx/anyflip-downloader/issues/94), [UIH-01 #95](https://github.com/yiwssx/anyflip-downloader/issues/95), [UIH-02 #96](https://github.com/yiwssx/anyflip-downloader/issues/96), [UIH-05 #99](https://github.com/yiwssx/anyflip-downloader/issues/99)._

## Architecture decision (ADR-UIH-001)
**Keep** Go-embedded same-origin Vanilla JS/CSS and the existing backend API. Do not add a frontend framework, remote assets or runtime CDN. Maintain both token mode (legacy) and Google Workspace cookie mode (production) using separate transport adapters but **one** workspace controller. Frontend role hints do not grant privileges; Go enforces RBAC and per-job ownership. Admin operations remain separate in `admin.js`.

```text
HTML: index.html OR google.html
  ├─ style.css        (canonical design tokens, legacy CSS cascade retained for review)
  ├─ workspace.js     (job state, progress, history, batch UI, cancellable polling)
  └─ app.js           (token adapter)
     OR google.js     (Google HttpOnly session adapter)
       └─ admin.js    (role-aware presentation ONLY; /api/admin/* checks in Go)
                       ↘ same-origin /api/jobs/* & /auth/* served by Go
```

`web.go` maps assets explicitly and sets strict CSP (`script-src 'self'`, `style-src 'self'`; no unsafe-inline/eval), no-store and other protective response headers. Google sessions use HttpOnly cookies and SameSite; the backend validates claims and authorization. The separate `/workspace.js` file is public JavaScript, not privileged API data.

## Baseline/contract inventory
| Surface | Transport / route | Trust and recovery |
| --- | --- | --- |
| Token entry | `index.html`, `app.js` | Legacy token in sessionStorage; checked by Go API; no RBAC |
| Google Workspace | `google.html`, `google.js` | verified `@rcat.ac.th` Google OIDC, HttpOnly session; 401/403 show sign-in |
| Shared workspace | `workspace.js` | `GET/POST /api/jobs`, batch, status, cancel, retry, own PDF |
| Admin | `admin.js`, `/api/admin/*` | `user/admin/super_admin`; role/owner enforced in Go, not via UI hiding |
| Static assets | `web.go` Go embed | same-origin CSP, explicit routes, no-store; no external CDN |

## UI state matrix
- **Jobs:** queued → preparing → downloading → converting → completed/failed/cancelled; archived jobs are history-only.
- **Download:** page count and `aria-valuenow` reflect progress; output is image-based PDF.
- **Recovery:** 429/5xx/network failure shows a message and bounded retry; 401/403 stops protected polling, and Google mode returns to sign-in.
- **Navigation:** overview, create, batch, history; Admin only for authorized roles. History filters are local to the returned owner-scoped list.
- **Layouts for verification:** 320, 390, 768, 1024, 1365, 1440 CSS px; table horizontal scrolling must stay contained.

## Security / quality baseline and release limitations
Existing tests include Go HTTP/CSP/auth/RBAC checks and mock-origin Playwright Chromium flows for both login modes and responsive widths. Some manual production/physical-device gates cannot be inferred from these tests: **HTTPS reverse-proxy headers, actual Workspace account sessions, two-account ownership/role checks, Android/iOS, keyboard/screen-reader acceptance, filesystem quota and recovery** all require collected host/device evidence at UIH-07. No host settings, OAuth credentials, Docker volumes, DNS, Tunnel or production data are altered by this refactor.

## Known follow-ups
1. CSS now has one canonical `:root`, but additional repeated legacy responsive rules remain pending **UIH-01** visual comparison and consolidation.
2. Accessibility automation, visual baselines and Firefox/WebKit remain **UIH-03/06** work; do not claim WCAG 2.2 AA compliance from basic ARIA or Chromium checks alone.
3. Admin presentation separation, confirmation UX and cross-role browser coverage remain **UIH-04**; backend RBAC enforcement must not change.
4. After code-only CI, **UIH-07** still requires separately authorized host/browser acceptance and rollback rehearsal.


## Implementation and verification addendum — UIH quality gates (2026-11-08)

- [PR #105](https://github.com/yiwssx/anyflip-downloader/pull/105) extracts canonical `web/tokens.css` and the Google-mode `web/admin-api.js` transport. Both authentication modes load `tokens.css` before `style.css`; Google mode loads `admin-api.js` before its Admin UI.
- A small Node policy check guards against accidental inline handlers/styles, asset load order changes, competing token roots, missing Go-embedded assets and weakening the source-level CSP. It supplements, not replaces, Go HTTP security tests.
- Chromium and Firefox CI collect **synthetic only** screenshots for named login/workspace/admin viewports (WebKit still runs functional/axe tests but screenshots are skipped because Playwright injects CSP-blocked inline styles) and upload them for seven days. Reviewers must inspect the actual artifacts; **this is evidence capture, not automatic golden-image regression**.
- Additional negative browser paths exercise Google session expiry during polling and Admin API role revocation. CI test credentials and email addresses are synthetic, never actual Workspace users.
- A successful automated axe audit only establishes the scope of the selected rules and pages, **not full WCAG 2.2 AA compliance**. Actual accessibility on screen readers, physical Android/iOS and HTTPS reverse-proxy and quota checks are separately gated by `docs/UIH_RELEASE_ACCEPTANCE.md` (UIH-07 #101).

## Extended authenticated accessibility and malformed-response regression
- The GitHub Browser E2E fixture now audits serious/critical WCAG 2.x/2.2 violations not only on anonymous login and token mode but also on authenticated Workspace and Super Admin UI (synthetic data).
- Malformed status payload injection verifies a visible error, automatic bounded recovery and preserved final job state without unhandled page errors.
- These automated checks are **not** a claim of full manual WCAG compliance, production security or live account acceptance; retain UIH-07 as a separate gate.
