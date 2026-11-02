#!/usr/bin/env bash
# Read-only AnyFlip public DNS and HTTPS diagnostics. Never edits production.
set -Eeuo pipefail
DOMAIN="${ANYFLIP_DOMAIN:-anyflip.rcat.ac.th}"
ORIGIN="${ANYFLIP_PUBLIC_ORIGIN_IP:-202.29.239.145}"

printf '%s\n' '===== AnyFlip authoritative DNS (A / AAAA / CNAME) ====='
for ns in maria.ns.cloudflare.com kirk.ns.cloudflare.com; do
  echo "--- $ns"
  for type in A AAAA CNAME; do
    printf '%s: ' "$type"
    dig +time=3 +tries=1 +short @"$ns" "$DOMAIN" "$type" | tr '\n' ' '
    printf '\n'
  done
done

printf '%s\n' '===== Compare canonical www and RMS DNS ====='
for host in www.rcat.ac.th rms.rcat.ac.th; do
  printf '%s: ' "$host"
  dig +time=3 +tries=1 +short @maria.ns.cloudflare.com "$host" A | tr '\n' ' '
  printf '\n'
done

printf '%s\n' '===== Public HTTPS ====='
if ! curl --silent --show-error --head --connect-timeout 5 --max-time 15 \
  --output /dev/null \
  --write-out 'public_http=%{http_code} peer=%{remote_ip} tls_verify=%{ssl_verify_result} total=%{time_total}s\n' \
  "https://$DOMAIN/healthz"; then
  echo 'PUBLIC_PATH_UNAVAILABLE (DNS, edge, origin, or network-policy failure)'
fi

printf '%s\n' '===== Direct RMS HTTPS (preserve Host and SNI) ====='
if ! curl --silent --show-error --head --connect-timeout 5 --max-time 15 \
  --resolve "$DOMAIN:443:$ORIGIN" \
  --output /dev/null \
  --write-out 'origin_http=%{http_code} peer=%{remote_ip} tls_verify=%{ssl_verify_result} total=%{time_total}s\n' \
  "https://$DOMAIN/healthz"; then
  echo 'ORIGIN_PATH_UNAVAILABLE_FROM_THIS_PROBE (not proof of global outage)'
fi
printf '%s\n' 'Diagnostics complete; no configuration changed.'
