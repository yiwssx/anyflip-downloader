---
name: anyflip-security-review
description: Use for threat modeling and security reviews of URL fetches, DNS/SSRF, Google auth, RBAC, CSRF, XSS, headers, privacy, storage quotas, dependency vulnerabilities and Docker isolation.
---

# anyflip-security-review

## Threat-driven review
1. Define the trust boundary and attacker-controlled input: book metadata and image paths; HTTP redirects and DNS; browser requests; OAuth callbacks; job IDs; persisted roles/audit; container volumes.
2. For network changes, require HTTPS, official AnyFlip host allowlist, strict redirect policy, DNS/IP public-only egress and size/time bounds. Prevent path escape, redirect laundering and privilege escalation.
3. For auth, validate the Google signature and claim set, same-origin/session behavior, RBAC role checks, job ownership and non-leaky error responses. Never treat Admin visibility as cross-owner PDF access.
4. For storage, reject symlinks, wrong ownership/permissions and path traversal, use atomic writes, constrained retention, bounded queues and quota-aware capacity. Preserve no-new-privileges and read-only rootfs.
5. Add at least one negative regression test per new trust boundary. Avoid generating a policy-only review that leaves a reproducible bug unfixed.
6. Run go test -race ./..., go vet ./..., go mod verify, and review GitHub CI govulncheck results. Use tools only in environments authorized for security testing.

## Report
Classify exploitable vs theoretical findings, cite affected file/function, provide safe reproducer, severity, proposed regression test and exact CI/host verification limits. Never expose credentials or private logs.
