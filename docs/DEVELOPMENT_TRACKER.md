# AnyFlip Downloader — Online Web GUI Development Tracker

**สถานะ:** 🟡 P0 Core built and verified with Go 1.25; GitHub Actions and authorized live-book verification outstanding; Web GUI not deployed  
**อัปเดตครั้งแรก:** 2026-10-22  
**เป้าหมาย:** Web GUI ผ่านเบราว์เซอร์ บน Ubuntu Server + Docker/Compose + Apache Reverse Proxy + HTTPS โดยคง CLI เดิมไว้

## ข้อเท็จจริงและขอบเขต

- โปรแกรมเดิมดาวน์โหลดภาพหน้าเอกสาร แล้วใช้ `pdfcpu` รวมเป็น **Image-based PDF**; **ไม่ใช่ PDF ต้นฉบับ** ที่เก็บข้อความ/Vector ไว้ครบ
- ใช้สำหรับเอกสารที่ผู้ใช้มีสิทธิ์ดาวน์โหลด/ทำสำเนาเท่านั้น ไม่เพิ่มฟีเจอร์ข้ามการจำกัดสิทธิ์, DRM หรือระบบยืนยันตัวตน
- เวอร์ชันแรก: **Go Backend + Responsive Web UI** เสิร์ฟจากแอปเดียวใน Docker; Apache เป็น HTTPS Reverse Proxy
- ไม่ติดตั้งฐานข้อมูล/OCR ตั้งแต่วันแรกโดยไม่จำเป็น; เน้นความเสถียร ความปลอดภัย และไฟล์ผลลัพธ์ถูกต้องก่อน
- **หนึ่ง PR ต่อหนึ่งงาน** พร้อมทดสอบและตรวจรับก่อน Merge; Tracker นี้เป็นศูนย์กลางติดตาม

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
- [ ] จัด Go version ให้ตรงกันระหว่าง `go.mod`, Docker และ CI; เพิ่ม unit/integration tests — Go/Docker/Workflow ตรงกันและทดสอบจริงผ่าน; GitHub Actions ยังแสดง 0 runs จึงยังไม่มี GitHub-native CI evidence
- **เกณฑ์ผ่าน:** CLI เดิมทำงาน, `go test ./...` และ build ผ่าน, Cancel ได้, output ครบและปลอดภัย

### P1 — คุณภาพภาพและการแปลง PDF
- [ ] เลือกภาพคุณภาพสูงสุดที่เข้าถึงได้ **โดยได้รับอนุญาต** พร้อม fallback ที่ชัดเจน
- [ ] รองรับ JPG/PNG/WebP ตามความสามารถตัวแปลง และตรวจการจัดเรียง/หมุน/ขนาดหน้า
- [ ] แสดงประเภทผลลัพธ์เป็น **Image-based PDF** อย่างชัดเจน
- [ ] เพิ่ม test fixtures สำหรับหน้าขาด ภาพเสีย ชื่อต่างรูปแบบ จำนวนหน้ามาก
- **เกณฑ์ผ่าน:** เอกสารทดสอบที่ได้รับอนุญาตรวมเป็น PDF ได้ครบ เรียงหน้าและเปิดอ่านได้

### P2 — Web GUI MVP
- [ ] หน้าเว็บ Responsive ใช้ได้ทั้ง Desktop/Mobile และข้อความภาษาไทย
- [ ] ฟอร์ม URL, ตรวจ metadata ที่หาได้, ชื่อไฟล์, เริ่มงาน, แสดงสถานะ, ยกเลิก, ดาวน์โหลดไฟล์
- [ ] Go HTTP API สำหรับ job lifecycle และ SSE (หรือวิธีเทียบเท่า) สำหรับ progress แบบ real-time
- [ ] ระบุสถานะ queued → preparing → downloading → converting → completed / failed / cancelled
- [ ] มี Job ID, แยกไฟล์แต่ละงาน, จำกัดการเข้าถึงไฟล์ และไม่เผย filesystem path
- [ ] Loading, error, retry guidance, accessibility พื้นฐาน; คง CLI ทำงานต่อได้
- **เกณฑ์ผ่าน:** ทดสอบ end-to-end บนมือถือและ Desktop ผ่านทั้งกรณีสำเร็จ ล้มเหลว และ Cancel

### P3 — Server / Docker / Security (ต้องผ่านก่อนเปิดใช้งานออนไลน์)
- [ ] Multi-stage Docker image, ไม่รันเป็น root, Compose + volumes ที่เหมาะสม
- [ ] Apache Reverse Proxy + HTTPS + security headers + health check
- [ ] Authentication / Authorization, rate limiting, job ownership และการป้องกัน abuse/SSRF
- [ ] จำกัด concurrent jobs, CPU/RAM, network, disk quota, timeout, max file size/page count
- [ ] จัดเก็บไฟล์ชั่วคราวพร้อมกำหนด retention/auto cleanup และแยกข้อมูลผู้ใช้
- [ ] Logging/observability ที่ไม่เปิดเผยข้อมูลลับ, แนวทาง backup/rollback, ทดสอบ restart/recovery
- [ ] ทดสอบ security และการใช้งานจริงก่อนเชื่อม public domain
- **เกณฑ์ผ่าน:** HTTPS และสิทธิ์ผู้ใช้ปลอดภัย; ไม่สามารถเข้าถึงไฟล์ข้ามงาน; ระบบมี resource caps

