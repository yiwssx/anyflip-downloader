---
name: anyflip-web-ui
description: Use for AnyFlip RCAT Web GUI design, Thai responsive layout, official branding, accessibility, administrator dashboard, JS/CSS changes and Chromium desktop/mobile browser E2E tests.
---

# anyflip-web-ui

## Design and implementation
1. Review web/index.html, web/style.css, web/app.js, web/google.js, web/admin.js and backend HTTP contracts before changing interfaces.
2. Maintain Thai-first copy, useful job stages/errors, keyboard focus, semantic controls, responsive layouts, reduced-motion support and helpful empty/permission-denied states.
3. Respect RCAT official identity: **never recolor or redraw the college crest as if it were the official emblem**. App-shell accents may use another palette. Keep the product icon visually distinct from the separate RCAT PDF HUB product.
4. Keep assets same-origin, no unnecessary CDN or runtime frontend dependencies. Do not weaken Content-Security-Policy to unsafe-inline or unsafe-eval.
5. Render role-specific navigation but enforce every privilege in Go too; UI hiding is not authorization.

## Verify
- node --check web/app.js; node --check web/google.js; node --check web/admin.js
- Run affected Go HTTP and web branding/CSP tests.
- Run tests/browser-e2e.mjs and tests/google-e2e.mjs through .github/workflows/browser-e2e.yml (pinned Playwright); verify Chromium desktop/mobile, both authentication modes and denial paths.
- Physical Android/iOS browser and screen-reader acceptance requires explicit host/device evidence; green Playwright is not a substitute.
