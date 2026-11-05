---
name: anyflip-docker-ops
description: Use for Docker Compose, non-root containers, pdfcpu HOME failures, Ubuntu filesystem quota, volumes, systemd service, Apache and Cloudflare Tunnel deployment or rollback plans.
---

# anyflip-docker-ops

## Protect the production data plane
- Inspect docs/HOST_ACCEPTANCE.md, docs/WEB_DEPLOYMENT.md, docs/GOOGLE_CUTOVER.md and docs/RBAC_OPERATIONS.md first.
- The stock compose.web.yaml is a template, NOT the complete live configuration. The operator has a quota-aware compose.quota.yaml, anyflip-quota.service, private .env, persistent named volumes and a customized LAN bind at 10.10.1.100:8094.
- Never overwrite those host files, run docker compose down -v, unmount the quota, delete job volumes, alter DNS/Tunnel or force Git reset to resolve a merge problem. Deploy only with an approved change window and backup/rollback path.
- Preserve non-root UID, read_only root, drop capabilities, no-new-privileges and bounded resources. pdfcpu must use writable HOME/XDG_CONFIG_HOME in the existing managed volume.
- Google OAuth redirects require canonical HTTPS. Never send Bearer tokens or cookies over unsecured LAN HTTP.
- Distinguish Cloudflare edge errors, Apache/Tunnel routing, DNS and container API problems before proposing a fix. Do not infer production state from CI.

## Acceptance procedure
1. Inventory running image SHA, service/overlay files, mount, quota status, health, and private backup status using non-destructive commands.
2. Use scripts/verify-host-readiness.sh and scripts/preflight-google-cutover.sh when appropriate; document any limitations.
3. Validate Compose configurations and shell scripts, then stage a reversible change without replacing operator overrides.
4. Test health, auth denial, persistence, safe retention, disk-full/mount loss and rollback only with operator authorization and controlled fixtures.
5. Record observed evidence separately from operator attestation.
