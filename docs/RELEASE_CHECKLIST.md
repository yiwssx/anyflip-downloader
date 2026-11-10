# AnyFlip Downloader — release acceptance checklist

**No public release is approved by this checklist or by GitHub Actions alone.**

Release-candidate workflow is explicitly manual and uploads nonpublic CI artifacts. It never creates a Git tag, GitHub Release, deployment, DNS record or public-facing endpoint.

## Automated gates

- [ ] GitHub `build` jobs pass on the exact candidate commit: build, vet, unit/race tests and Docker Compose boot/restart/API smoke
- [ ] Module checksums validate with `go mod verify`
- [ ] Reachability-aware Go vulnerability scan (`govulncheck`) passes; review any scan findings before approval
- [ ] Manual `Release candidate` run passes with an explicit `vX.Y.Z` or `vX.Y.Z-rc.N` identifier
- [ ] Download candidate archive from Actions and verify `SHA256SUMS` for all four target binaries
- [ ] Verify the GitHub-signed SLSA build provenance for each binary against `yiwssx/anyflip-downloader`, the trusted `.github/workflows/release.yml`, exact source SHA, and `refs/heads/main`; checksum text alone is not source provenance

- [ ] CI-only crash-orphan stage test passes after synthetic SIGKILL, and host-preflight shell syntax/read-only runner checks pass; no host proof is implied


## Attested candidate (manual pilot, no public release or host deployment)

The approved pilot reuses the existing **manual** `Release candidate` workflow. It is **not** run on pull requests, does not publish a tag, GitHub Release, GHCR image or deploy an application, and requires `main` for the candidate job. The job builds the four existing CLI components and uses `actions/attest@v4` (SHA-pinned) to issue signed SLSA provenance for the actual bytes. It then verifies all four file hashes from `SHA256SUMS` and each GitHub attestation before publishing a short-retention, unapproved Actions candidate artifact. The job has `contents:read`, `id-token:write`, `attestations:write`, **not** `packages:write` or `contents:write`; an untrusted PR never receives those candidate-job permissions.

The source identity and artifact digest are verified in the candidate job using `gh attestation verify` and the exact `GITHUB_SHA`; a distinct consumer must verify **downloaded bytes** again before using them. For an approved candidate downloaded from Actions, invoke (for each file):

```bash
sha256sum -c SHA256SUMS
gh attestation verify ./anyflip-downloader_vX.Y.Z_linux_amd64 \
  --repo yiwssx/anyflip-downloader \
  --signer-workflow yiwssx/anyflip-downloader/.github/workflows/release.yml \
  --source-ref refs/heads/main \
  --source-digest YOUR_VERIFIED_COMMIT_SHA
```

The candidate archive itself is a transport wrapper; its ZIP hash is not the SHA-256 digest of a packaged binary. A successfully attested **CLI release candidate** is not a signed OCI Web GUI release, an approved Staging deploy, an operator health check, or proof of backup/rollback. Its existence cannot clear RCAT Deployment Control HDA-06 for other apps, HDA-04 staging/host gates or AnyFlip Production acceptance. This workflow must be explicitly dispatched from `main` for a specific candidate version; merging source changes alone does not generate a candidate artifact.

## Reuse signed CLI binary for isolated Web staging (source path only)