### P4 — ความสามารถเพิ่มเติม (หลัง MVP)
- [ ] OCR ไทย/อังกฤษ เป็น **Searchable PDF** (ไม่อ้างว่าเป็น PDF ต้นฉบับ)
- [ ] Batch Download / Queue / History / Retry failed tasks
- [ ] Persistence (เช่น SQLite) หากมีการใช้งานหลายงานหรือจำเป็นต้องกลับมาดูประวัติ
- [ ] Benchmark และปรับ concurrency ตามสเปก Server จริง
- [ ] พิจารณา Desktop packaging ในอนาคตโดยใช้ Core เดียวกัน

### P5 — เอกสาร / Release
- [ ] README สำหรับ Web GUI, Docker, Apache, คำแนะนำการใช้และข้อจำกัด PDF จากภาพ
- [ ] CI ทดสอบและ Build, Dependency/Security Checks, versioned release notes
- [ ] คู่มือ Deploy/Upgrade/Rollback และ Checklist ตรวจรับก่อนปล่อยเวอร์ชัน

## ลำดับงานและเงื่อนไข

`P0 → P1 → P2 → P3 → P4/P5`

> ต้องผ่าน P3 ก่อนเปิดให้บุคคลภายนอกใช้งานจริง แม้ P2 จะมีหน้าเว็บที่ทำงานได้แล้วก็ตาม

## บันทึกความคืบหน้า (ตรวจสอบ ณ 2026-10-23)

| วันที่ | งาน | สถานะ | หลักฐาน |
|---|---|---|---|
| 2026-10-22 | Master Tracker | ✅ Merged | [PR #1](https://github.com/yiwssx/anyflip-downloader/pull/1) |
| 2026-10-23 | P0-A: workspace, CLI input validation, Go CI toolchain | ✅ Merged & Go 1.25 verified | [PR #2](https://github.com/yiwssx/anyflip-downloader/pull/2) |
| 2026-10-23 | P0-B: URL, config, page paths | ✅ Merged & Go 1.25 verified | [PR #3](https://github.com/yiwssx/anyflip-downloader/pull/3) |
| 2026-10-23 | P0-C: bounded config fetch, redirects and cancellation | ✅ Merged & Go 1.25 verified | [PR #4](https://github.com/yiwssx/anyflip-downloader/pull/4) |
| 2026-10-23 | P0-D: bounded image fetch and retries | ✅ Merged & Go 1.25 verified | [PR #5](https://github.com/yiwssx/anyflip-downloader/pull/5) |
| 2026-10-23 | P0-E: complete image set and verified PDF output | ✅ Merged & Go 1.25 verified | [PR #6](https://github.com/yiwssx/anyflip-downloader/pull/6) |
| 2026-10-23 | P0-F: reusable cancellable job engine and progress events | ✅ Merged & Go 1.25 verified | [PR #7](https://github.com/yiwssx/anyflip-downloader/pull/7) |
| 2026-10-23 | P0-G: DNS/IP egress, HTTPS hardening and exponential backoff | 🟡 Pending PR review | [PR #8](https://github.com/yiwssx/anyflip-downloader/pull/8) |
| — | P0 verification gates | ⏳ GitHub Actions + permitted live-book checks outstanding | No public deployment approved |
| — | P1–P5: quality, Web GUI, server hardening, features and release | ⏳ Not started | No hosted web UI claimed |

### Real verification evidence

- Target: Ubuntu 26.04 sandbox, Go 1.25.0, actual `go.mod` dependencies.
- Executed: `gofmt` check, `go build ./...`, `go vet ./...`, `go test -count=1 ./...`, `CGO_ENABLED=1 go test -race -count=2 ./...` (all passed on each merged slice and the final Core branch).
- Includes synthetic image-to-PDF integration tests via `pdfcpu`, page count/atomic output checks and cancellation.
- **These are sandbox full-suite tests, not GitHub Actions checks.** Actions remains unverified (0 workflow runs).
- End-to-end tests against live, publisher-authorized AnyFlip books and production Docker/Apache/HTTPS have **not** been performed.

## ข้อกำหนดการติดตาม

- ทุกงานที่เริ่มต้องมี Branch/PR แยก, อ้างอิง Tracker นี้, พร้อมสถานะและผล CI
- เปลี่ยน checkbox เป็นเสร็จ **เมื่อมีหลักฐานทดสอบผ่านเท่านั้น**; ห้ามนับการเปิด PR ว่าเสร็จ
- ใส่ลิงก์ PR/Commit และวันที่เมื่อแต่ละ Phase ผ่าน Gate
- ไม่แก้หรือ Merge ไป `main` โดยข้ามขั้น Review/Verification
