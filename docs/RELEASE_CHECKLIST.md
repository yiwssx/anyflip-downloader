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
gh attestation verify ./anyflip-downloader_vX.Y.Z_linux_amd64 \\
  --repo yiwssx/anyflip-downloader \\
  --signer-workflow yiwssx/anyflip-downloader/.github/workflows/release.yml \\
  --source-ref refs/heads/main \\
  --source-digest <exact-github-source-sha>
```

The candidate archive itself is a transport wrapper; its ZIP hash is not the SHA-256 digest of a packaged binary. A successfully attested **CLI release candidate** is not a signed OCI Web GUI release, an approved Staging deploy, an operator health check, or proof of backup/rollback. Its existence cannot clear RCAT Deployment Control HDA-06 for other apps, HDA-04 staging/host gates or AnyFlip Production acceptance. This workflow must be explicitly dispatched from `main` for a specific candidate version; merging source changes alone does not generate a candidate artifact.

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