The signed `linux-amd64` candidate from [Run #42749720972](https://github.com/yiwssx/anyflip-downloader/actions/runs/42749720972) **also contains the Web GUI**: the same Go program accepts `-web`, and `web.go` embeds all Web assets. This avoids a new Go compilation or artifact-publishing workflow merely to prove Web Mode. A disposable, isolated offline smoke of the **actual downloaded binary** at source `bc72fe9c959ca91c61557b41712ce0ed5b9e1c79` returned HTTP 200 for `/healthz` and `/`, and HTTP 401 for an unauthenticated API request. No RCAT Host was accessed.

For a future **separately authorized Staging executor**, `scripts/prepare-signed-web-staging.sh` accepts only an `anyflip-downloader_v*_linux_amd64` file, a 40-hex source commit and `sha256:<64hex>` file hash. It verifies the actual binary bytes **and** the GitHub signature for this repository, the trusted `.github/workflows/release.yml`, `main`, and that exact commit with `gh attestation verify`; the candidate is first copied into a private temporary context, and the copied bytes are checked before use; failure cleans the context. Successful verification leaves that isolated context and an audit pair for a separately authorized staging executor. It does **not** build or start Docker or read host credentials. `deploy/staging/Dockerfile.signed-web` provides the same Alpine non-root runtime, CA certificates and Web entrypoint as the existing application Dockerfile, but copies only the already-verified executable rather than rebuilding Go source.

The reviewed pilot binary hash is `sha256:dc7378bcb7fbccda147c139d2f0b9f1dfcd6f03f2d5bfa64722e63f493735183` for the above source SHA. These are **historical pinned evidence**, not an instruction to redeploy old code to Production; newer source needs new release evidence.

This is a **preparation recipe, not a signed OCI image**. The Alpine runtime base and built staging image still require their own reviewed immutable identity, an isolated authorized staging service/port/volume, backup/restore policy, rollback plan and post-deploy check before HDA-04 can accept a real deployment. The running Host may have operator-owned Compose/quota overlays, secrets and volumes: never overwrite them or run `docker compose down -v`. No Production, Cloudflare or DNS change is authorized by this source delivery. Maintain the existing main-only manual Release Candidate workflow; do not create a second publisher or an always-on deploy trigger.

## RCAT IOT isolated loopback Staging — host activation pending

**Owner decision (10 November 2026):** use the existing RCAT IOT Host `10.10.1.100` for a separate Staging instance only. No change to Production, Cloudflare, DNS, Google Workspace, quota service, or existing `compose.web.yaml` / `compose.quota.yaml` is allowed.

The operator-owned activation is `deploy/staging/isolated-anyflip.sh` and its dedicated `deploy/staging/compose.yaml`. It refuses any host not owning `10.10.1.100`, any preexisting Staging container/volume/network, a busy Staging port or an unrecognised local image. Its first activation generates a **new private token** under root-owned `/etc/rcat-staging/anyflip.env` (0600; containing no Production credentials). It uses only:

- Docker Compose project: `rcat-anyflip-staging`, with its own dedicated bridge network; external ingress is restricted to host loopback, while outbound networking is **permitted** (unlike the previously broken internal-only bridge)
- Host bind: `127.0.0.1:18094` exclusively; no external ingress/DNS
- Docker volume: `rcat-anyflip-staging-jobs`, never Production `anyflip-downloader_anyflip_quota_jobs`
- Signed Go binary repackaged as a **local-only** staging image, built with the reviewed non-root runtime Dockerfile and source revision label
- Synthetic health/unauthenticated API probes only: 200 for `/healthz`, 401 for protected job API; no real book data
- Fail-closed: stop **only** its own staging service if smoke fails, retain staging data volume, never use `down -v`.

**First-activation network compatibility (10 November 2026):** on Ubuntu 26.04 / newer Docker, using an `internal: true` network with a published `127.0.0.1:18094` port may leave the container running but make the host health probe unreachable. The first real activation on RCAT IOT **failed health and safely stopped only Staging**; its signed image, staging volume and independent token were retained. The repaired Compose uses a separate standard Docker bridge and still binds *only* to host loopback. Because the bridge permits **outbound** network access, treat it as a change of staging network policy; **do not** expose the staging port on `0.0.0.0` or point a public reverse proxy at it. No Production network is changed.

For this exact previously failed pilot, the operator can update only the isolated source checkout then invoke `sudo sh deploy/staging/isolated-anyflip.sh retry rcat-anyflip-staging:sha-bc72fe9c959ca91c61557b41712ce0ed5b9e1c79`. The `retry` action requires the original single **stopped** staging container, exactly matching pinned image, dedicated Compose-owned network and volume, and root-owned secret. It recreates only the stage container/network via `docker compose down` **without `--volumes`**, preserves `rcat-anyflip-staging-jobs` and `/etc/rcat-staging/anyflip.env`, and then applies the same stage health and unauthenticated API checks. This is not a generalized Production redeploy or data migration. If those exact conditions are missing, it refuses and the operator should inspect that difference rather than reset or delete anything.

The staged image is **not created or signed by these source files alone**. A *separately authorized Host executor/operator* must use the signed binary from [Run #42749720972](https://github.com/yiwssx/anyflip-downloader/actions/runs/42749720972) (source SHA `bc72fe9c959ca91c61557b41712ce0ed5b9e1c79` and Linux AMD64 binary SHA-256 `dc7378bcb7fbccda147c139d2f0b9f1dfcd6f03f2d5bfa64722e63f493735183`), run `scripts/prepare-signed-web-staging.sh` to independently verify *those same bytes*, and build the local tag `rcat-anyflip-staging:sha-bc72fe9c959ca91c61557b41712ce0ed5b9e1c79` using `deploy/staging/Dockerfile.signed-web` with `--build-arg SOURCE_COMMIT=bc72fe9c959ca91c61557b41712ce0ed5b9e1c79`. The image's revision label is a local safety check, **not** a cryptographic signature of the complete OCI runtime. Record the exact local image ID before running `isolated-anyflip.sh activate <pinned-tag>` on the Host. `status` is read-only; `stop <pinned-tag>` stops the staging container without deleting volumes.

**Operational gate:** Source delivery and CI alone do not install this stack on the RCAT IOT server. The current ChatGPT GitHub connection has no privileged Host terminal; it must not use the Read-only Diagnostics Agent to run deployment commands. A separate authorized executor identity, host preflight, saved rollback image and live staging/restore proof are still needed for HDA-04. Once the staging pilot is accepted, further deployment authorization must bind to real signed artifact bytes and a fresh approval, not an Issue-supplied tag or unsiged image label. Previous Read-only Diagnostics and existing Production acceptance are not repeated.

## Restricted Staging Hook — explicit local bootstrap (not yet ChatGPT execution)

The owner approved a **separate restricted Staging executor** on RCAT IOT `10.10.1.100`. This phase provides the **host-side root-owned fixed hook**, not a network listener, GitHub request consumer or authorization bypass. The existing Read-only Ops Agent and its GitHub App credentials are unchanged.

The one-time operator bootstrap installs a root-owned `/usr/local/libexec/rcat-staging/anyflip-downloader-stage` plus a reviewed root-owned Compose copy. It also creates a distinct non-login `rcat-stage-exec` OS identity with **no** Docker or sudo group, and a root-only `/etc/rcat-staging/anyflip-approved-image` pin to the **already running, previously signed and approved** AnyFlip staging image. It first requires the existing stage health=200 and unauthenticated API=401, exactly `127.0.0.1:18094`, the existing dedicated project/volume and the expected source SHA. Nothing is pulled, rebuilt, restarted, deployed, deleted or granted a sudo rule during installation.

From the host operator's **already accepted isolated source checkout** after the GitHub PR is merged, use a reviewed exact main commit:

```bash
SRC="$HOME/rcat-anyflip-staging-source"
git -C "$SRC" pull --ff-only origin main
sudo bash "$SRC/scripts/install-restricted-staging-hook.sh" \
  --approve-restricted-staging-hook "$SRC" EXACT_MERGED_MAIN_COMMIT_SHA
```

The expected result is `RESTRICTED_STAGE_HOOK_INSTALLED`. If any precondition fails, do not reset the existing volume or change the Production Compose. This bootstrap is intentionally **one-time and no-clobber**. It does not alter `/opt/anyflip-downloader`, Production, live Staging Container, `/etc/rcat-ops`, DNS, Cloudflare or existing GitHub App credentials.

The fixed hook accepts **only** `deploy <40hex source SHA> sha256:<local Docker image ID>`, and only when both exactly match its root-managed image lock. It refuses an unknown host, different image, additional staging containers or unsafe permission/port/volume conditions; when already healthy, it **does not restart**. It cannot upgrade to an unreviewed version or perform rollback. The hook is not exposed through sudoers or a service, and the dedicated user cannot invoke privileged Docker operations. `run_fixed_staging_hook` in RCAT Deployment Control remains nonexecuting unless invoked by a **separately authorized** trusted caller after independent GitHub approval, signed artifact verification, replay consumption, stage recovery and access review.

**Not yet installed via this ChatGPT session.** No SSH/Host command capability is connected. After operator install, record one redacted `RESTRICTED_STAGE_HOOK_INSTALLED` result; do not repeat CI, source attestation or the previously accepted Staging startup check. Subsequent privileged ChatGPT-triggered deploy/rollback requires a distinct GitHub executor credential, an authenticated short-lived owner approval, a securely bound real artifact/image release and a tested rollback. Never grant the diagnostics service account a Docker socket, general sudo, an arbitrary command interface or access to Production.

## Physical acceptance and operator authorization

- [ ] Publisher-authorized real-book complete PDF page-count/quality, crop, Thai legibility and mixed-orientation checks on at least two PDF viewers
- [ ] Physical browser QA on desktop and mobile, including start, cancel, failure and download flows
- [x] Operator-confirmed LAN migration into a separate size-bounded ext4 filesystem: active Docker named volume reports **5.8 GiB** at `/data/jobs`, matching the 6 GiB loopback image; systemd mount dependency, container health and HTTP 200 were observed. **Scoped LAN infrastructure task closed 2026-11-01.** See [host acceptance guide](HOST_ACCEPTANCE.md).
- [ ] Controlled ENOSPC/low-space, simultaneous downloads, real interruption/retention, mount-loss fail-closed behavior, and recovery/rollback drill remain **unverified release gates**. This checkbox is not satisfied by `df` capacity or healthy startup alone.
- [ ] Run token-free `scripts/verify-host-readiness.sh` from the actual Ubuntu deployment directory; inspect logs privately for redaction. Optional `scripts/verify-web-deployment.sh` authenticated probes require loopback/HTTPS, never plain LAN HTTP.
- [ ] Review LAN port `10.10.1.100:8094` and token handling. Do not enable public access without an independently reviewed HTTPS proxy, access control, trusted-client rate limiting, firewall and monitoring
- [ ] Obtain explicit operator approval to publish a specific version and record approval date, commit SHA, CI run IDs and redacted acceptance evidence

## Rollback

1. Preserve the existing private `.env` and the job volume before changing binaries or Compose.
2. Record the deployed Git commit and container digest (if applicable).
3. Build/deploy a previously accepted commit and rerun health, auth and file checks; do not execute `docker compose down -v`.
4. Check for new/old binary state incompatibility; never assume in-memory Web jobs survive restart.

See [P1 verification](P1_VERIFICATION.md), [deployment runbook](WEB_DEPLOYMENT.md) and [master tracker](DEVELOPMENT_TRACKER.md).
