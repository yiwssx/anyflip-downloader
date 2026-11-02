# AnyFlip DNS / SaaS-O2O incident diagnostics

Incident: https://github.com/yiwssx/new-www.rcat.ac.th/issues/786
Cross-project DNS tracker: https://github.com/yiwssx/new-www.rcat.ac.th/blob/main/docs/workstreams/dns-cross-project-integrity-tracker.md

## Confirmed constraints — do not ask owner to reconfirm
- Institutional authoritative zone rcat.ac.th uses maria/kirk in its domain operator account.
- Domain operator changed only www CNAME for SaaS/O2O. The provider rcat.dpdns.org uses separate roan/serena NS.
- AnyFlip A=202.29.239.145 was added as DNS-only, same as longstanding rms/pdf/iot/radio hosts.
- AnyFlip public DNS unexpectedly returns Cloudflare anycast IPs even though the intended setting is DNS-only.
- Internal AnyFlip GUI on 10.10.1.100:8094/healthz and RMS direct-origin HTTPS worked in prior internal probes.
- The provider SaaS account only registers www as a custom hostname. Shared edge IP alone does not prove shared origin.

## Run — non-destructive, no root
    bash scripts/diagnose-public-dns.sh

The script reads both institutional authoritative nameservers, compares www/RMS, tests normal public HTTPS, and tests direct RMS HTTPS using curl --resolve while preserving Host and TLS SNI. It never uses -k or changes DNS, proxy, server, firewall, containers or certificates.

Timeouts from an external vantage point are not proof of a global outage; network policy and geography may influence results.

## Containment
- Never redirect all institutional subdomains into the O2O gateway as a workaround.
- Never change institutional NS, www CNAME, RMS origin or sibling DNS records to repair AnyFlip.
- The institutional all-record DNS drift monitor is owned by the website workstream; do not create duplicate scheduled monitoring in this repo.

## Acceptance criteria
- maria and kirk return anyflip A=202.29.239.145 with no unapproved IPv6 or CNAME.
- Public HTTPS /healthz works from authorized external locations.
- www SaaS/O2O remains operational and all institutional subdomains preserve their DNS records.
- Verified production restoration recorded on incident #786 before closure.
