# AnyFlip Downloader — Online Web GUI Development Tracker

**สถานะ:** 🟡 Download Core + Web GUI + Docker CI verified; **LAN-only storage/quota cutover CLOSED by operator on 2026-11-01; public HTTPS/production not approved**  
**อัปเดตครั้งแรก:** 2026-10-22  
**เป้าหมาย:** Web GUI ผ่านเบราว์เซอร์ บน Ubuntu Server + Docker/Compose + Apache Reverse Proxy + HTTPS โดยคง CLI เดิมไว้

## ข้อเท็จจริงและขอบเขต

- โปรแกรมเดิมดาวน์โหลดภาพหน้าเอกสาร แล้วใช้ `pdfcpu` รวมเป็น **Image-based PDF**; **ไม่ใช่ PDF ต้นฉบับ** ที่เก็บข้อความ/Vector ไว้ครบ
- ใช้สำหรับเอกสารที่ผู้ใช้มีสิทธิ์ดาวน์โหลด/ทำสำเนาเท่านั้น ไม่เพิ่มฟีเจอร์ข้ามการจำกัดสิทธิ์, DRM หรือระบบยืนยันตัวตน
- เวอร์ชันแรก: **Go Backend + Responsive Web UI** เสิร์ฟจากแอปเดียวใน Docker; Apache เป็น HTTPS Reverse Proxy
- ไม่ติดตั้งฐานข้อมูล/OCR ตั้งแต่วันแรกโดยไม่จำเป็น; เน้นความเสถียร ความปลอดภัย และไฟล์ผลลัพธ์ถูกต้องก่อน
- **หนึ่ง PR ต่อหนึ่งงาน** พร้อมทดสอบและตรวจรับก่อน Merge; Tracker นี้เป็นศูนย์กลางติดตาม

## P5-ADMIN — Admin Console + Google Workspace RBAC (NEW, 2026-11-03)

