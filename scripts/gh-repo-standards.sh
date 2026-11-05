#!/usr/bin/env bash
# Audits or applies GitHub governance using a separately authenticated owner.
# Deliberately never called by GitHub Actions or production deploy.
set -euo pipefail

REPO="yiwssx/anyflip-downloader"
if [[ "$#" -ne 1 ]]; then
  echo "Usage: bash scripts/gh-repo-standards.sh audit|apply" >&2
  exit 2
fi
MODE="$1"
if [[ "$MODE" != "audit" && "$MODE" != "apply" ]]; then
  echo "Usage: bash scripts/gh-repo-standards.sh audit|apply" >&2
  exit 2
fi
if ! command -v gh >/dev/null 2>&1; then
  echo "GitHub CLI (gh) is required; authenticate with gh auth login." >&2
  exit 2
fi
gh auth status >/dev/null 2>&1 || {
  echo "gh is not authenticated." >&2
  exit 2
}

info="$(gh api "repos/$REPO" --jq '[.full_name, .default_branch, .visibility, (.permissions.admin|tostring)] | join(" ")')"
read -r actual branch visibility admin <<<"$info"
[[ "$actual" == "$REPO" && "$branch" == "main" ]] || {
  echo "Unexpected repository or default branch. Aborting." >&2
  exit 1
}
echo "Repository: $actual | branch: $branch | visibility: $visibility | admin: $admin"

audit() {
  gh api "repos/$REPO" --jq '{visibility,default_branch,delete_branch_on_merge,allow_auto_merge,allow_squash_merge,allow_merge_commit,allow_rebase_merge,allow_update_branch,has_wiki}'
  echo "Repository rulesets:"
  gh api "repos/$REPO/rulesets" --jq '[.[] | {id,name,enforcement,target}]'
  echo "Actions token policy:"
  gh api "repos/$REPO/actions/permissions/workflow" --jq '{default_workflow_permissions,can_approve_pull_request_reviews}' || echo "Unable to read Actions permissions (check token scopes)." >&2
  echo "Security scanning, reporting and collaborators require additional inspection in GitHub Settings."
}

if [[ "$MODE" == "audit" ]]; then
  audit
  exit 0
fi

[[ "$admin" == "true" && "$visibility" == "public" ]] || {
  echo "Apply requires admin permission on the expected public repository." >&2
  exit 1
}

# Avoid a partial settings rollout if required read endpoints are inaccessible.
gh api "repos/$REPO/rulesets" >/dev/null
gh api "repos/$REPO/actions/permissions/workflow" >/dev/null

echo "Applying repository merge hygiene, Actions policy and main ruleset ..."
gh api --method PATCH "repos/$REPO" \
  -F delete_branch_on_merge=true \
  -F allow_auto_merge=true \
  -F allow_update_branch=true \
  -F allow_squash_merge=true \
  -F allow_merge_commit=false \
  -F allow_rebase_merge=false \
  -F has_wiki=false >/dev/null

gh api --method PUT "repos/$REPO/actions/permissions/workflow" \
  -f default_workflow_permissions=read \
  -F can_approve_pull_request_reviews=false >/dev/null

# PRs are mandatory, but approval count stays zero while the sole maintainer
# is also the author; self-approval is not possible on GitHub.
policy="$(cat <<'JSON'
{
  "name": "main-branch-policy",
  "target": "branch",
  "enforcement": "active",
  "bypass_actors": [],
  "conditions": {
    "ref_name": {"include": ["refs/heads/main"], "exclude": []}
  },
  "rules": [
    {"type": "deletion"},
    {"type": "non_fast_forward"},
    {"type": "required_linear_history"},
    {"type": "pull_request", "parameters": {
      "dismiss_stale_reviews_on_push": true,
      "require_code_owner_review": false,
      "required_approving_review_count": 0,
      "required_review_thread_resolution": true,
      "require_last_push_approval": false
    }},
    {"type": "required_status_checks", "parameters": {
      "strict_required_status_checks_policy": true,
      "required_status_checks": [
        {"context": "build"},
        {"context": "Docker Compose smoke test"}
      ]
    }}
  ]
}
JSON
)"
rule_id="$(gh api "repos/$REPO/rulesets" --jq '[.[] | select(.name=="main-branch-policy") | .id][0] // empty')"
if [[ -n "$rule_id" ]]; then
  printf '%s\n' "$policy" | gh api --method PUT "repos/$REPO/rulesets/$rule_id" --input - --jq '{id,name,enforcement}'
else
  printf '%s\n' "$policy" | gh api --method POST "repos/$REPO/rulesets" --input - --jq '{id,name,enforcement}'
fi

if gh api --method PUT "repos/$REPO/vulnerability-alerts" >/dev/null; then
  echo "Dependabot vulnerability alerts: enabled"
else
  echo "WARNING: Could not enable vulnerability alerts; inspect Security settings." >&2
fi

echo "GitHub-side baseline applied. Verify security features, collaborators and required checks."
audit
