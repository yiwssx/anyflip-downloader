# UIH-07 — Host Acceptance & Controlled Rollback

**Status: PREPARATION ONLY. No host verification or deployment has been performed by this change.**  
Tracking: [UIH Epic #93](https://github.com/yiwssx/anyflip-downloader/issues/93), [UIH-07 #101](https://github.com/yiwssx/anyflip-downloader/issues/101).

## Release boundary
Code merge and synthetic CI are not production acceptance. Only a designated operator may approve a release after reviewing immutable source/image versions, backups, restoration procedures, change window and rollback plan. Do not alter DNS, Cloudflare Tunnel, Apache, Google OAuth, secrets or Docker quota/mounts from this checklist. Never run `docker compose down -v` on the operator host. Preserve `compose.quota.yaml`, `anyflip-quota.service` and existing job/RBAC/history data.

## Acceptance record (fill from direct evidence)
| Gate | Evidence, timestamp and operator |
| --- | --- |
| Approved change window, operator and source commit | Pending |
| Pinned image digest, previous known-good image and rollback plan | Pending |
| Off-volume backup and actual restoration rehearsal | Pending |
| Go build/vet/tests/race/vulnerability, Docker and browser CI | Pending |
| Real LAN `10.10.1.100:8094` and HTTPS `anyflip.rcat.ac.th` health | Pending |
| HTTPS/CSP/no-store/X-Content-Type-Options and Secure/HttpOnly/SameSite cookies | Pending |
| Two authorized @rcat.ac.th users, negative personal Gmail login | Pending |
| User/Admin/Super Admin separation, live role revoke and redacted audit | Pending |
| Cross-user job/history/PDF download denied at API | Pending |
| Authorized single/batch, progress, cancel, retry, history and image-PDF | Pending |
| Durable volume, hard quota, restart and controlled ENOSPC recovery | Pending |
| Physical Android/iOS, 320px–1440px, keyboard and screen reader | Pending |
| Post-release monitoring and verified rollback | Pending |

## Before rollout
1. Record immutable commit SHA, PRs and successful GitHub CI run URLs. Review synthetic Chromium/Firefox screenshots for layout differences; WebKit functional/axe CI remains mandatory, while WebKit screenshot capture is deferred due to Playwright CSP-injected styles. Screenshot artifacts are **not** golden-image comparisons, actual phones or WCAG certification.
2. Verify backups, persistent mount permissions, `0600` RBAC store, quota service, rollback image and clean deployment configuration without copying private tokens into logs/Issues.
3. Verify Google Workspace OAuth allowed domains and canonical callback with the operator. Do not manipulate production credentials or DNS to satisfy tests.

## During authorized acceptance
- Verify real HTTPS `/`, `/tokens.css`, `/style.css`, `/workspace.js` and `/admin-api.js` (Google mode only), strict CSP without unsafe-inline/unsafe-eval, no-store and cookie attributes. Capture redacted response-header evidence, never cookie values.
- With separate institutional accounts test user/admin/super_admin roles, permission changes, last-super-admin protections, audit, negative Gmail login and object-level denial to another user's PDF.
- Use **authorized** AnyFlip books only for single/batch creation, progress, cancel, retry, history, conversion and downloaded PDF. Verify quotas, durable state and restart recovery.
- Test keyboard focus, live status, dialogs, horizontal overflow and real device/browser combinations (Android/iOS/Safari/Firefox/Chromium) and at least one screen reader. Capture version, result and evidence.
- Monitor resource pressure, API retries, CSP console failures, authorization errors and tunnel routing. Do not impact other RCAT services.

## Abort and rollback
Abort on a verified security/RBAC/IDOR/cookie/CSP failure, data loss, persistent auth errors, inaccessible core workflows, unverified backups or quota/mount regression. Roll back with the **previous operator-approved image and unchanged persistent volume/overlays** through the established host procedure; never delete volumes. Reconfirm auth, role separation, job history, PDF, quota and service health, and log the incident.

## Required operator decision
- [ ] Every UIH source/CI dependency accepted with evidence
- [ ] Real host, Google Workspace, RBAC, device and recovery checks passed
- [ ] Rollback exercised successfully; backups verified
- [ ] Designated operator authorized release and signed off

**Issue #101 must stay OPEN until these gates are actually met.**
