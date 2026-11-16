# RCAT AnyFlip — Svelte 5 UI foundation (SUI-01)

**Status: SUI-01 accepted preview + SUI-02 isolated user-flow candidate (not served by Go).** This directory is intentionally isolated. The existing Go-embedded `web/` UI, Google Workspace authentication, RBAC, jobs, Docker services and live host remain unchanged.

## Stack

- Svelte 5 with runes + TypeScript, Vite, Tailwind CSS v4
- shadcn-svelte `new-york` registry config in `components.json`
- Local, source-owned Button/Card foundations under `src/lib/components/ui` following the shadcn-svelte composition conventions. The official shadcn-svelte CLI can add/replace further controls (Dialog/Sheet/Tabs/Select) in subsequent migration PRs.
- Bits UI and Lucide for accessible primitives and iconography
- Light-first, iOS-inspired **web** design, not Apple/iOS native controls

## Local preview

```sh
cd frontend
npm ci
npm run check
npm run build
npm run dev
```

The preview runs at the local Vite server only. It deliberately **does not** call the Go jobs/auth/admin APIs, display fabricated user data, or submit real downloads. Calls to action requiring those APIs are disabled. Use the appearance tab to change Light/Dark mode; desktop has a collapsible sidebar and mobile has four bottom tabs.

## Next migration gates (Issue #130)

1. SUI-01: Lockfile committed; CI uses `npm ci`. Chromium preview checks six widths (320, 390, 768, 1024, 1365, 1440), keyboard, Light/Dark, collapsible sidebar, no API calls, no horizontal overflow and severe/critical axe findings. Synthetic screenshots are uploaded for visual review. Require green exact-head checks before merging.
2. SUI-02: Bring existing token and Google transport behavior into Svelte and implement real job workflows with matched error/retry/cancellation semantics. Do not substitute client-only authorization.
3. SUI-03: Port Admin Console including role checks, 409/428 conflict and audit presentation, unchanged Go ownership/permissions.
4. SUI-04: Embed built hashed assets via Go under the **existing same-origin and strict CSP**. Plan `connect-src`, `script-src`, `style-src`, path refresh, Vite asset paths, and rollback carefully; do not turn on Vite HMR on Production.
5. SUI-05: Review real before/after screenshots desktop/mobile, 320–1440px, real keyboard/voiceover, production HTTPS session behavior, and operator-authorized cutover.

**Never alter** `compose.quota.yaml`, `anyflip-quota.service`, private RBAC store, job volumes, OAuth secrets, Cloudflare Tunnel, DNS, existing LAN port or production images as a side effect of this work.

Follow [Issue #130](https://github.com/yiwssx/anyflip-downloader/issues/130), `AGENTS.md`, and [ADR-SUI-001](../docs/SUI_SVELTE_DESIGN_SYSTEM.md).

## Isolated functional entrypoints (SUI-02)

- `/index.html`: preserved SUI-01 visual-only preview.
- `/token.html`: legacy Bearer token user workflows (for synthetic/controlled local tests).
- `/google.html`: Google Workspace cookie user workflows, with real /auth/me handshake only when eventually served by Go.
- All three are static Vite outputs. Neither new functional entrypoint is wired into the Go `web/` production embed yet.
- SUI-02 synthetic CI stubs `/api/jobs*` and `/auth/*`; no real document, user cookie, OAuth client or production URL is contacted.
- Critical invariants: no cross-user cache persistence, no client-only RBAC grant, 32-hex job IDs, bounded backoff, cancellation of stale polls, explicit 401/403 and 429 handling, native PDF download.

To verify locally, run `npm ci && npm run check && npm run build`, then `npm run preview` and `npm run test:journeys` with Chromium installed.
