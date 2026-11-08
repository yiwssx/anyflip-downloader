# RCAT AnyFlip — update latest UI/UX on Ubuntu host

**Operator-only, production-impacting if executed.** Nothing in this document automatically deploys. The existing production host is `10.10.1.100`, serving `https://anyflip.rcat.ac.th` with Google Workspace SSO and a host-managed quota. Preserve current `compose.quota.yaml`, private `.env`, named volume, RBAC store, `anyflip-quota.service`, DNS/Tunnel/Apache.

## 1. Preserve state and inspect tracked local overrides

Run **on the Ubuntu host**:

```bash
cd /opt/anyflip-downloader
sudo git status --short
sudo git log -1 --oneline
sudo test -f .env && sudo test -f compose.quota.yaml || { echo 'STOP: private env or quota override missing'; exit 1; }
sudo systemctl is-active --quiet anyflip-quota.service || { echo 'STOP: quota service inactive'; exit 1; }
stamp="$(date +%Y%m%d-%H%M%S)"
sudo install -d -m 700 /root/anyflip-deploy-backups
sudo cp -p .env "/root/anyflip-deploy-backups/env-$stamp"
sudo cp -p compose.quota.yaml "/root/anyflip-deploy-backups/compose.quota-$stamp.yaml"
sudo git rev-parse HEAD | sudo tee "/root/anyflip-deploy-backups/source-$stamp.txt" >/dev/null
```

If **only** `compose.web.yaml` is modified (the prior operator hit this exact Git error), inspect that diff on the host and confirm its live-specific settings are preserved in the quota overlay. Only after reviewing it, park that tracked file **reversibly**:

```bash
sudo git diff -- compose.web.yaml
sudo git stash push -m "RCAT host compose override $stamp" -- compose.web.yaml
```

If other tracked files are modified or there are unresolved host differences, resolve them individually; do not run `git reset --hard` or blindly `git restore` files. Do **not** automatically pop the stash after upgrade: it might override new security defaults.

## 2. Pull and run READ-ONLY preflight (must pass)

```bash
sudo git pull --ff-only origin main
sudo git rev-parse HEAD
sudo docker compose --env-file .env -f compose.web.yaml -f compose.quota.yaml config --quiet
sudo bash scripts/preflight-google-cutover.sh
```

Preflight must validate `ANYFLIP_AUTH_MODE=google`, `ANYFLIP_HOST_BIND_IP=10.10.1.100`, redirect `https://anyflip.rcat.ac.th/auth/google/callback`, private Google configuration, existing `/data/jobs` volume, `anyflip-quota.service` and both existing health endpoints. A `BLOCK:` result means **STOP**. Never paste `.env` values or resolved Compose YAML in a ticket or chat.

## 3. Deploy only the Web service, during an approved maintenance window

```bash
sudo docker compose --env-file .env -f compose.web.yaml -f compose.quota.yaml up -d --build --no-deps anyflip-web
sudo docker compose --env-file .env -f compose.web.yaml -f compose.quota.yaml ps anyflip-web
```

Recreation can interrupt active jobs/in-memory sessions. This does not recreate Cloudflare Tunnel, quotas or other services. **Never run `docker compose down -v`.**

## 4. Verify health and authentication (without sharing secrets)

```bash
curl --noproxy '*' -fsS http://10.10.1.100:8094/healthz
curl -fsS https://anyflip.rcat.ac.th/healthz
curl -sS -o /dev/null -w 'anonymous API HTTP %{http_code}\n' https://anyflip.rcat.ac.th/api/jobs
curl -sS -o /dev/null -w 'Google redirect HTTP %{http_code}\n' https://anyflip.rcat.ac.th/auth/google
curl -fsS https://anyflip.rcat.ac.th/ | grep 'mode-google'
```

Expected: both health 200, anonymous jobs 401, Google redirect 303, and `mode-google` HTML. Real two-account SSO/RBAC/IDOR, authorized document/PDF, mobile/screen-reader, quota and rollback evidence are **still required** before #101 can close.

## Recovery

Before upgrade, note the *running* image ID and previous source SHA privately (see `docs/UIH_RELEASE_ACCEPTANCE.md`). If security, SSO or quota checks fail, **stop rollout and use the previously approved image/source/config with unchanged persistent volume**, ensuring previous image is still available. Do not delete Docker volumes, OAuth/RBAC stores or loopback quota filesystem. Restore only the approved previous environment config and confirm all health and role checks. No generic automatic rollback is safe without verifying the running host's overrides and mounted data.
