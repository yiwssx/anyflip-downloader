# AnyFlip Downloader — release acceptance checklist

**No public release is approved by this checklist or by GitHub Actions alone.**

Release-candidate workflow is explicitly manual and uploads nonpublic CI artifacts. It never creates a Git tag, GitHub Release, deployment, DNS record or public-facing endpoint.

## Automated gates

- [ ] GitHub `build` jobs pass on the exact candidate commit: build, vet, unit/race tests and Docker Compose boot/restart/API smoke
- [ ] Module checksums validate with `go mod verify`
- [ ] Reachability-aware Go vulnerability scan (`govulncheck`) passes; review any scan findings before approval
- [ ] Manual `Release candidate` run passes with an explicit `vX.Y.Z` or `vX.Y.Z-rc.N` identifier
- [ ] Download candidate archive from Actions and locally verify `SHA256SUMS` before trusting binaries

## Physical acceptance and operator authorization

- [ ] Publisher-authorized real-book complete PDF page-count/quality, crop, Thai legibility and mixed-orientation checks on at least two PDF viewers
- [ ] Physical browser QA on desktop and mobile, including start, cancel, failure and download flows
- [ ] Target Ubuntu Docker disk/volume quota is **actually** enforced; test low space, simultaneous downloads, restart cleanup and failure rollback
- [ ] Run `scripts/verify-web-deployment.sh` against the actual approved deployment; inspect logs privately for redaction
- [ ] Review LAN port `10.10.1.100:8094` and token handling. Do not enable public access without an independently reviewed HTTPS proxy, access control, trusted-client rate limiting, firewall and monitoring
- [ ] Obtain explicit operator approval to publish a specific version and record approval date, commit SHA, CI run IDs and redacted acceptance evidence

## Rollback

1. Preserve the existing private `.env` and the job volume before changing binaries or Compose.
2. Record the deployed Git commit and container digest (if applicable).
3. Build/deploy a previously accepted commit and rerun health, auth and file checks; do not execute `docker compose down -v`.
4. Check for new/old binary state incompatibility; never assume in-memory Web jobs survive restart.

See [P1 verification](P1_VERIFICATION.md), [deployment runbook](WEB_DEPLOYMENT.md) and [master tracker](DEVELOPMENT_TRACKER.md).
