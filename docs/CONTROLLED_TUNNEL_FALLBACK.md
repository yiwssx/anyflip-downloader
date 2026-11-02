# Isolated AnyFlip recovery ingress — RCAT controlled domain

Incident: [new-www #786](https://github.com/yiwssx/new-www.rcat.ac.th/issues/786).

## What was provisioned on Cloudflare (2026-11-02)

- Controlled zone: `rcat.dpdns.org`. No changes to `rcat.ac.th` or its DNS account.
- New remotely-managed Cloudflare Tunnel: `rcat-anyflip-direct-ingress`, UUID `3f3c41dd-8c67-4cbf-a743-3881eba895cf`.
- New independent proxied DNS CNAME: `anyflip.rcat.dpdns.org` -> `3f3c41dd-8c67-4cbf-a743-3881eba895cf.cfargotunnel.com`.
- Exact ingress: `anyflip.rcat.dpdns.org` -> `http://127.0.0.1:8094`; catch-all -> HTTP 404.
- The tunnel is **INACTIVE** until a connector runs on `10.10.1.100`. DNS-only publication does not make the application reachable.
- The original URL `anyflip.rcat.ac.th` is unaffected and **will not automatically be repaired** by this alternative entry point.
- The separate `www.rcat.ac.th` SaaS/O2O Gateway, Vercel routing, institutional Reverse Proxy, and RMS/PDF/IoT/Radio are unchanged.

## Safe connection procedure on the existing Ubuntu host (10.10.1.100)

This operation runs a **new, independent** Docker Compose project; it does not rebuild or restart the AnyFlip app or alter its storage/quota services.

1. Update the checked-out source to a released commit after the CI passes:

   ```bash
   cd /opt/anyflip-downloader
   git pull --ff-only
   ```

2. In your connected Cloudflare account, open **Networking > Tunnels > rcat-anyflip-direct-ingress**. Copy the tunnel token from the connector install step. **Never paste the token into GitHub, ChatGPT, shell history or application logs.** The startup script prompts for it invisibly and writes to `~/.config/rcat-anyflip/tunnel.env` with file mode 0600. The file is stored outside this repository.

3. Run one command on the Ubuntu server:

   ```bash
   bash scripts/start-tunnel-ingress.sh
   ```

   The script first verifies `http://127.0.0.1:8094/healthz`, validates Docker access, then launches only the `anyflip-tunnel` service from `compose.tunnel.yaml` under Docker Compose project `rcat-anyflip-ingress`. Host networking is deliberate: the existing Docker Web GUI publishes only `127.0.0.1:8094` on the host. Cloudflared connects from localhost without opening an inbound firewall port or changing Apache.

4. Verify external HTTPS with:

   ```bash
   bash scripts/verify-tunnel-ingress.sh
   ```

   Acceptance is a real HTTPS 200 at `https://anyflip.rcat.dpdns.org/healthz`; an inactive connector can return a Cloudflare 1016 or 1033 error. Then test an authenticated browser session/download, independently of the health endpoint.

## Authentication and cutover safety

- A public tunnel bypasses the institution's existing Apache reverse proxy; **do not assume it inherits Apache's access controls**. The AnyFlip app must enforce its own authentication before production use.
- The existing `ANYFLIP_AUTH_MODE` defaults to `token` until separately configured. The remote tunnel cannot change this application environment. Do not disable authentication to make the fallback pass.
- If Google Workspace login is already enabled, Google OAuth requires an *exact* authorized callback URL matching the alternative hostname: `https://anyflip.rcat.dpdns.org/auth/google/callback`. The OAuth client and server-side `GOOGLE_REDIRECT_URI` must be updated in a deliberate auth cutover; existing sessions may be invalidated on app restart. See `docs/AUTHENTICATION.md`. The alternate public health endpoint returning 200 does **not** prove interactive sign-in works.
- Preserve volume `/data/jobs`, quota loopback mount and `anyflip-quota.service`; do not run a second Web GUI or change `compose.web.yaml`.

## Rollback and cleanup

To stop only the new tunnel (not AnyFlip, its data, RMS or `www`):

```bash
cd /opt/anyflip-downloader
docker compose --project-name rcat-anyflip-ingress --env-file "$HOME/.config/rcat-anyflip/tunnel.env" -f compose.tunnel.yaml stop anyflip-tunnel
```

The original production routes remain untouched. After full acceptance, decide separately whether the new URL is an acceptable published entry point or whether the original `anyflip.rcat.ac.th` must be restored through another authorized path. **Do not close Incident #786 merely because the tunnel exists.**

Cloudflare official documentation: [Tunnel getting started](https://developers.cloudflare.com/tunnel/get-started/), [Tunnel routing and CNAME](https://developers.cloudflare.com/tunnel/concepts/routing/).
