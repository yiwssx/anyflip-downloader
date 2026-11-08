# UIH-01 — CSS layering and maintenance contract

Tracking: [UIH-01 #95](https://github.com/yiwssx/anyflip-downloader/issues/95), [Epic #93](https://github.com/yiwssx/anyflip-downloader/issues/93).

## Cascade order (preserves the previous concatenated stylesheet order)
| Order | Token Mode | Google Workspace Mode | Scope |
| --- | --- | --- | --- |
| 1 | `tokens.css` | `tokens.css` | Shared canonical colors, spacing, typography/focus tokens |
| 2 | `style.css` | `style.css` | Common site shell, forms, downloads, history, sign-in, original core responsive rules |
| 3 | — | `admin.css` | Admin-only UI and its original responsive rules; endpoint served only in Google mode |
| 4 | `readability.css` | `readability.css` | Shared typography, spacing and responsive readability refinements (existing cascade tail) |

**Behavior-preservation method:** this change moves contiguous unmodified CSS segments, preserving declaration order for each active page. Token mode does not load Admin styles because no Admin elements are served. No selectors, values, colors or official RCAT artwork changed. Go embeds all assets and serves them with the existing strict CSP and no-store policy.

### Developer rules
- Add new shared colors/font/spacing tokens to `tokens.css` only; do not introduce competing `:root` declarations.
- Add base component rules to `style.css`, Admin-specific rules to `admin.css`, and global final readability adjustments to `readability.css`. Avoid introducing new fallback rules in more than one file.
- Maintain stylesheet order: `tokens.css → style.css → [admin.css] → readability.css`. `scripts/check-frontend-policy.mjs` and Go tests enforce this contract.
- Do not weaken CSP or use inline HTML styles/handlers. Admin CSS is **not** authorization; backend RBAC remains authoritative.
- CI Browser E2E covers desktop and mobile in Chromium, Firefox and WebKit. Screenshot artifacts are synthetic review evidence, not pixel-perfect golden tests.

### Remaining UIH-01 debt
The existing core and final readability layer still have older overlapping responsive breakpoint rules, deliberately preserved to prevent unreviewed layout changes. The eventual clean-up requires a viewport-by-viewport visual comparison and operator acceptance. **This refactor is not an assertion that all CSS overrides have been removed or that physical/mobile UX acceptance passed.**
