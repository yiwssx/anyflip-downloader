---
name: anyflip-incident-triage
description: Use to diagnose anyflip.rcat.ac.th or LAN downloader incidents, job failures, PDF conversion errors, DNS/Tunnel/Apache problems, quota faults and CLI-versus-Web discrepancies.
---

# anyflip-incident-triage

## Evidence-first triage
1. Identify exact route and phase: DNS -> Cloudflare/Tunnel/Apache -> app health/auth -> metadata -> image downloads -> pdfcpu conversion -> volume persistence -> response download.
2. Ask for or read bounded, redacted operator logs and job error codes; avoid copying tokens, user document URLs, personal account IDs, entire role stores or environment files.
3. Contrast the official stock Compose template with the operator's actual deployment. Confirm which IP is bound: 10.10.1.100:8094 is not 127.0.0.1:8094. A loopback probe can fail even if the LAN binding is healthy.
4. Check container DNS/HTTPS separately from host CLI. A successful config.js HTTP 200 does not prove PDF conversion or authenticated Web completion.
5. For pdfcpu config errors on read-only rootfs, verify HOME and XDG_CONFIG_HOME resolve to writable service-owned job storage; do not disable read_only rootfs.
6. For disk issues, distinguish application soft budget from hard ext4 quota; inspect mount and service dependency without changing persistent data.
7. Reproduce safely with synthetic/authorized fixtures, isolate one failure boundary, add regression tests, submit focused PR, and report whether the live host has actually been retested.

## References
docs/DNS_O2O_INCIDENT.md, docs/CONTROLLED_TUNNEL_FALLBACK.md, docs/HOST_ACCEPTANCE.md, docs/WEB_DEPLOYMENT.md and docs/GOOGLE_CUTOVER.md.
