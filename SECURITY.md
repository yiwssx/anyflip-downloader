# Security policy

## Supported code

Security fixes target the current main branch. There is no promise of support
for historical commits, locally modified containers or unapproved builds.

## Private vulnerability reporting

**Do not** file public issues or pull requests containing exploitable details,
OAuth secrets, Cloudflare Tunnel credentials, live document URLs, user identities
or real private PDFs.

Use the [GitHub private vulnerability reporting entry point](https://github.com/yiwssx/anyflip-downloader/security/advisories/new)
when it is enabled. If it is unavailable, request a private reporting channel
from the repository maintainer without exposing exploit details in public.
Administrators must enable and periodically verify private vulnerability
reporting under **Settings → Security → Code security**.

Useful reports include the affected commit/version, non-sensitive reproduction
steps, impact and a redacted proof of concept. Do not test against the live
college service, other users' data or third-party sites without authorization.

## Security design boundaries

- Only process documents the operator is authorized to download or reproduce.
- Preserve explicit upstream host/IP allowlists, SSRF blocking, TLS validation,
  request limits and cancellation; do not bypass publisher access controls.
- Production sign-in uses verified Google Workspace identities for @rcat.ac.th
  with server-side RBAC and per-job ownership checks. A matching email suffix
  by itself is insufficient authorization.
- Keep credentials in operator-controlled private configuration, never in Git.
- Release candidates are manual and do not publish or deploy automatically.
- Operator-run quota volumes, port binding, Cloudflare Tunnel and DNS are
  intentionally outside GitHub Actions access.

See [authentication](docs/AUTHENTICATION.md),
[RBAC operations](docs/RBAC_OPERATIONS.md) and
[release gates](docs/RELEASE_CHECKLIST.md).
