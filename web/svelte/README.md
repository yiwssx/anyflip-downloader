# Svelte bundle placeholder (SUI-04)

This directory is deliberately **not** a compiled frontend in the Git repository. A pinned, isolated Svelte build creates `google.html`, `token.html` and fingerprinted `assets/` before the special candidate Go binary is compiled.

The default legacy UI remains unchanged. `ANYFLIP_WEB_UI=svelte` fails startup unless the embedded bundle is complete and correctly references same-origin `/svelte/assets/`.

Do not hand-edit hashed artifacts or use a writable host volume to serve static files. See `docs/SUI_SVELTE_BUILD_INTEGRATION.md`.
