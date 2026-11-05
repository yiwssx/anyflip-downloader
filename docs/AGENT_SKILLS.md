# Codex / Coding Agent Skills — AnyFlip Downloader

Skills เหล่านี้เป็นไฟล์เฉพาะ Repository ไม่ใช่การติดตั้ง Plugin หรือเครื่องมือลงบน Production Server และไม่เพิ่ม Runtime Dependencies ของ Go.

## ติดตั้ง / ใช้งาน

1. Checkout หรืออัปเดต Repository ที่มีโฟลเดอร์ `.agents/skills/` และ `AGENTS.md`.
2. เปิด Codex จากโฟลเดอร์รากของ Repository; Codex จะค้นพบ Skills โดยอัตโนมัติจาก `.agents/skills/`.
3. เปิดรายการ Skills ใน Codex ด้วย `/skills` หรือเรียกใช้แบบระบุชื่อ เช่น `$anyflip-go-pdf` และ `$anyflip-google-rbac`.
4. หาก Skills ไม่ปรากฏหลังอัปเดต ให้เริ่ม Codex Session ใหม่.

## เลือก Skill ให้ตรงงาน

| งาน | Skill |
|---|---|
| ดาวน์โหลด/แปลง PDF/คุณภาพภาพ | `$anyflip-go-pdf` |
| OAuth, Session, RBAC, Admin Console | `$anyflip-google-rbac` |
| หน้าเว็บ, Branding, Accessibility, Browser E2E | `$anyflip-web-ui` |
| Ubuntu, Docker, Storage Quota, Tunnel, Rollback | `$anyflip-docker-ops` |
| Security Audit, SSRF, Authentication, Secrets | `$anyflip-security-review` |
| CI, PR, Vulnerability, Release, Tracker | `$anyflip-ci-release` |
| แก้ Incident, อ่าน Log, CLI/Web แตกต่าง | `$anyflip-incident-triage` |

## ตรวจสอบและอัปเดต

Run: `python3 scripts/validate-agent-skills.py`. The `build` GitHub Actions workflow runs the same check automatically.

ทุก Skill มีไฟล์ `.agents/skills/<skill-name>/SKILL.md` ซึ่งกำหนด Trigger และขั้นตอนเฉพาะ พร้อมอ้างอิง Runbook ที่มีอยู่แล้วใน `docs/`. แก้ไขด้วย Branch + PR ตาม `AGENTS.md`.

หมายเหตุ: ผล CI, การผ่าน Browser E2E และคำยืนยันจากผู้ดูแลเซิร์ฟเวอร์เป็นหลักฐานคนละประเภท ห้ามสรุปว่า Deploy Production ผ่านจาก CI เพียงอย่างเดียว.
