# AnyFlip Downloader — repository guidance for coding agents

## Mission and architecture
- Maintain the Go CLI and self-hosted Go Web GUI. Image pages are assembled into an **image-based PDF**, never advertised as the publisher's source PDF.
- The authoritative current state is the code, docs/DEVELOPMENT_TRACKER.md, merged PRs and CI; historical checklist items about the removed local-account authentication are **not** current product requirements.
- Use Go version from go.mod, embedded Web files in web/, Node/Playwright browser tests in tests/, and the workflows under .github/workflows/.

## Human-triggered evidence-aware execution
- Read the live GitHub issue/PR state, target branch and exact-head CI before acting; `docs/DEVELOPMENT_TRACKER.md` provides history and supporting evidence, not permission to reopen completed work. The absence of open issues is not a request to invent a new one.
- Keep the owner's requested scope and Definition of Done fixed. Work within an existing approved issue/PR where applicable; do not manufacture new trackers, phases, workflows, broad refactors or repeated acceptance campaigns.
- Reuse successful evidence only for the same source commit, claim, configuration and trust boundary. For a changed surface run its relevant existing checks and all required PR checks; do not re-run unrelated Go, browser, Docker or RCAT host tests just to repeat prior proof. Existing mandatory security/Go checks in the Delivery contract still apply when relevant.
- Distinguish code merged, CI passed, and *operator-attested* Production acceptance. Do not treat CI as host proof, and do not silently reopen completed operator acceptance without a real regression.
- Stop at DONE when the approved criteria are evidenced; otherwise identify the precise BLOCKED or pending owner/host action. Retry only with an evidence-backed corrective change, and never grant yourself Production authority.

## Non-negotiable safeguards
- Work only with documents the operator is authorized to download or reproduce. Do not bypass login, DRM, entitlement checks, rate limits, or access-control denials.
- Keep the publisher-host allowlist, IP/SSRF protection, verified TLS, bounded requests, atomic PDF publication, cancellation, and per-job isolation.
- Production requires verified @rcat.ac.th Google Workspace identities and server-side RBAC. Never authorize by email suffix alone; enforce verified Google token claims, roles and job ownership in the backend.
- Never commit .env, OAuth secrets, cookies, tunnel tokens, identity references, private role stores or user PDF data. Redact diagnostics.
- Production host settings are operator-owned: preserve compose.quota.yaml, anyflip-quota.service, Docker volumes, quota mounts, Cloudflare Tunnel, DNS and 10.10.1.100:8094 bind. Never run docker compose down -v, force-reset host-local edits, or silently deploy/restart production.

## Delivery contract
1. Inspect the current main branch, PRs, CI and relevant tracker/runbooks; confirm scope before editing.
2. Make focused changes in a feature branch and PR. Do not mark an item complete merely because a PR exists.
3. Run gofmt, go build ./..., go vet ./..., go test -count=1 ./..., and go test -race ./... for Go changes. Run affected Node/Playwright, Docker/Compose and security checks for their surfaces.
4. Require green CI on the exact PR head and review behavior at authorization, persistence, and error boundaries before merging.
5. Report exact commit, PR, tests and remaining host/physical acceptance. CI and sandbox checks are not production-server proof.

## Repository skills
Codex discovers scoped skills in .agents/skills/ automatically. Select only the relevant skill for a task:
- anyflip-go-pdf — Go download, page validation, PDF fidelity and quality.
- anyflip-google-rbac — Google Workspace SSO, privileged roles, audit and ownership.
- anyflip-web-ui — RCAT branding, Thai UX, accessibility and browser E2E.
- anyflip-docker-ops — Docker, quota storage, tunnel and non-destructive operations.
- anyflip-security-review — threat-driven review and regression tests.
- anyflip-ci-release — CI, vulnerabilities, tracker, controlled release candidate.
- anyflip-incident-triage — reproducible incident diagnosis from Web job to container.

## Primary runbooks
docs/DEVELOPMENT_TRACKER.md; docs/P1_VERIFICATION.md; docs/GOOGLE_CUTOVER.md; docs/RBAC_OPERATIONS.md; docs/HOST_ACCEPTANCE.md; docs/RELEASE_CHECKLIST.md; docs/WEB_DEPLOYMENT.md.
