# GitHub Repository Governance — anyflip-downloader

## Scope

This policy governs only the public fork yiwssx/anyflip-downloader.
The actual RCAT service, Google OAuth client, Docker volumes, quota overlay,
Cloudflare Tunnel and DNS are controlled by a separate Ubuntu host operator.
Neither a pull request nor a CI run deploys or publishes anything automatically.

## Baseline observed on 2026-11-05

- Account yiwssx has repository Admin permission, main is default, fork is public.
- Before this work, main had no protection and no repository rulesets existed.
- GitHub checks on main at commit 1fd04dd99f0a0221170b17b2b20b24b407b23ccb:
  build, Docker Compose smoke test and Chromium desktop/mobile smoke: success.
- Existing Actions had contents:read tokens. Release Candidate is manually
  dispatched and uploads artifacts only, without tags, releases or deployment.
- Existing Dependabot covered Go Modules only.

These are observed repository values, not an assertion that the GitHub admin
settings were changed by the connected GitHub integration, and not evidence
of independent access to the running Ubuntu host.

## Versioned repository baseline

- Work in topic branches and PRs, CODEOWNERS, PR/Issue templates.
- Require successful build and Docker Compose smoke on main.
  Browser E2E is path-conditional: do not make it universally required until
  it is scheduled on every PR (otherwise absent checks can block a merge).
- GitHub Actions minimum contents:read, checkout without persistent token,
  full verified action commit SHA pins and obsolete-run cancellation.
- Dependabot: Go Modules, GitHub Actions and Docker images, bounded grouped
  updates, review before merging, no auto-deployment.
- No secrets or private documents in Git; line endings and local artifacts
  normalized via EditorConfig, Git attributes and gitignore.
- Disclosure via SECURITY.md; physical/operator release acceptance via
  docs/RELEASE_CHECKLIST.md.

## Apply GitHub-admin configuration

This repository intentionally has no admin PAT, OAuth client secret or
production credential in GitHub Actions. The connected GitHub integration has
file-write access, but does not expose supported admin mutation endpoints.

From an admin-owned trusted workstation with GitHub CLI signed into the
authorized repository administrator, review and run:

    gh auth status
    bash scripts/gh-repo-standards.sh audit
    bash scripts/gh-repo-standards.sh apply
    bash scripts/gh-repo-standards.sh audit

The script scopes itself to this repository and requires the explicit apply
argument. It creates/updates the main-branch-policy ruleset without bypass
actors, requires PR before merge, disables force-push/deletion, requires
linear history, resolved review conversations and strict green build +
Docker Compose smoke checks. Approval count is zero for a single maintainer;
a PR author cannot approve their own work.

It also enables squash merge, auto-delete merged branches, auto-merge option,
update-branch option, disables merge commits/rebase merge and the unused wiki,
enforces default read-only Actions workflow permissions and disallows Actions
bot approvals. It attempts to enable vulnerability alerts.

The ruleset can prevent merging until required checks pass. Change the policy
only after verifying the specific check names and reviewer capacity. Re-run
audit to verify actual GitHub-side state. A file-based policy is not a working
branch protection rule until it has been applied and observed.

## Manual security/access review in GitHub UI

- **Settings → Collaborators / installed apps:** inspect effective human,
  team and GitHub App permissions. Remove stale access, invitations and unused
  webhooks, ensure administrators have MFA/passkeys. Admin status for yiwssx
  does not imply no other collaborators exist.
- **Settings → Security / Code security:** verify private vulnerability
  reporting, dependency graph, Dependabot alerts and security updates,
  secret scanning with push protection (where supported), and CodeQL default
  setup for Go. Triage real findings; do not silently suppress them.
- **Settings → Actions:** restrict allowed actions to approved sources where
  supported. Keep default workflow token read-only and untrusted PRs away from
  real host/OAuth/Tunnel credentials. Review Actions artifact retention.
- **Settings → Environments:** if deployment automation is introduced later,
  protect production with explicit human approval and scoped secrets.
  This repository has no production deployment workflow as of this baseline.
- **Settings → General:** keep Issues/Projects enabled as currently configured.
  Public visibility is a conscious source-code disclosure decision: separately
  review Git history for sensitive content before any visibility changes.
- **Operator secrets:** rotate any exposed credential at its issuing provider.
  Deleting a leaked secret from the latest Git revision does not revoke it.

## Ongoing maintenance

Monthly: inspect dependency alerts/PRs, CI failures, inactive branches,
privileged users and installed apps. Require passing checks on each merged head.

Before a release: record commit, exact CI run IDs and operator approval,
verify manual physical tests and rollback. Candidate artifacts do not provide
publication or deployment authorization.

## Validation

    python3 scripts/check-repository-policy.py
    bash -n scripts/gh-repo-standards.sh

Never store personal access tokens, OAuth credentials or live service
configuration in this repository.
