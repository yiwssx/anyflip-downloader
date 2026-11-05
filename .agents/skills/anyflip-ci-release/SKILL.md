---
name: anyflip-ci-release
description: Use when fixing GitHub Actions, Go build or Docker tests, dependency scans, branch/PR workflow, changelog, release candidate packaging and production readiness tracker updates.
---

# anyflip-ci-release

## Workflow
1. Read .github/workflows/build.yml, .github/workflows/browser-e2e.yml, .github/workflows/release.yml, docs/RELEASE_CHECKLIST.md and docs/DEVELOPMENT_TRACKER.md.
2. Check the exact main SHA, open PRs/issues and CI conclusions before calling any task complete. A local pass, PR-head pass and post-merge main pass are distinct evidence.
3. Keep go.mod authoritative for Go; run formatting, go build ./..., go mod verify, go vet ./..., go test -count=1 ./..., go test -race ./..., and vulnerability scanning per existing CI.
4. Trigger browser checks when Web UI/JS or auth behavior changes; Docker smoke for Compose, startup, volume or security adjustments. Do not silently disable a failing gate.
5. Use focused feature branches and PRs, then update tracker evidence and CHANGELOG.md with factual dates/links. Preserve legacy historical sections without counting retired auth features as open tasks.
6. Manual release.yml produces a candidate artifact only. Never create a public tag, GitHub Release, deploy, rotate secrets or enable DNS without explicit operator approval and verified release gates.

## Acceptance
Include exact PR/commit, Go/Docker/Browser outcomes, security warnings, remaining physical tests, operator sign-off and rollback path. Refer to docs/RELEASE_CHECKLIST.md.