**Epic:** [#69 — Admin Console + RBAC](https://github.com/yiwssx/anyflip-downloader/issues/69) · **Status:** PLANNED, 0/8 child tasks complete; **no Admin/RBAC code deployed**. This is a NEW Google Workspace-backed RBAC design, **not** the abandoned local account/password/Admin modules of PRs #57–#59.

**Roles:** `user` (own jobs), `admin` (institution-wide metrics/job metadata and audited cancellation), `super_admin` (app-role management + admin oversight). Admin and Super Admin **do not** receive cross-user PDF download access by default. The app-level `super_admin` is NOT a Google Workspace domain Super Admin. A verified `@rcat.ac.th` Google OIDC identity is required for every role; bind permissions to Google `iss+sub`, not to email text or browser-only visibility.

### Subtasks and sequence
- [ ] [RBAC-0 #70 — Permission matrix, threat model and API contracts](https://github.com/yiwssx/anyflip-downloader/issues/70)
- [ ] [RBAC-1 #71 — Persistent role assignments + safe Super Admin bootstrap](https://github.com/yiwssx/anyflip-downloader/issues/71)
- [ ] [RBAC-2 #72 — Server-side permission enforcement and object-level isolation](https://github.com/yiwssx/anyflip-downloader/issues/72)
- [ ] [RBAC-3 #73 — User directory and Super Admin role management API](https://github.com/yiwssx/anyflip-downloader/issues/73)
- [ ] [RBAC-4 #74 — Admin job oversight, metrics and safe cancellation](https://github.com/yiwssx/anyflip-downloader/issues/74)
- [ ] [RBAC-5 #75 — Responsive Admin Dashboard, role-based navigation](https://github.com/yiwssx/anyflip-downloader/issues/75)
- [ ] [RBAC-6 #76 — Security audit, redaction, privileged-action safeguards](https://github.com/yiwssx/anyflip-downloader/issues/76)
- [ ] [RBAC-7 #77 — Unit/race/browser/security CI, production gate, backup and rollback](https://github.com/yiwssx/anyflip-downloader/issues/77)

**Development order:** #70 → #71 → #72 → #73/#74/#76 → #75 → #77. One reviewed PR per bounded unit, update issue acceptance evidence per milestone. **Production dependency:** [#68 — Google Workspace-only login cutover](https://github.com/yiwssx/anyflip-downloader/issues/68); do not make Admin available publicly until the real `@rcat.ac.th` SSO/host acceptance is complete.

**Non-negotiable release criteria:** no arbitrary first-user admin, no self-promotion, no loss of last Super Admin, no cross-user PDFs or insecure API privileges, persistent role backups/restore, auditable privilege changes without secrets in logs. Preserve `10.10.1.100:8094`, `compose.quota.yaml`, `anyflip-quota.service`, existing PDF volume, Tunnel and `www.rcat.ac.th`; no Docker volume deletion or silent production deployment.

---

## P2-UX2 — Unified RCAT Web UI/UX (2026-11-02)

**Tracking:** [PR #62](https://github.com/yiwssx/anyflip-downloader/pull/62), branch `feature/unified-responsive-ui-ux`; code-only UI redesign, no host deployment.

- [x] Responsive RCAT-branded application shell, navigation, clearer content hierarchy and Google Workspace sign-in page
- [x] Unified token and Google modes without changing authentication logic, downloads, job APIs or filesystem behavior
- [x] Dashboard totals/active/completed, searchable/filterable job history, stage timeline, batch URL counter and actionable empty/error states
- [x] Mobile/tablet layout, keyboard focus/skip-link, semantic form labels, progressbar ARIA and reduced-motion support
- [x] Browser regression coverage for both authentication modes, desktop/mobile layouts and critical downloads
- [x] PR #62 Go Build/Vet/Unit/Race/Govulncheck, Docker smoke and Chromium Desktop/Mobile Browser E2E passed at head `aec48e94` — [Go/Docker CI](https://github.com/yiwssx/anyflip-downloader/actions/runs/41408168134) · [Browser CI](https://github.com/yiwssx/anyflip-downloader/actions/runs/41408168236). Final documentation-only commit requires a fresh check.
- [ ] Physical screen-reader/keyboard and Android/iOS browser acceptance on deployed HTTPS host
- **Release:** Preserve current quota-aware systemd/Docker/volume settings; no host upgrade or OAuth cutover performed.

## P4-SSO — Google Workspace only (2026-11-02)

**Decision update:** Supersedes local accounts, roles, password change, private account audit and offline administrator recovery features from PRs #57–#59. These features were removed in [PR #60](https://github.com/yiwssx/anyflip-downloader/pull/60) to meet the simplified requirement: **only verified Google Workspace members of `@rcat.ac.th` can sign in**.

- [x] Google OAuth 2.0 authorization code + PKCE + nonce/state and backend RSA ID-token signature verification
- [x] Require Google issuer/audience/expiry, `email_verified`, matching email domain and `hd=rcat.ac.th`
- [x] Remove local user registration, local passwords/Admin/recovery dashboard and related API routes
- [x] Browser Google-only login with HttpOnly session and per-Google-subject job ownership
- [x] Reject legacy API Bearer credentials when in Google SSO mode; default legacy token mode preserved for existing deployment until cutover
- [ ] PR #60 CI, Go Unit/Race, browser test and vulnerability scan
- [ ] Operator creates Google OAuth Client and sets authorized HTTPS redirect URI
- [ ] HTTPS reverse proxy/production host acceptance with real `@rcat.ac.th` accounts
- **No host deployment or quota changes.** Previous P4-AUTH entries below remain historical records, not current design requirements. See [AUTHENTICATION.md](AUTHENTICATION.md).

## P4-AUTH — Account Login / Session / RBAC (2026-11-02)

**Tracking:** [PR #57](https://github.com/yiwssx/anyflip-downloader/pull/57) merged into `main` (2026-11-02). Host deployment not attempted. GitHub Issues are disabled in this repository (HTTP 410), so this tracker and PRs are the work records.

- [x] Development branch and PR created; token-only mode retained by default; no Anonymous Session
- [x] Implemented Argon2id password hashing, first-admin bootstrap, 0600 account store
- [x] Implemented Login/Logout/Me, HttpOnly Strict session cookies, same-origin protection and Admin-only account creation
- [x] Implemented job ownership scope for status/history, batch, retry, download, cancel; Admin and service-token override
- [x] Implemented tokenless Login GUI in opt-in `accounts` mode, with separate legacy token-only GUI
- [x] Verified Go Build/Vet/Unit/Race and vulnerability scan, Docker smoke and Chromium browser E2E in both modes on PR #57 head `330c277f` — [Go/Docker run 41381964185](https://github.com/yiwssx/anyflip-downloader/actions/runs/41381964185) · [Browser run 41381964175](https://github.com/yiwssx/anyflip-downloader/actions/runs/41381964175); follow-up docs-only revision requires CI recheck
- [ ] Host HTTPS reverse-proxy configuration and private initial-admin provisioning (operator-controlled)
- [ ] Physical browser/LAN security acceptance, account backup, recovery/reset/deactivation, logging and public security gates
- **Release gate:** Account mode must not be deployed through the existing plain-HTTP LAN URL. Preserve host-local quota Compose, systemd service, mounts, data and backups. See [AUTHENTICATION.md](AUTHENTICATION.md).

### P4-AUTH-2 — Account lifecycle (2026-11-02)

**Tracking:** [PR #58](https://github.com/yiwssx/anyflip-downloader/pull/58) merged on 2026-11-02; account lifecycle code passed Go, Docker and Browser CI; **not deployed**.

- [x] Self-service password change with current-password verification and mandatory logout across sessions
- [x] Admin-only password reset of another user; browser and machine-service token cannot bypass Admin role checks
- [x] Suspend/reactivate accounts; disabled users cannot log in, use existing cookies or resurrect sessions on reactivation; running/queued jobs are cancelled and new admission is blocked
- [x] Protect last active Admin; prevent Admin from disabling own account or resetting own password without verifying current password
- [x] Keep version-1 account store compatibility, persist disabled state and credential rotation atomically, rollback on storage failure
- [x] Implement account management UI and automated Go/Browser regression tests
- [x] PR #58 Go Build, Vet, Unit/Race, vulnerability, Docker and two-mode browser CI verified on `a8e29a75` — [Go/Docker CI](https://github.com/yiwssx/anyflip-downloader/actions/runs/41383540050) · [Browser CI](https://github.com/yiwssx/anyflip-downloader/actions/runs/41383540043); documentation-only tracker update pending final CI
- [ ] Physical HTTPS proxy/browser testing, account backup/restore and credential rotation recovery procedure
- [ ] Audit log policy, administrative account recovery and operational abuse limits for public release
- **Scope:** Code only. Do not replace operator-customized quota files, mounts, services, token or LAN binding.

### P4-AUTH-3 — Private audit and offline recovery (2026-11-02)

**Tracking:** PR #59 candidate, branch `feature/p4-auth-audit-recovery`; code is not deployed.

- [x] Private 0600 JSONL event log in account mode; fixed event/result schema, sanitized account identifiers and no credentials/tokens/URLs/IPs
- [x] Log login success/failure, logout, administrator account operations, password changes and offline recovery, with audit intent before security-sensitive mutations
- [x] Bounded 512 KiB active log plus one 512 KiB rotation archive; reject unsafe file types, permissions and oversized files
- [x] Offline-only, **existing administrator** recovery command; no HTTP route or user-to-admin promotion; stdin password and exclusive 0600 pre-recovery backup
- [x] Add unit/security tests for redaction, rotation, fail-closed audit failures, unsafe storage, backup collisions and recovery
- [x] Go Build/Vet/Unit/Race, govulncheck, Docker smoke and dual-mode Browser E2E passed on PR #59 source head `e1656373` — [Go/Docker CI](https://github.com/yiwssx/anyflip-downloader/actions/runs/41384609087) · [Browser CI](https://github.com/yiwssx/anyflip-downloader/actions/runs/41384609118). Final documentation-only commit subject to recheck.
- [ ] Host acceptance: stop service, verify backup/restore, HTTPS access, operator-only account/audit file permissions
- [ ] External tamper-evident audit retention and production security review (the local rolling JSONL is **not** tamper-proof)
- **Scope:** No server changes. Preserve quota-aware Docker volumes and systemd; no public deployment.

## Phase / Checklist

### P0 — ปรับปรุง Download Core และความปลอดภัย (Blocker)
- [x] แยกการดาวน์โหลด/แปลง PDF ออกจาก `main.go` และ global flags ให้เรียกใช้ได้ทั้ง CLI และ Web
- [x] ตรวจสอบ AnyFlip URL, host allowlist, HTTPS redirect และ DNS/IP egress เพื่อป้องกัน private-network SSRF
- [x] ตรวจสอบข้อมูลจาก config.js: จำนวนหน้า ชื่อไฟล์หน้า missing arrays / malformed inputs
- [x] ป้องกันค่า Threads/Chunk size ที่ไม่สมเหตุผล เช่น 0, ติดลบ และมากเกินขีดจำกัด
- [x] ใช้ temporary directory แยกตาม job พร้อม cleanup ปลอดภัย ห้ามลบโฟลเดอร์ที่ผู้ใช้มีอยู่ก่อน
- [x] เพิ่ม HTTP timeouts, bounded retries, exponential backoff, error handling และ TLS certificate verification
- [x] เพิ่ม cancellation ผ่าน `context.Context` และ progress event ที่ไม่ผูกกับ terminal
- [x] ตรวจความครบหน้า ลำดับหน้า คุณภาพข้อมูล และไฟล์ PDF ไม่เสีย/ไม่เขียนทับโดยไม่แจ้ง
- [x] จัด Go version ให้ตรงกันระหว่าง `go.mod`, Docker และ CI; เพิ่ม Unit/Integration/Race tests — GitHub Actions Go + Docker smoke jobs ผ่าน
- **เกณฑ์ผ่าน:** CLI เดิมทำงาน, `go test ./...` และ build ผ่าน, Cancel ได้, output ครบและปลอดภัย

### P1 — คุณภาพภาพและการแปลง PDF
- [ ] เลือกภาพคุณภาพสูงสุดที่เข้าถึงได้ **โดยได้รับอนุญาต** พร้อม fallback ที่ชัดเจน — P1-C มี 404/410-only fallback; P1-E (#27) ผ่าน CI สำหรับโหมด opt-in เปรียบเทียบจำนวนพิกเซลจากตัวเลือกใน metadata และยังคงค่าเริ่มต้นเดิม; **รอหนังสือจริงที่ได้รับอนุญาตและการตรวจด้วยโปรแกรมอ่าน PDF**
- [ ] รองรับ JPG/PNG/WebP ตามความสามารถตัวแปลง และตรวจการจัดเรียง/หมุน/ขนาดหน้า — PR #19 ทดสอบ binary formats/corruption, PR #23 ทดสอบ MediaBox/สัดส่วน/ลำดับ และ PR #25 ตรวจ EXIF 1–8 ด้วย PDF จริงใน CI; รอ visual QA และหนังสือที่ได้รับอนุญาตจริงก่อนปิด Gate
- [x] แสดงประเภทผลลัพธ์เป็น **Image-based PDF** อย่างชัดเจน — PR #34 ผ่าน CI และ Merge แล้ว; CLI, Web GUI และ HTTP download header ระบุประเภทไฟล์ พร้อม regression tests
- [ ] เพิ่ม test fixtures สำหรับหน้าขาด ภาพเสีย ชื่อต่างรูปแบบ จำนวนหน้ามาก — P1-F (#31) ผ่าน CI ด้วย PDF 129 หน้าจากภาพทดสอบจริง (รวมหน้าขาดและเสียช่วงท้าย) แต่ยังต้องทดสอบหนังสือที่ได้รับอนุญาตและการใช้งานจริง
- **เกณฑ์ผ่าน:** เอกสารทดสอบที่ได้รับอนุญาตรวมเป็น PDF ได้ครบ เรียงหน้าและเปิดอ่านได้

### P2 — Web GUI MVP
- [x] หน้าเว็บ Responsive ภาษาไทยสำหรับ Desktop/Mobile (โค้ดพร้อม; ยังต้องทำ browser QA จริง)
- [x] ฟอร์ม URL, ตรวจ metadata ที่หาได้, ชื่อไฟล์, เริ่มงาน, แสดงสถานะ, ยกเลิก, ดาวน์โหลดไฟล์
- [x] Go HTTP API สำหรับ job lifecycle และ polling 1.1 วินาทีสำหรับ progress
- [x] ระบุสถานะ queued → preparing → downloading → converting → completed / failed / cancelled
- [x] มี Job ID, แยกไฟล์แต่ละงาน, จำกัดการเข้าถึงไฟล์ และไม่เผย filesystem path
- [x] Loading, error, retry guidance, accessibility พื้นฐาน; คง CLI ทำงานต่อได้
- **เกณฑ์ผ่าน:** ทดสอบ end-to-end บนมือถือและ Desktop ผ่านทั้งกรณีสำเร็จ ล้มเหลว และ Cancel

### P2-E2E — Automated Browser Regression (CI)

- [x] **CI gate:** ทดสอบหน้า Web GUI ด้วย Chromium Desktop (1365×900) และ Mobile (390×844) บน Go server จริง โดยจำลอง API เพื่อทดสอบฟอร์ม, progress, ดาวน์โหลด PDF, Batch, Cancel, History และ Retry โดยไม่ดึงเอกสารจริงจากผู้เผยแพร่
- [x] **CI verification:** `node --test tests/browser-e2e.mjs` ผ่านบน branch/PR นี้และหลัง Merge; Workflow `browser-e2e.yml` จำกัดการ trigger เฉพาะไฟล์ Web และชุดทดสอบที่เกี่ยวข้อง เพื่อลดการทดสอบซ้ำที่ไม่จำเป็น
- [ ] **Physical acceptance:** เปิดและทดสอบด้วย Browser Desktop/Mobile จริงที่เครื่องผู้ใช้ผ่าน LAN, ตรวจการเรนเดอร์ภาษาไทย/ไฟล์ PDF ที่ได้รับอนุญาต โดยไม่เก็บ token, URL ส่วนตัว หรือหน้าเอกสารไว้ใน public CI
- **ผลทดสอบ:** [PR #48](https://github.com/yiwssx/anyflip-downloader/pull/48) ผ่าน [Browser CI #40834513098](https://github.com/yiwssx/anyflip-downloader/actions/runs/40834513098) และ [Go/Docker CI #40834513180](https://github.com/yiwssx/anyflip-downloader/actions/runs/40834513180); ส่วน Physical acceptance ยัง OPEN
- **ข้อจำกัด:** Mocked Browser E2E ไม่ยืนยันการเข้าถึง AnyFlip จริง, ความถูกต้องของหนังสือทั้งเล่ม, ประสิทธิภาพ Ubuntu Host, Hard Volume Quota หรือความพร้อมเปิด Public HTTPS

### P3 — Server / Docker / Security (ต้องผ่านก่อนเปิดใช้งานออนไลน์)
- [x] เตรียม Multi-stage Docker image แบบ non-root, Compose + volumes ที่เหมาะสม (ยังต้อง Build/Run บน Docker host จริง)
- [ ] Apache Reverse Proxy + HTTPS + security headers + health check
- [ ] Authentication / Authorization, rate limiting, job ownership และการป้องกัน abuse/SSRF — P3-I เพิ่มการจำกัดการลอง Bearer token ผิดซ้ำแยกตาม TCP peer (5 ครั้ง/1 นาที → 429 พร้อม Retry-After, พัก 5 นาที), รองรับรายการติดตามสูงสุด 1,024 peers; **ผ่าน Go Build/Vet/Unit/Race และ Docker Compose CI (#36) แล้ว** และยังไม่มี multi-user/job ownership หรือ reverse-proxy public acceptance
- [ ] จำกัด concurrent jobs, CPU/RAM, network, disk quota, timeout, max file size/page count — P3-H เพิ่มการตรวจเพดานขนาดภาพก่อนส่งเข้า pdfcpu (สูงสุด 40 ล้านพิกเซล และ 12,000 พิกเซล/ด้าน; รองรับ 8K UHD); P3-J1 (#38) ผ่าน Go Build/Vet/Unit/Race + Docker CI สำหรับ Web-only 512-page cap และ checkpointed 512 MiB job budget (image workspace + PDF staging), พร้อมล้าง workspace เมื่อเกิน; P3-J2 (#40) เพิ่ม global managed-volume accounting และ minimum-free-space guard แบบ soft limit; Ubuntu operator verified a separate loopback ext4 6 GiB filesystem for `/data/jobs` and safe systemd startup. Controlled ENOSPC/low-disk behavior and public production gates remain deferred.
- [ ] จัดเก็บไฟล์ชั่วคราวพร้อมกำหนด retention/auto cleanup และแยกข้อมูลผู้ใช้ — เพิ่ม cleanup หลัง restart พร้อมตรวจอายุและชื่อไฟล์ของระบบแล้ว; ยังไม่มี multi-user accounts และต้องทดสอบ Volume จริง
- [ ] Logging/observability ที่ไม่เปิดเผยข้อมูลลับ, แนวทาง backup/rollback, ทดสอบ restart/recovery — P3-G (#30) ผ่าน CI ตรวจว่า Web job logs ไม่มี error ดิบ (URL/Token/Path); ยังรอการตรวจสอบ Logs/Restart บนเครื่องจริง
- [ ] ทดสอบ security และการใช้งานจริงก่อนเชื่อม public domain
- **เกณฑ์ผ่าน:** HTTPS และสิทธิ์ผู้ใช้ปลอดภัย; ไม่สามารถเข้าถึงไฟล์ข้ามงาน; ระบบมี resource caps

### P3-J2 — Hard Volume Quota / Host Capacity (LAN cutover closed)

- [x] **Code slice:** aggregate accounting of managed retained PDFs, image workspaces and PDF staging, symlink-safe scans and atomic job admission. [PR #40](https://github.com/yiwssx/anyflip-downloader/pull/40) passed Go Build/Vet/Unit/Race and Docker Compose CI.
- [x] **Host bounded filesystem / migration:** operator installed a 6 GiB ext4 loopback image under `/var/lib/anyflip-quota/jobs-6g.ext4`, registered `/mnt/anyflip-quota-stage`, and moved the active Docker data path to the `anyflip-downloader_anyflip_quota_jobs` named volume. Inside the container `df -h /data/jobs` reported **5.8 GiB**, `/dev/loop0` (rather than the 914 GiB root filesystem). Previous `anyflip-downloader_anyflip_jobs` volume was retained for rollback. Reported `QUOTA_CUTOVER_PASS`.
- [x] **Startup / LAN acceptance:** operator-provided logs show `anyflip-quota.service` enabled/active, mount unit `mnt-anyflip\\x2dquota\\x2dstage.mount` loaded/active with `Where=/mnt/anyflip-quota-stage`, `Requires/BindsTo/After` for the same unit, Docker restart policy `no`, container `healthy`, and HTTP `200` at `10.10.1.100:8094`. A later inspection after a service restart again showed the mount, 5.8 GiB filesystem, healthy container and HTTP 200. This is evidence of working startup configuration, **not a controlled simulated mount failure or a proven full reboot sequence**.
- [x] **Operator workflow closure (2026-11-01):** close the **LAN storage migration and bounded-capacity configuration task** to continue other development. No additional migration/deployment is required for this closure.
- [ ] **Deferred independent production acceptance (NOT closed):** controlled ENOSPC/low-disk test and behavior at the hard boundary, abnormal stop during a real conversion, complete authorized-book visual PDF QA in two readers, physical browsers, and any future public HTTPS/security approval. Do not test filesystem exhaustion on a shared live production host.
- **Status:** **CLOSED for LAN storage infrastructure only**; application soft limit remains 4 GiB and **a 6 GiB filesystem capacity is not evidence that a controlled fill-to-limit test passed**. See [HOST_ACCEPTANCE.md](HOST_ACCEPTANCE.md).

### P3-J3 — Crash-Orphan PDF Staging Recovery

- [x] **Code slice:** พัฒนา cleanup สำหรับโฟลเดอร์ `.anyflip-pdf-stage-*` ที่ค้างจากการปิดโปรเซสผิดปกติ โดยยืนยันชื่อที่ระบบสร้าง, ตรวจอายุของทุกไฟล์ภายใน, ไม่ตาม symlink และไม่ลบเมื่อมี Web job กำลังทำงาน; regression tests ครอบคลุม restart, active-job protection และไฟล์ที่ไม่เกี่ยวข้อง
- [ ] **Physical acceptance:** ทดสอบการหยุด Docker ระหว่างประกอบ PDF และตรวจการคืนพื้นที่จริงหลังครบ retention พร้อม low-disk/host hard quota บน Ubuntu Server; ยังห้ามปิด P3 production gate
- **สถานะ:** [PR #46](https://github.com/yiwssx/anyflip-downloader/pull/46) ผ่าน Go Build/Vet/Unit/Race/Govulncheck และ Docker Compose smoke ใน [CI 40722973213](https://github.com/yiwssx/anyflip-downloader/actions/runs/40722973213); ยังรอทดสอบ physical host

### P3-K — Strict Browser CSP / Same-Origin Assets

- [x] **Code & CI:** แยก JavaScript และ CSS ออกจาก HTML เป็นไฟล์ที่ฝังใน Go binary และเสิร์ฟบน origin เดียวกัน; ปรับ `Content-Security-Policy` ให้ไม่ใช้ `unsafe-inline`/`unsafe-eval`; เพิ่ม Go regression tests และ Node JavaScript syntax check
- [ ] **Physical browser QA:** ตรวจการใช้งาน Desktop/Mobile ทุกขั้นตอน (สร้างงาน, ความคืบหน้า, Cancel, Batch, History, ดาวน์โหลด PDF) และตรวจ CSP console บน LAN host จริง
- **สถานะ:** โค้ดผ่าน Go Build/Vet/Unit/Race, Node JS syntax, Govulncheck และ Docker Compose smoke ใน [PR #47](https://github.com/yiwssx/anyflip-downloader/pull/47) · [CI #40817348188](https://github.com/yiwssx/anyflip-downloader/actions/runs/40817348188); **Browser/host/public HTTPS gate ยัง OPEN**

### P3-L — Crash-Recovery CI and Host Acceptance Handoff

- [x] **Code/CI gate:** Docker Compose smoke proved synthetic stale-stage cleanup after SIGKILL, preservation of fresh stage, healthy restart and read-only host preflight on an ephemeral CI runner; [PR #49](https://github.com/yiwssx/anyflip-downloader/pull/49) · [CI 40836826648](https://github.com/yiwssx/anyflip-downloader/actions/runs/40836826648). Real Ubuntu host has not been tested.
- [x] **Operator handoff:** [HOST_ACCEPTANCE.md](HOST_ACCEPTANCE.md) documents local Compose preservation, filesystem quota inspection, controlled OS hard-quota proof, real authorized-book and physical browser evidence. No host migration or deployment is performed by this PR.
- [ ] **Deferred release gates:** Host bounded ext4 filesystem cutover and service startup were operator-verified and closed as an infrastructure task; controlled ENOSPC, real interruption/retention tests, authorized full-book review in two PDF viewers, physical browser acceptance, and public release approval remain OPEN.
- **No public deployment approved.**

### P4 — ความสามารถเพิ่มเติม (หลัง MVP)
- [ ] OCR ไทย/อังกฤษ เป็น **Searchable PDF** (ไม่อ้างว่าเป็น PDF ต้นฉบับ)
- [x] Batch Download / Queue / History / Retry failed tasks — [PR #42](https://github.com/yiwssx/anyflip-downloader/pull/42): authenticated atomic 8-item batch, last-50 temporary statuses, new-ID retry of failed/cancelled jobs; Go Build/Vet/Unit/Race/Govulncheck + Docker CI passed ([run 40711947203](https://github.com/yiwssx/anyflip-downloader/actions/runs/40711947203)). History is **memory-only**; durable history remains separately unchecked.
- [x] **Persistent history metadata** — [PR #44](https://github.com/yiwssx/anyflip-downloader/pull/44): เก็บเฉพาะสถานะงานที่สิ้นสุดแล้ว (สูงสุด 50 รายการ/7 วัน) ใน JSON แบบ atomic 0600 บน named volume; ไม่มี source URL, token, title, file path หรือ retry settings; หลังรีสตาร์ตดูสถานะย้อนหลังได้ แต่ไม่สามารถดาวน์โหลด PDF/Retry งานเดิมได้ (PDF retention ยัง 1 ชั่วโมง) โดยไม่เพิ่ม SQLite หรือฐานข้อมูลภายนอก. Go Build/Vet/Unit/Race/Govulncheck + Docker smoke ผ่านสำหรับโค้ดและการทดสอบ [run 40716392681](https://github.com/yiwssx/anyflip-downloader/actions/runs/40716392681); PR-head CI ผ่านทุกชุด [run 40716753587](https://github.com/yiwssx/anyflip-downloader/actions/runs/40716753587); หลัง Merge `main` ผ่านทุกชุด [run 40716907611](https://github.com/yiwssx/anyflip-downloader/actions/runs/40716907611)
- [ ] Benchmark และปรับ concurrency ตามสเปก Server จริง
- [ ] พิจารณา Desktop packaging ในอนาคตโดยใช้ Core เดียวกัน

### P5 — เอกสาร / Release
- [x] README/Runbook สำหรับ Web GUI, Docker, Apache, คำแนะนำการใช้และข้อจำกัด PDF จากภาพ (ตรวจเอกสารแล้ว ยังไม่ใช่ Production Release)
- [x] CI ทดสอบและ Build, Dependency/Security Checks, versioned release notes — [PR #41](https://github.com/yiwssx/anyflip-downloader/pull/41) passed Go/Docker CI and reachable vulnerability scan after updating x/image/x/text; release candidate is manual and does not publish.
- [x] คู่มือ Deploy/Upgrade/Rollback และ Checklist ตรวจรับก่อนปล่อยเวอร์ชัน — [docs/RELEASE_CHECKLIST.md](RELEASE_CHECKLIST.md) and existing [WEB_DEPLOYMENT.md](WEB_DEPLOYMENT.md); actual physical host acceptance remains pending.

## ลำดับงานและเงื่อนไข

`P0 → P1 → P2 → P3 → P4/P5`

> ต้องผ่าน P3 ก่อนเปิดให้บุคคลภายนอกใช้งานจริง แม้ P2 จะมีหน้าเว็บที่ทำงานได้แล้วก็ตาม

## บันทึกความคืบหน้า (ตรวจสอบ ณ 2026-10-28)

| วันที่ | งาน | สถานะ | หลักฐาน |
|---|---|---|---|
| 2026-10-22 | Master Tracker | ✅ Merged | [PR #1](https://github.com/yiwssx/anyflip-downloader/pull/1) |
| 2026-10-23 | P0-A: workspace, CLI input validation, Go CI toolchain | ✅ Merged & Go 1.25 verified | [PR #2](https://github.com/yiwssx/anyflip-downloader/pull/2) |
| 2026-10-23 | P0-B: URL, config, page paths | ✅ Merged & Go 1.25 verified | [PR #3](https://github.com/yiwssx/anyflip-downloader/pull/3) |
| 2026-10-23 | P0-C: bounded config fetch, redirects and cancellation | ✅ Merged & Go 1.25 verified | [PR #4](https://github.com/yiwssx/anyflip-downloader/pull/4) |
| 2026-10-23 | P0-D: bounded image fetch and retries | ✅ Merged & Go 1.25 verified | [PR #5](https://github.com/yiwssx/anyflip-downloader/pull/5) |
| 2026-10-23 | P0-E: complete image set and verified PDF output | ✅ Merged & Go 1.25 verified | [PR #6](https://github.com/yiwssx/anyflip-downloader/pull/6) |
| 2026-10-23 | P0-F: reusable cancellable job engine and progress events | ✅ Merged & Go 1.25 verified | [PR #7](https://github.com/yiwssx/anyflip-downloader/pull/7) |
| 2026-10-23 | P0-G: DNS/IP egress, HTTPS hardening and exponential backoff | ✅ Merged & Go 1.25 verified | [PR #8](https://github.com/yiwssx/anyflip-downloader/pull/8) |
| — | P0 verification gates | ✅ Go CI + Docker runtime CI ผ่าน; ⏳ หนังสือที่ได้รับอนุญาตและ Browser QA ยังไม่ทดสอบ | No public deployment approved |
| 2026-10-23 | P2: Thai responsive browser UI, token-auth API and in-memory job queue | ✅ Merged and Go 1.25 verified; browser/device QA pending | [PR #9](https://github.com/yiwssx/anyflip-downloader/pull/9) |
| 2026-10-23 | P3-A: hardened Docker/Compose package and Apache HTTPS template | ✅ Merged; Compose v2.40.3 config check passed; target-server deployment still pending | [PR #10](https://github.com/yiwssx/anyflip-downloader/pull/10) |
| 2026-10-23 | P3-B: restart-safe orphan cleanup for retained job PDFs/images | ✅ Merged; Go 1.25 Build/Test/Vet/Race pass | [PR #12](https://github.com/yiwssx/anyflip-downloader/pull/12) |
| 2026-10-23 | P3-C: graceful shutdown, HTTP timeouts, Docker stop grace | ✅ Merged; Go 1.25 full suite and real local SIGTERM smoke pass | [PR #13](https://github.com/yiwssx/anyflip-downloader/pull/13) |
| 2026-10-23 | P3-D: post-deployment HTTP/Compose smoke script | ✅ Merged; HTTP 200/401/404/400 smoke tests ผ่าน | [PR #14](https://github.com/yiwssx/anyflip-downloader/pull/14) |
| 2026-10-23 | P3-E: GitHub Actions Docker build, Compose smoke, and restart | ✅ Merged; Go + Docker smoke ผ่านทั้ง PR และ main | [PR #15](https://github.com/yiwssx/anyflip-downloader/pull/15) · [Actions run](https://github.com/yiwssx/anyflip-downloader/actions/runs/39898711151) |
| 2026-10-26 | Web GUI same-link diagnosis: canonical image URLs, redacted error codes, visible Job ID | ✅ Go 1.25 + GitHub CI passed; ⏳ user-hosted Docker/Apache E2E still required | [PR #17](https://github.com/yiwssx/anyflip-downloader/pull/17) |
| 2026-10-28 | P3-F: pdfcpu read-only volume HOME/config correction; LAN operator smoke | ✅ PR #18 merged; Go + Docker CI passed; operator reports Web GUI works; ⏳ complete-book output not independently verified | [PR #18](https://github.com/yiwssx/anyflip-downloader/pull/18) · [Actions run](https://github.com/yiwssx/anyflip-downloader/actions/runs/40433393217) |
| 2026-10-28 | P1-B: actual mixed JPEG/PNG/WebP import, corruption fail-closed and source immutability | ✅ Merged; Go Build/Vet/Unit/Race and Docker CI all passed on final PR head; other P1 gates still open | [PR #19](https://github.com/yiwssx/anyflip-downloader/pull/19) · [CI 40631954574](https://github.com/yiwssx/anyflip-downloader/actions/runs/40631954574) |
| 2026-10-28 | P1-C: publisher-advertised image fallback after 404/410 only | ✅ Merged; Go Build/Vet/Unit/Race + Docker CI passed; resolution-quality proof and full authorized book remain open | [PR #22](https://github.com/yiwssx/anyflip-downloader/pull/22) · [CI 40638593872](https://github.com/yiwssx/anyflip-downloader/actions/runs/40638593872) |
| 2026-10-28 | P1-D: real PDF per-page MediaBox geometry, portrait/landscape order across PDF chunks | ✅ Merged; Go Build/Vet/Unit/Race + Docker CI passed; EXIF rotation and browser rendering still open | [PR #23](https://github.com/yiwssx/anyflip-downloader/pull/23) · [CI 40638773859](https://github.com/yiwssx/anyflip-downloader/actions/runs/40638773859) |
| 2026-10-28 | P1-D2: JPEG EXIF orientation 1–8 in isolated PDF staging + MediaBox regression | ✅ Merged; Build/Vet/Unit/Race + Docker CI passed. Physical reader visual QA and full-book acceptance open | [PR #25](https://github.com/yiwssx/anyflip-downloader/pull/25) · [CI 40657393852](https://github.com/yiwssx/anyflip-downloader/actions/runs/40657393852) |
| 2026-10-28 | P1-E: opt-in high-pixel-resolution selection from publisher-declared choices (CLI/Web) | ✅ Merged; Go Build/Vet/Unit/Race + Docker CI passed; full authorized book and visual QA still OPEN | [PR #27](https://github.com/yiwssx/anyflip-downloader/pull/27) · [CI 40658697472](https://github.com/yiwssx/anyflip-downloader/actions/runs/40658697472) |
| 2026-10-28 | P1-E Web QA regression: browser-form markup, authenticated opt-in job API and PDF geometry using mock-origin fixtures | ✅ Merged; Go Build/Vet/Unit/Race and Docker CI passed; physical browser, LAN host and live book validation still OPEN | [PR #28](https://github.com/yiwssx/anyflip-downloader/pull/28) · [CI 40659004326](https://github.com/yiwssx/anyflip-downloader/actions/runs/40659004326) |
| 2026-10-28 | P3-G: Web job log privacy — no raw source URL, credentials, workspace or transport details | ✅ Merged; Go Build/Vet/Unit/Race and Docker CI passed; server-side observability and public readiness remain OPEN | [PR #30](https://github.com/yiwssx/anyflip-downloader/pull/30) · [CI 40662371036](https://github.com/yiwssx/anyflip-downloader/actions/runs/40662371036) |
| 2026-10-28 | P1-F: synthetic 129-page complete PDF, chunk order, missing/late-corrupt page fail-closed | ✅ Merged; Build/Vet/Unit/Race + Docker CI passed; not evidence of a real authorized book or large-server benchmark | [PR #31](https://github.com/yiwssx/anyflip-downloader/pull/31) · [CI 40662689066](https://github.com/yiwssx/anyflip-downloader/actions/runs/40662689066) |
| 2026-10-28 | P3-H: reject oversized decoded image dimensions before PDF import; safe Web error | ✅ Merged; Go Build/Vet/Unit/Race + Docker CI passed; target-host memory and disk quota gates OPEN | [PR #33](https://github.com/yiwssx/anyflip-downloader/pull/33) · [CI 40665058331](https://github.com/yiwssx/anyflip-downloader/actions/runs/40665058331) |
| 2026-10-28 | P1-G: explicitly label CLI, GUI and PDF HTTP download as image-based (not source vector/text PDF) | ✅ Merged; Go Build/Vet/Unit/Race and Docker CI passed; unrelated real-book viewer acceptance remains OPEN | [PR #34](https://github.com/yiwssx/anyflip-downloader/pull/34) · [CI 40665519907](https://github.com/yiwssx/anyflip-downloader/actions/runs/40665519907) |
| 2026-10-28 | P3-I: bounded failed-token throttle for Web API | ✅ Merged; Go Build/Vet/Unit/Race and Docker Compose smoke passed on final PR head; target Ubuntu and public authorization gates OPEN | [PR #36](https://github.com/yiwssx/anyflip-downloader/pull/36) · [CI 40702293824](https://github.com/yiwssx/anyflip-downloader/actions/runs/40702293824) |
| 2026-10-28 | P3-J1: checkpointed per-Web-job page/storage budget and fail-closed PDF publishing | ✅ Merged, Go Build/Vet/Unit/Race and Docker Compose smoke passed; **only soft job limit**, hard volume quota and target-host capacity still OPEN | [PR #38](https://github.com/yiwssx/anyflip-downloader/pull/38) · [CI 40704369464](https://github.com/yiwssx/anyflip-downloader/actions/runs/40704369464) |\n| 2026-10-28 | P3-J2 code admission: managed aggregate 4 GiB soft ceiling, nonroot free-space guard and concurrent reservations | ✅ Merged; PR/main Go+Docker CI passed; **hard host quota OPEN** | [PR #40](https://github.com/yiwssx/anyflip-downloader/pull/40) · [CI 40709563153](https://github.com/yiwssx/anyflip-downloader/actions/runs/40709563153) · [main 40709998946](https://github.com/yiwssx/anyflip-downloader/actions/runs/40709998946) |
| 2026-10-28 | P5 CI hardening, remediation of 5 reachable Go vulnerabilities, manual release candidate packaging and acceptance checklist | ✅ Merged; Go/Docker/Govulncheck CI passed; no tag/release published | [PR #41](https://github.com/yiwssx/anyflip-downloader/pull/41) · [CI 40710618424](https://github.com/yiwssx/anyflip-downloader/actions/runs/40710618424) |
| 2026-10-28 | P4 admin-only batch, transient job history and failed/cancelled retry | ✅ Merged; Go/Docker/Govulncheck CI passed; no persistent DB or multi-user identity | [PR #42](https://github.com/yiwssx/anyflip-downloader/pull/42) · [CI 40711947203](https://github.com/yiwssx/anyflip-downloader/actions/runs/40711947203) · [main 40712120907](https://github.com/yiwssx/anyflip-downloader/actions/runs/40712120907) |
| 2026-10-28 | P4 terminal Web history: 50 redacted JSON records / seven-day metadata retention across restart, separate one-hour PDF cleanup; no persisted source URL/token/retry settings | ✅ Merged to `main`; PR final-head [40716753587](https://github.com/yiwssx/anyflip-downloader/actions/runs/40716753587) and post-merge main [40716907611](https://github.com/yiwssx/anyflip-downloader/actions/runs/40716907611) Go/Docker/security CI all green | [PR #44](https://github.com/yiwssx/anyflip-downloader/pull/44) |
| 2026-10-28 | P3-J3 reclaim crash-orphan PDF staging dirs after one hour, only with no active Web job and safe filesystem entries | ✅ Go Build/Vet/Unit/Race/Govulncheck + Docker CI verified; host abrupt-restart acceptance OPEN | [PR #46](https://github.com/yiwssx/anyflip-downloader/pull/46) · [CI 40722973213](https://github.com/yiwssx/anyflip-downloader/actions/runs/40722973213) |
| 2026-10-29 | P3-K same-origin compiled Web JS/CSS, deny-by-default CSP without unsafe-inline, Node syntax and HTTP regression tests | ✅ PR Go/Node/Docker/security CI passed; physical browser acceptance remains OPEN | [PR #47](https://github.com/yiwssx/anyflip-downloader/pull/47) · [CI 40817348188](https://github.com/yiwssx/anyflip-downloader/actions/runs/40817348188) |
| 2026-10-29 | P2-E2E Chromium desktop/mobile workflows, synthetic API tests for job progress/download/cancel/batch/history/retry and path-filtered CI | ✅ Browser/Go/Docker PR CI passed; real LAN browser QA OPEN | [PR #48](https://github.com/yiwssx/anyflip-downloader/pull/48) · [Browser 40834513098](https://github.com/yiwssx/anyflip-downloader/actions/runs/40834513098) · [Go/Docker 40834513180](https://github.com/yiwssx/anyflip-downloader/actions/runs/40834513180) |
| 2026-10-29 | P3-L synthetic Docker SIGKILL/stale-stage recovery + token-free read-only Ubuntu preflight and hard-quota handoff | ✅ CI code gate passed; real host hard quota, physical acceptance and public release OPEN | [PR #49](https://github.com/yiwssx/anyflip-downloader/pull/49) · [CI 40836826648](https://github.com/yiwssx/anyflip-downloader/actions/runs/40836826648) |
| 2026-11-01 | P3-J2 LAN hard-storage filesystem cutover with operator-confirmed 5.8 GiB ext4 `/data/jobs`, named volume, systemd mount dependency, and HTTP 200 | ✅ CLOSED (LAN storage work only); controlled ENOSPC and public production acceptance deferred | Operator-provided terminal verification (private); [HOST_ACCEPTANCE.md](HOST_ACCEPTANCE.md) |
| — | P1/P2/P3 real host/browser/authorized-book acceptance; P4 OCR/target-host performance/desktop; P5 public release | ⏳ OPEN (not verified; do not mark complete) | Operator host/permission/production sign-off required |

### P1 acceptance evidence and remaining gates

- **Verified in CI:** PR #19 real JPEG/PNG/WebP import and fail-closed corruption; PR #22 publisher-declared 404/410-only fallback + full two-page PDF regression and refusal to fall back on auth/rate-limit/redirect; PR #23 4-page mixed MediaBox aspect/ordering, original SHA-256 and incomplete-output rejection; PR #25 EXIF orientation 1–8, both TIFF endiannesses, isolated lossless PNG normalization, source preservation, rotated PDF MediaBox and malformed Exif rejection; PR #27/#28 opt-in quality selection and Web API from publisher-listed mock-origin images; PR #31 synthetic 129-page PDF integrity and late-page fail-closed regressions.
- **Not verified:** highest **actual** pixel resolution across all publisher choices, visual EXIF fidelity in multiple PDF viewers, page/crop/Thai text fidelity, full publisher-authorized book, browser/mobile, target Ubuntu storage and runtime limits. For rotated JPEGs above 25 million pixels the system fails explicitly; this is a documented limit, not proof of support. A release claim requires these independently recorded checks.
- [P1 verification matrix](P1_VERIFICATION.md) records repeatable acceptance steps without publishing a real token, copied pages or server-private logs.

### Real verification evidence

- Target: Ubuntu 26.04 sandbox, Go 1.25.0, actual `go.mod` dependencies.
- Executed: `gofmt` check, `go build ./...`, `go vet ./...`, `go test -count=1 ./...`, `CGO_ENABLED=1 go test -race -count=2 ./...` (all passed on each merged slice and the final Core branch).
- Includes synthetic image-to-PDF integration tests via `pdfcpu`, page count/atomic output checks and cancellation.
- GitHub Actions has confirmed **successful** Go Build/Vet/Unit/Race and Docker image/Compose runtime smoke on PR #15, including restart and post-restart checks: [run 39898711151](https://github.com/yiwssx/anyflip-downloader/actions/runs/39898711151). The equivalent post-merge `main` run 39898810788 also completed successfully. PR #19's actual mixed JPEG/PNG/WebP import and corruption regression cases passed on final-head [run 40631954574](https://github.com/yiwssx/anyflip-downloader/actions/runs/40631954574), along with the Docker smoke job.
- Docker deployment config: Compose v2.40.3 static validation passed in sandbox; **real Docker image build, non-root container boot, API probes and restart** later passed on GitHub-hosted Actions runner. This does not establish deployment on the user's Ubuntu server.
- Operator feedback on 2026-10-28 confirms the self-hosted LAN Web GUI works after PR #18's writable pdfcpu configuration change. This is **not** proof of a complete authorized book, cross-device browser acceptance, public Apache/HTTPS deployment, or production security readiness. Preserve the existing host-specific LAN bind (`10.10.1.100:8094`); the stock Compose template intentionally remains loopback-only.
- Full end-to-end production Docker/Apache/HTTPS and physical browser/mobile QA remain open. P3-A includes configuration templates, not proof of public deployment.

### Code-only handoff (2026-10-28)

- Verified runtime work (#40 capacity admission, #42 batch/temporary history/retry, #44 redacted durable history metadata) and CI/security/release-candidate guard (#41) are merged into `main`. The exact post-merge `main` CI run for #42 passed: [40712120907](https://github.com/yiwssx/anyflip-downloader/actions/runs/40712120907).
- The 4 GiB managed-volume ceiling and 512 MiB job reservations are application-level checks, **not hard disk quotas**. The operator subsequently configured a distinct approximately 6 GiB ext4 filesystem for LAN deployment and verified container-visible capacity; controlled ENOSPC enforcement, multi-user isolation and public security approval remain unverified.
- P1 live authorized-book visual QA, P2 browser/mobile testing and remaining target-host HTTPS/controlled-ENOSPC/recovery/security release gates are **OPEN**. P3-J2 LAN storage migration has been closed separately. LAN-only operation is separate from an approved public release.
- P4 safe durable **status-only history** is merged as [PR #44](https://github.com/yiwssx/anyflip-downloader/pull/44) via bounded atomic JSON metadata (not SQLite), with successful PR/main Go/Docker/security CI. It does not retain URLs, tokens or retry specs, recover pending jobs, or extend PDF lifetime. OCR, target-machine benchmarks and optional desktop packaging remain independent, uncompleted features.
- P5 candidate workflow is manual and intentionally does **not** publish any GitHub release. Acceptance checklist exists; full Release gate remains OPEN until operator authorization.

## ข้อกำหนดการติดตาม

- ทุกงานที่เริ่มต้องมี Branch/PR แยก, อ้างอิง Tracker นี้, พร้อมสถานะและผล CI
- เปลี่ยน checkbox เป็นเสร็จ **เมื่อมีหลักฐานทดสอบผ่านเท่านั้น**; ห้ามนับการเปิด PR ว่าเสร็จ
- ใส่ลิงก์ PR/Commit และวันที่เมื่อแต่ละ Phase ผ่าน Gate
- ไม่แก้หรือ Merge ไป `main` โดยข้ามขั้น Review/Verification
