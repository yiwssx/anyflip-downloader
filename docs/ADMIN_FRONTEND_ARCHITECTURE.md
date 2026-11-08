# Admin UI architecture (UIH-04)
Tracking: [#98](https://github.com/yiwssx/anyflip-downloader/issues/98), Epic [#93](https://github.com/yiwssx/anyflip-downloader/issues/93)

## Separation of concerns
- `/admin-api.js`: small same-origin transport restricted to `/api/admin/*`. Returns successful JSON or throws errors with HTTP status; **not an authorization engine**.
- `/admin.js`: role-aware presentation, keyboard-friendly tables, confirmations, fetching state and focus return. On server 401/403 the privileged UI is hidden. Backend must still enforce each route and ownership.
- `/auth/me`: Google mode's server-verified current principal. `admin.js` checks it only for presentation and navigation; **never** for granting authority.
- `web.go`: explicit same-origin, embedded `admin-api.js` route only in Google mode; assets are public, privileged data endpoints are not.

## Contract and restrictions
- Keep `user/admin/super_admin` matrix unchanged. Only Super Admin can mutate roles or see privileged audit. Administrators never download another user's PDF by role alone.
- No remote frontend dependencies, inline scripts, localStorage credentials or relaxation of Content-Security-Policy.
- Preserve existing admin HTML IDs and API paths during extraction.
- Code-only architecture changes are **not** a production role, server, or Google Workspace rollout.

## Acceptance evidence
CI includes Go CSP/embedded assets + RBAC tests and token/Google browser E2E for responsive Admin flows. Manual focus/screen-reader/device and physical HTTPS checks remain required under #97/#101.
