# Contributing to RCAT AnyFlip Downloader

This is a Go CLI and self-hosted Web GUI fork. Contribution does not authorize
downloading restricted publisher content or changing the college production
service. Read [AGENTS.md](AGENTS.md) and [SECURITY.md](SECURITY.md) first.

## Workflow

1. Open an issue using an existing template or link a tracker item.
2. Create a topic branch from current main; do not push directly to main.
3. Keep changes narrowly scoped and use clear, conventional-style commit titles.
4. Open a pull request using the repository template and wait for required CI.
5. For changes affecting access control, stored data, PDFs, deployment or
   infrastructure, include rollback details and operator/physical acceptance.
6. Do not merge a PR with failing required checks or present CI as production proof.

## Local validation

Use the Go version from go.mod.

    gofmt -w ./your_modified_file.go
    go mod verify
    go build ./...
    go vet ./...
    go test -count=1 ./...
    go test -race ./...
    python3 scripts/validate-agent-skills.py
    python3 scripts/check-repository-policy.py

For Web edits also run the relevant Chromium E2E tests; for container edits
run Docker Compose validation and isolated smoke tests. Full acceptance is
described in [RELEASE_CHECKLIST](docs/RELEASE_CHECKLIST.md).

## Secrets and privacy

Never commit .env files, Google/OAuth credentials, tunnel tokens, login
cookies, downloaded PDFs, identifying user data or private audit/role stores.
Use redacted synthetic test fixtures. Follow [private vulnerability
reporting](SECURITY.md) for security findings.

## Production ownership

The operator controls the live Ubuntu host, existing quota overlay,
compose.quota.yaml, 10.10.1.100:8094 listener, Cloudflare Tunnel and DNS.
Neither merging nor running GitHub Actions grants deployment approval.
