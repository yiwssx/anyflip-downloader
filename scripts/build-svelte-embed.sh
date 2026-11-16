#!/usr/bin/env bash
# Local/CI-only candidate bundle staging. Never run this script on a live RCAT
# deployment host as a side effect of checkout or unrelated production upgrades.
set -euo pipefail
repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo"
test -f frontend/package-lock.json
test -f web/svelte/README.md
npm ci --no-audit --no-fund --prefix frontend
(cd frontend && SUI_EMBED_BUILD=1 npm run check && SUI_EMBED_BUILD=1 npm run build)
mkdir -p web/svelte
cp -R frontend/dist/. web/svelte/
test -s web/svelte/token.html
test -s web/svelte/google.html
test -d web/svelte/assets
echo "SUI04_EMBED_ASSET_PREPARED=true (CI/dev checkout only)"
