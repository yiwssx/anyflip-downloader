# SUI-04 — Opt-in Svelte bundle in the existing Go origin (2026-11-16)

Tracking: [#130](https://github.com/yiwssx/anyflip-downloader/issues/130). **Code-only candidate. NO Production rollout approved.**

## Invariants

- `ANYFLIP_WEB_UI` defaults to `legacy`, preserving the currently accepted `web/index.html` or Google `web/google.html`.
- `ANYFLIP_WEB_UI=svelte` is recognized **only** when a complete, same-origin bundle has been compiled into the Go binary. Unsupported values or incomplete bundles fail startup; no implicit fallback and no remote asset URLs.
- `/svelte/assets/<hashed-name>.js|css|...` is embedded and served read-only. Requests to other paths, directories, private source, config, or traversal are not served.
- All HTML, JS and CSS retain existing restrictive CSP (`script-src 'self'; style-src 'self'; connect-src 'self'`, no unsafe-inline/eval) and `Cache-Control: no-store`. OAuth session cookies, Google callback, server-side RBAC, per-user jobs/history, PDFs and quotas remain Go-owned.
- No new Node runtime in production. The optional **`Dockerfile.svelte`** builds with Node in a throwaway build stage and copies immutable Vite assets into the Go builder. The production `Dockerfile` is unchanged and cannot start in svelte mode without a bundle.
- Build assets are excluded by `web/svelte/.gitignore`; source-only repo keeps just a harmless placeholder. The shared `compose.web.yaml`, `compose.quota.yaml`, quota service, LAN host bind and offsite backups are untouched.

## Build & local smoke (not a Host deployment)

From a **disposable developer / CI checkout**, with pinned Node 22+, npm and Go installed:

```sh
bash scripts/build-svelte-embed.sh
go test -run '^TestWebSvelteFrontend' -count=1 ./...
go build -trimpath -o /tmp/anyflip-sui04-candidate .
```

The script executes `npm ci` then builds with `SUI_EMBED_BUILD=1`, copying generated `token.html`, `google.html` and hashed assets under `web/svelte/`. Local Go builds without these assets remain fully backward-compatible, and `ANYFLIP_WEB_UI=svelte` correctly fails closed.

`Dockerfile.svelte` is the **separate candidate builder** for controlled staging. Do **not** swap it into the existing Production Compose or deploy/change environment files in this phase. Do not run this build script inside `/opt/anyflip-downloader` on the live host.

## Verification and cutover hold

- SUI CI separately checks the original Svelte preview/user/Admin role matrix, then compiles an embedded bundle into disposable CI Go servers (token + Google synthetic modes), checks hashed asset URLs, CSP, MIME, no-store, legitimate health and anonymous 401/403 boundaries.
- Existing repository Go security/race/vulnerability + Docker smoke and old UI Chromium/Firefox/WebKit E2E remain required because `web.go` routes change.
- Raw Google account/login, actual HTTPS reverse-proxy/Tunnel, real Android/iOS, backups/restore and rollback rehearsal are **SUI-05 Host/operator gates**, not CI evidence.
- **Future operator-only cutover:** explicitly select verified candidate image digest and `ANYFLIP_WEB_UI=svelte` in a controlled, approved stage/host window **without changing any existing OAuth secrets, role store, job volume, quota overlays, DNS or Tunnel**. Keep previous image/digest for immediate rollback. Restore the previously running image **and its previously selected frontend mode** on failure (Svelte stays Svelte after a branding-only update); never delete a volume.
- This document does not authorize Production restart/deploy. Do not claim completion of SUI-05 or of the full migration.
