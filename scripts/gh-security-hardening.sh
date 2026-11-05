#!/usr/bin/env bash
# GitHub administrative security follow-up for the public AnyFlip fork.
# Read-only by default. Never runs against Ubuntu, Cloudflare or production.
set -euo pipefail

REPO="yiwssx/anyflip-downloader"
MODE="audit"
if [[ "$#" -gt 1 ]]; then
  echo "Usage: bash scripts/gh-security-hardening.sh [audit|apply]" >&2
  exit 2
fi
if [[ "$#" -eq 1 ]]; then MODE="$1"; fi
if [[ "$MODE" != "audit" && "$MODE" != "apply" ]]; then
  echo "Usage: bash scripts/gh-security-hardening.sh [audit|apply]" >&2
  exit 2
fi
command -v gh >/dev/null || {
  echo "GitHub CLI is required. Sign in with gh auth login." >&2
  exit 2
}
gh auth status >/dev/null 2>&1 || {
  echo "Not authenticated to GitHub CLI." >&2
  exit 2
}
identity="$(gh api "repos/$REPO" --jq '[.full_name, .default_branch, .visibility, (.permissions.admin|tostring)] | join(" ")')"
read -r actual default_branch visibility is_admin <<<"$identity"
if [[ "$actual" != "$REPO" || "$default_branch" != "main" ]]; then
  echo "Wrong repository/default branch; refusing to continue." >&2
  exit 1
fi
if [[ "$is_admin" != "true" ]]; then
  echo "The current GitHub login lacks repository Admin permission." >&2
  exit 1
fi

print_optional() {
  local label="$1"
  local path="$2"
  local jq_query="$3"
  echo "== $label =="
  if ! gh api "$path" --jq "$jq_query"; then
    echo "UNVERIFIED: insufficient permission, unavailable feature or endpoint error." >&2
  fi
}

audit() {
  echo "Repository: $REPO (visibility=$visibility, default=$default_branch)"
  print_optional "Actions workflow token / PR approval" \
    "repos/$REPO/actions/permissions/workflow" \
    '{default_workflow_permissions,can_approve_pull_request_reviews}'
  print_optional "Secret scanning / push protection (feature status only)" \
    "repos/$REPO" \
    '{security_and_analysis}'
  echo "== Dependabot security alerts =="
  if gh api "repos/$REPO/vulnerability-alerts" >/dev/null 2>&1; then
    echo "Enabled"
  else
    echo "UNVERIFIED (may be disabled or unreadable)"
  fi
  print_optional "Dependabot security update PRs" \
    "repos/$REPO/automated-security-fixes" \
    '{enabled,paused}'
  print_optional "Private vulnerability reporting" \
    "repos/$REPO/private-vulnerability-reporting" \
    '{enabled}'
  print_optional "CodeQL default setup" \
    "repos/$REPO/code-scanning/default-setup" \
    '{state,languages,query_suite,schedule}'
  print_optional "Collaborators and effective permissions (read only)" \
    "repos/$REPO/collaborators?per_page=100" \
    '[.[] | {login,permission: .permissions}]'
  print_optional "Pending collaborator invitations (read only)" \
    "repos/$REPO/invitations?per_page=100" \
    '[.[] | {id,invitee: .invitee.login,permissions}]'
  print_optional "Webhooks (no payload URLs or secrets)" \
    "repos/$REPO/hooks?per_page=100" \
    '[.[] | {id,active,events}]'
  print_optional "GitHub environments (names only)" \
    "repos/$REPO/environments" \
    '[.environments[]? | .name]'
  echo "Account MFA/passkeys, all GitHub App access, and historical secret validity require separate owner review."
}

if [[ "$MODE" == "audit" ]]; then
  audit
  exit 0
fi

if [[ "$visibility" != "public" ]]; then
  echo "This security baseline only applies to the expected public fork." >&2
  exit 1
fi

# Required access preflight; no changes if administration is unreadable.
gh api "repos/$REPO/actions/permissions/workflow" >/dev/null
secret_scanning="$(gh api "repos/$REPO" --jq '.security_and_analysis.secret_scanning.status // "unknown"')"
push_protection="$(gh api "repos/$REPO" --jq '.security_and_analysis.secret_scanning_push_protection.status // "unknown"')"

echo "Enforcing read-only Actions token and no workflow PR approvals..."
gh api --method PUT "repos/$REPO/actions/permissions/workflow" \
  -f default_workflow_permissions=read \
  -F can_approve_pull_request_reviews=false >/dev/null

echo "Enabling Dependabot vulnerability alerts and security update PRs..."
gh api --method PUT "repos/$REPO/vulnerability-alerts" >/dev/null
gh api --method PUT "repos/$REPO/automated-security-fixes" >/dev/null

echo "Enabling private vulnerability reporting..."
gh api --method PUT "repos/$REPO/private-vulnerability-reporting" >/dev/null

# Avoid blindly changing a feature when its current status cannot be read.
if [[ "$secret_scanning" == "disabled" || "$push_protection" == "disabled" ]]; then
  echo "Enabling Secret Scanning and Push Protection..."
  gh api --method PATCH "repos/$REPO" \
    -f "security_and_analysis[secret_scanning][status]=enabled" \
    -f "security_and_analysis[secret_scanning_push_protection][status]=enabled" \
    --jq '{security_and_analysis}' >/dev/null
elif [[ "$secret_scanning" == "enabled" && "$push_protection" == "enabled" ]]; then
  echo "Secret Scanning and Push Protection are already enabled."
else
  echo "WARNING: Secret Scanning status unreadable. No blind change; inspect Security settings." >&2
fi

# Do not overwrite an existing CodeQL default configuration. If the endpoint
# is unavailable, retain the previous setup and report an explicit warning.
if codeql_state="$(gh api "repos/$REPO/code-scanning/default-setup" --jq '.state // "unknown"' 2>/dev/null)"; then
  if [[ "$codeql_state" == "configured" ]]; then
    echo "CodeQL default setup is already configured."
  elif [[ "$codeql_state" == "not-configured" ]]; then
    echo "Enabling CodeQL default setup (GitHub chooses supported languages)..."
    if ! gh api --method PATCH "repos/$REPO/code-scanning/default-setup" \
      -f state=configured --jq '{state,run_id,run_url}'; then
      echo "WARNING: CodeQL activation could not be confirmed; inspect the Security UI." >&2
    fi
  else
    echo "WARNING: unexpected CodeQL state '$codeql_state'; not changing it." >&2
  fi
else
  echo "WARNING: CodeQL configuration could not be read; no blind update." >&2
fi

echo "== Post-change audit =="
audit
echo "Changes were limited to the named GitHub repository; review warnings and confirm scans succeed."
