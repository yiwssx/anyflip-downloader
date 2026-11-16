# ADR-SUI-001 — Svelte 5 + shadcn-svelte Design System (2026-11-16)

**Owner-requested change** tracked only in [SUI issue #130](https://github.com/yiwssx/anyflip-downloader/issues/130). Existing UIH-001's no-framework decision remains the **current production reality**; this ADR supersedes it **for the new optional frontend implementation only**, after a separate release decision.

## Decision and why

Use Svelte 5, TypeScript, Vite, Tailwind CSS v4, `shadcn-svelte` (new-york configuration, Bits UI primitives) and Lucide Svelte. A static build can later be embedded in the existing Go service without adding a Node runtime in production. Do not use Next.js, React, external CDN scripts/styles, a second authentication service or a separate SPA hosting domain.

This is a deliberate application redesign, not an exercise in recoding the existing Vanilla JS/CSS. SUI-01 is an isolated, navigable, no-data preview and carries **no claim of feature parity**. Real job and Admin UI migrations require their own API contract evidence before replacement.

## Visual system

| Semantic | Light | Dark |
| --- | --- | --- |
| Page | #F5F5F7 (approx.) | #1C1C1E (approx.) |
| Raised/card | #FFFFFF | #2C2C2E (approx.) |
| Text | high-contrast neutral | high-contrast near-white |
| Primary action | accessible system-blue inspired | lighter system-blue inspired |
| Success / warning / error | semantic, not decorative | semantic, not decorative |
| Typography | system UI, Thai-capable fallback | same |
| Spacing | 4/8/12/16/24/32px | same |
| Touch hit areas | >=44x44 CSS px | same |
| Radius | 12–20px common components | same |

Color source of truth: `frontend/src/app.css` using OKLCH `--background`, `--primary`, `--ring`, `--card`, etc. Both themes must maintain readable contrast. The institutional crest and official source logo are not altered by theme styling.

## Application architecture / trust boundaries

- `frontend/`: optional build candidate (current SUI-01 prototype, **not connected** to real APIs).
- `web/`: existing production UI, unchanged in the foundation PR.
- Existing Go backend remains the sole authority for `@rcat.ac.th` Google OIDC identity, session cookies, `user/admin/super_admin`, job ownership, quota, audit, output PDF and anti-SSRF policies.
- Svelte view state may choose navigation or show/hide admin options, **never** grant authorization.
- Later build integration must explicitly preserve strict same-origin CSP, no inline script/style/eval, no runtime CDN, non-cacheable HTML where needed and hashed asset content-type/refresh behavior. Every new asset served by Go must be allowlisted and embedded.
- Keep token-mode compatibility only as required by existing Go code contracts; do not expose Google token or private session state to JavaScript.

## Screen IA and interaction conventions

1. Mobile: four-item bottom nav for Overview, Download, History, Appearance; safe-area padding.
2. Desktop: accessible sidebar navigation; compact main content with clear hierarchy and grouped tasks.
3. User journeys: review/submit URL, stage timeline, progress, cancel/retry/download, batch operations, scoped history, exact errors.
4. Administration: role-aware navigation, separated modules, paginated tables, role-change confirmations, conflict feedback; **no giant never-ending dashboard**.
5. Accessible semantics: keyboard, skip navigation, aria-live for async status, focus management, reduced motion and contrast testing on changed screens.

## Bounded engineering gates (without repeating unrelated acceptance)

| Gate | Evidence required | Status at SUI-01 branch creation |
| --- | --- | --- |
| Foundation code | Svelte preview, semantic tokens, real source components, shadcn-svelte registry config | Implementing |
| Dependencies | reproducible lockfile, dependency review, `svelte-check` and Vite production build | Pending CI / lock |
| Frontend regression | Desktop/mobile screenshot and keyboard checks against preview | Pending |
| API parity / auth | API and Google/RBAC browser matrix for newly migrated views | Not in foundation scope |
| Deployment | Go embedding, strict CSP, same-origin cookies, backup and rollback | Not in foundation scope |
| Production | Owner-approved exact digest and host/device acceptance | Not authorized |

**Stop rule:** Do not mark this migration DONE from a successful foundation PR. A green preview build is not proof of end-to-end job, OAuth or RBAC parity. Do not reopen historical accepted UIH tasks simply because this new implementation exists.
