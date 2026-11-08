# UIH-07 — Host Acceptance & Controlled Rollback

**Status: OPERATOR-ACCEPTED on 2026-11-08 (reported by the designated RCAT operator).** Repository/CI results are independently available; live-host/device/rollback results are **operator attestations**, not checks executed or observed by the assistant. This documentation change does not deploy, alter, or independently inspect production.  
Tracking: [UIH Epic #93](https://github.com/yiwssx/anyflip-downloader/issues/93), [UIH-07 #101](https://github.com/yiwssx/anyflip-downloader/issues/101).

## Release boundary
Code merge and synthetic CI are not production acceptance. Only a designated operator may approve a release after reviewing immutable source/image versions, backups, restoration procedures, change window and rollback plan. Do not alter DNS, Cloudflare Tunnel, Apache, Google OAuth, secrets or Docker quota/mounts from this checklist. Never run `docker compose down -v` on the operator host. Preserve `compose.quota.yaml`, `anyflip-quota.service` and existing job/RBAC/history data.

## Operator acceptance record (attested by operator; raw evidence not attached)
| Gate | Evidence, timestamp and operator |
| --- | --- |
| Approved change window, operator and source commit | Operator reported PASS on 2026-11-08; underlying live-host evidence retained by operator, not independently collected here |
| Pinned image digest, previous known-good image and rollback plan | Operator reported PASS on 2026-11-08; underlying live-host evidence retained by operator, not independently collected here |
| Off-volume backup and actual restoration rehearsal | Operator reported PASS on 2026-11-08; underlying live-host evidence retained by operator, not independently collected here |
| Go build/vet/tests/race/vulnerability, Docker and browser CI | Operator reported PASS on 2026-11-08; underlying live-host evidence retained by operator, not independently collected here |
| Real LAN `10.10.1.100:8094` and HTTPS `anyflip.rcat.ac.th` health | Operator reported PASS on 2026-11-08; underlying live-host evidence retained by operator, not independently collected here |
| HTTPS/CSP/no-store/X-Content-Type-Options and Secure/HttpOnly/SameSite cookies | Operator reported PASS on 2026-11-08; underlying live-host evidence retained by operator, not independently collected here |
| Two authorized @rcat.ac.th users, negative personal Gmail login | Operator reported PASS on 2026-11-08; underlying live-host evidence retained by operator, not independently collected here |
| User/Admin/Super Admin separation, live role revoke and redacted audit | Operator reported PASS on 2026-11-08; underlying live-host evidence retained by operator, not independently collected here |
| Cross-user job/history/PDF download denied at API | Operator reported PASS on 2026-11-08; underlying live-host evidence retained by operator, not independently collected here |
| Authorized single/batch, progress, cancel, retry, history and image-PDF | Operator reported PASS on 2026-11-08; underlying live-host evidence retained by operator, not independently collected here |
| Durable volume, hard quota, restart and controlled ENOSPC recovery | Operator reported PASS on 2026-11-08; underlying live-host evidence retained by operator, not independently collected here |
| Physical Android/iOS, 320px–1440px, keyboard and screen reader | Operator reported PASS on 2026-11-08; underlying live-host evidence retained by operator, not independently collected here |
| Post-release monitoring and verified rollback | Operator reported PASS on 2026-11-08; underlying live-host evidence retained by operator, not independently collected here |

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

## Required operator decision — signed off by operator statement (2026-11-08)
- [x] Every UIH source/CI dependency accepted, per operator confirmation
- [x] Real host, Google Workspace, RBAC, device and recovery checks reported passed by the operator
- [x] Rollback and backups reported verified by the operator
- [x] Designated operator authorized closure of all UIH tasks and Epic

**Recorded basis:** In this conversation on 2026-11-08 the RCAT host operator stated “ยืนยันว่าผ่านทั้งหมด ปิดงานได้” (confirmed all passed; may close work). Source baseline at that point: `d2a6b04afb34e0b9e8535cbd4e849f058c54f659` on `main`. Related merged work: PRs #102–#111. Synthetic quality gates: [PR #110 Go/Docker](https://github.com/yiwssx/anyflip-downloader/actions/runs/42480451316), [PR #110 Browser](https://github.com/yiwssx/anyflip-downloader/actions/runs/42480451400), [PR #111 Go/Docker](https://github.com/yiwssx/anyflip-downloader/actions/runs/42480729037), [PR #111 Browser](https://github.com/yiwssx/anyflip-downloader/actions/runs/42480728978); all reported green in GitHub. No private OAuth secrets, user records, operational logs, deployment image digest, or rollback artifacts were supplied to the repository. Do not infer that this document establishes the **exact deployed production digest** or independent full WCAG 2.2 AA certification. WebKit browser behavior is tested but WebKit CI screenshots remain an explicitly documented tooling exception. The operator owns retention of sensitive release/rollback records.

**Closure decision:** UIH-07 may be closed based on the operator's explicit sign-off; any future incident or regression is a new operational/security issue, not proof of a passed gate that has not been independently observed.
