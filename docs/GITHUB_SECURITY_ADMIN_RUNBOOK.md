# Security Administration Closeout — anyflip-downloader

This covers **GitHub repository settings only**. It neither deploys the project nor
changes the real Ubuntu server, quota store, Google OAuth, Cloudflare Tunnel or DNS.

## Access boundary

The connected GitHub app can read and write repository code, issues, and PRs, but
cannot administer several GitHub Settings endpoints. The operator's GitHub CLI
session, authenticated with repository Admin permissions, must apply those
settings. A successful Pull Request does **not** activate the GitHub settings.

This repository is intentionally hard-coded as `yiwssx/anyflip-downloader` in
`scripts/gh-security-hardening.sh`. The script fails closed if the repository,
branch, visibility or Admin rights are not what is expected.

## Operator-only commands

From a trusted local clone on the `main` branch, after merging the PR:

```bash
gh auth status
git pull --ff-only origin main
bash scripts/gh-security-hardening.sh audit
bash scripts/gh-security-hardening.sh apply
bash scripts/gh-security-hardening.sh audit
```

The `audit` mode (default) is read-only. `apply` requests only protective
repository-side settings:

- Default GitHub Actions workflow token `read` and disallow workflow bot PR approvals.
- Dependabot alerts and automated security-fix PRs.
- Private vulnerability reporting.
- Secret scanning and push protection, only when the current settings can be
  read. It does not turn off an existing security feature.
- CodeQL default setup, **only if** the current state is `not-configured`;
  it never overwrites an existing configured CodeQL setup. Initial scans can
  take time or report findings that need triage.

Security API failures are reported; they must **not** be counted as success.
The script never removes collaborators, invitations, webhooks, Apps or Environments.

## Human administrative review

`audit` includes collaborator permissions, invitations, webhooks (without
URLs/secrets), environment names and feature statuses when the account can
read them. Inspect the full Settings pages for:

1. Unexpected human users, inherited/team privileges, invitations, installed
   GitHub Apps and webhooks; revoke stale access only after confirming use.
2. Administrator MFA/passkeys and recovery readiness.
3. Whether `CodeQL default setup` completed a successful scan; assess and
   remediate relevant findings. Keep the existing Go `govulncheck` CI.
4. Whether Secret Scanning and Push Protection are enabled and alerts reviewed.
5. Old commits for historical exposure: invalidate any actually leaked tokens
   at their providers. Never post secret values in public issues.
6. Any GitHub Environments with deployment credentials or unreviewed jobs.
7. Dependency alerts/security fixes; test each PR. Do not enable unattended
   production deployments or blind auto-merges.

The repository's `main-branch-policy` ruleset is already active, with PRs,
strict `build` and `Docker Compose smoke test`, no force pushes/deletion,
linear history, and zero bypass actors. Do not weaken these rules to complete
the administrative closeout.

## Evidence and closure

Keep sensitive audit output on the operator's machine. Report only nonsecret
settings states (for example enabled/disabled, checks' result, whether extra
access was found) to [Issue #91](https://github.com/yiwssx/anyflip-downloader/issues/91).
Only close it after GitHub-side controls have been **independently verified**;
a script or PR existing on main is not sufficient evidence.

As a minimum, verify:
- Actions token read, PR approval false.
- Dependabot alerts/security updates enabled.
- Secret scanning/push protection enabled (or an explicit unsupported-status
  justification), private vulnerability reporting enabled.
- CodeQL default setup configured and its initial run passed, or a documented
  incompatibility.
- Human / App access, invitations, webhooks and environments reviewed.
