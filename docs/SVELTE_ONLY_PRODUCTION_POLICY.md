# Remove legacy fallback from the approved Svelte production binary

## Confirmed why legacy returned after reboot

The exact operator-owned `/etc/systemd/system/anyflip-quota.service`
originally called:

```text
docker compose -f compose.web.yaml -f compose.quota.yaml up --no-build --no-color anyflip-web
```

This omitted any overlay setting `ANYFLIP_WEB_UI=svelte`. The observed
container image ID was **already** the previously accepted Svelte image
(`sha256:80056ad0eb1908fedd70eb5f86857b583dc2385a6e2e1f91e2d19f93bc9223d3`),
but the environment contained **neither** `ANYFLIP_WEB_UI=svelte` nor
`ANYFLIP_WEB_UI=legacy`. The Go server chose legacy by default. This
was a frontend selection failure, not loss of the Svelte image, source
rollback or browser cache.

## New policy

Svelte production images built from `Dockerfile.svelte` now compile Go
using `-tags=svelte_only`. The build tag sets `requireSvelteUI=true`
**at compile time**.

| ANYFLIP_WEB_UI | Production Svelte-only image | Plain local Go test/legacy compatibility build |
| --- | --- | --- |
| unset / empty | **Svelte** | legacy (legacy test fixture only) |
| `svelte` | **Svelte** | Svelte if assets valid |
| `legacy` | **startup error** | legacy (compatibility only) |
| any other value | startup error | startup error |
| Svelte assets missing/corrupt | **startup error** | error if explicitly requesting Svelte |

This is a deliberate **fail-closed** rule: a broken frontend must not
quietly display the retired login UI. Google Workspace verification,
server-side RBAC, CLI download behavior, PDF/history/quota and Cloudflare
routing are unchanged.

The runtime binary is self-contained and includes the correct Vite
assets; no Node process is needed after build. The production build-tagged
`web_assets_svelte_only.go` embeds only the Vite Svelte bundle and the
official institutional logo/favicon. Unlike the plain Go compatibility build,
it **does not embed legacy HTML, JavaScript or CSS at all**. Retired
`/app.js`, `/tokens.css`, `/admin.js` and related static routes return
404 in Svelte mode; Google OAuth and RBAC API routes remain active.

Its OCI label
`th.ac.rcat.anyflip.frontend=svelte-only` can be inspected without
exposing an OAuth secret. The default, plain Go build and legacy HTML
assets are retained **in the Git source tree and default local test build**
only to avoid invalidating pre-existing CLI/Go tests. They are **excluded
from the Svelte-only production binary** and cannot be selected or served
by its HTTP UI. This change does not erase Git history or delete those
local testing fixtures.

## Release order (do NOT reuse the old image as proof of this policy)

1. First complete the separate **owner-owned quota startup** correction
   in PRs #140–#141 using
   `scripts/install-svelte-quota-startup.sh` and the authorized
   one-time `scripts/apply-svelte-quota-startup.sh --apply`. Confirm
   `QUOTA_SVELTE_APPLY=PASS`. This prevents future boot commands from
   selecting the old mode even with the existing already-approved image.
2. Only afterward, build a **new** image from this accepted source via
   `Dockerfile.svelte` and complete the existing signed / operator
   release and pinned-image re-pin process. The older source-image ID
   `sha256:80056...` was built before the new compile-time policy and
   **does not** inherit it through a Git pull or unit reload.
3. At a maintenance window with no active downloads, cut over **only**
   `anyflip-web`, preserve the private `compose.quota.yaml`,
   `/mnt/anyflip-quota-stage`, mounted `/data/jobs`, existing Google
   OAuth configuration, private RBAC store and public DNS. Verify source
   SHA, new image ID, `th.ac.rcat.anyflip.frontend=svelte-only`,
   `ANYFLIP_WEB_UI=svelte`, health and anonymous API denial.
4. Preserve the previously accepted image for rollback, but do not
   enable the old `rcat-anyflip-svelte-boot.service` that was
   demonstrated to race Docker startup. Reboot proof remains pending
   the next normal maintenance reboot.

A production image may be configured with `ANYFLIP_WEB_UI=legacy`
only to **fail startup**; it cannot serve the old HTML.

## Evidence

The required `build` GitHub Actions workflow checks the two Go build
configurations and runs a targeted test for the Svelte-only policy;
Svelte's existing integration workflow checks the real embedded assets.
GitHub CI proves source/build behavior, **not** that the Host has
already replaced its pre-policy image.

The separate host-wide `containerd` startup timeout identified on
2026-11-18 is **not** repaired by this frontend change.
