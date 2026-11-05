## Summary
<!-- What changed, why, and which issue/tracker entry is affected? -->

## Scope and risk
- [ ] Production deployment, service restart, DNS/Tunnel, quota and host-local Compose are unchanged
- [ ] Auth/RBAC, SSRF/network boundaries, PDF output and persistence reviewed if affected
- [ ] No secrets, token values, user book URLs, private PDFs or personal data included
- [ ] Compatibility, rollback and documentation impacts described

## Validation (record results, not assumptions)
- [ ] gofmt, go build ./..., go vet ./... (if Go changes)
- [ ] go test -count=1 ./... and go test -race ./... (if Go changes)
- [ ] Applicable browser/Google mode, Docker Compose, security and governance CI
- [ ] Required branch checks green on the exact PR head SHA

## Deployment/acceptance
<!-- Neither merging nor running GitHub Actions deploys production. -->
- Deployment required? No / Operator-controlled separately
- Operator approval, physical QA and rollback evidence (when applicable):

## Issue / tracker
<!-- Closes #... only for fully completed scope; otherwise use References #... -->
