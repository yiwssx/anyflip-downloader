# Changelog

All notable changes in this fork will be recorded here. Dates refer to source changes, **not** approved public releases.

## Unreleased (2026-10-28)

- Authenticated Thai responsive Web GUI and cancellable job lifecycle, while keeping the original CLI available.
- Download security hardening, outbound TLS/DNS restrictions, fail-closed missing/corrupt page handling and atomic image-based PDF publication.
- Image fidelity regressions for JPEG, PNG, WebP, page geometry, EXIF orientation, publisher-declared 404/410 fallback, optional highest-pixel-count image selection, and a synthetic 129-page book.
- Non-root Docker/Compose runtime, controlled stop/restart, one-hour artifact retention and privacy-safe job diagnostics.
- Web-only 512-page and 512 MiB per-job checkpointed storage budget, image-dimension validation and failed-token throttling.
- P3-J2 **code-only** aggregate managed-volume accounting, free-space admission and concurrent job reservations; an actual Ubuntu filesystem hard quota is still pending.
- Manual release-candidate packaging, version validation, checksum manifest and security-gated CI workflow. No production release claimed.

The software produces an **image-based PDF**, not the publisher's original text/vector PDF. Public-facing deployment, live book/browser acceptance, filesystem quotas and operator authorization are still pending.
