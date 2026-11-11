# AnyFlip — Two signed Staging versions and safe rollback boundary

**Status (11 November 2026): prepared, NOT executable.** This document does
not authorize Docker, Host changes, source publishing, Production, or a new
GitHub Approval. Historical evidence is reused; only new trust boundaries
need one proof each.

## What is already proven

- Human-approved, one-time AnyFlip `resume` on the **same** pinned Stage
  image: [RCAT Control Issue #48](https://github.com/yiwssx/rcat-deployment-control/issues/48).
  Stage /healthz=200, unauthenticated API=401, `replayed=false`.
- Existing Host Stage image is locally pinned to source
  `bc72fe9c959ca91c61557b41712ce0ed5b9e1c79`, Docker image ID
  `sha256:9b08f4c2342596e906a0429d09384650b3d9fae540285e8c7e1856912901a399`.
  This is an independently verified signed **binary-derived** local image,
  **not** a signed Docker-save archive.
- The first *real signed image archive candidate* came from manual
  [workflow #42994750274](https://github.com/yiwssx/anyflip-downloader/actions/runs/42994750274)
  (`v0.2.0-rc.115`, source
  `3dca0d19ce18048129d7506bbba7aff98b852c06`).
  The workflow's binary/archive build, GitHub attestation and trusted
  main/source verification all passed. The downloaded ZIP
  `anyflip-candidate-v0.2.0-rc.115` (Actions artifact ID `13843183507`,
  expires 25 November 2026) had reported ZIP SHA-256
  `9e671cf7f21e78d359e90beb1cf6debd0ac6eb7a8370f2f69aafecf32453c36c`.
  Independent downstream archive bytes/OCI tree and source/config consistency
  were previously inspected, with Docker archive SHA-256
  `a552f0ca3d854a9251daa632ad524253e69090276493731dc7281fcd3165f6d7`
  and Docker archive index/image receipt
  `sha256:bc1491a6d0b2416969cad81f83a64f327dcc2bef0aeecb2b35f89d06c10f361a`.
  **Important distinction:** independent `gh attestation verify` of the
  downloaded artifact outside Actions was not completed in that inspection;
  the attestation verification *inside the trusted workflow* succeeded.

## Real blockers: no false two-version claim

- A **second signed image archive from a different, genuine `main`
  source commit** has not yet been generated, downloaded and verified.
  The current source branch for the second candidate is not a signed release.
- The old Host baseline is not retroactively signed as a Docker archive.
  Upgrading from that legacy image to the first signed version is a separate
  **legacy-to-signed transition** risk. Never treat the previous same-image
  `resume` acceptance as a safe cross-version upgrade.
- Candidate ZIPs are short-lived Actions transport artifacts, **not** durable
  rollback storage. Before Host changes, keep independently downloaded and
  verified A/B archives in a root-only, non-Production location with reviewed
  checksums, finite retention and enough capacity.
- The existing v1 Fixed Hook refuses *any* change to the one root-locked image;
  do not change `/etc/rcat-staging/anyflip-approved-image` to bypass it.
  A **separately reviewed** versioned switch/restore hook and request-scoped
  human approval (from + to source/image IDs, direction, backup reference) are
  still required. Stage timer stays disabled.

## Read-only input gate (after two real signed candidates exist)

Store the two *unpacked* image archives and a manually reviewed
`version-pair.json` in the **same private verification directory**. The
manifest has exactly these fields, with independently obtained real values
substituted for all examples:

```json
{
  "schema_version": 1,
  "kind": "rcat.anyflip.stage.signed_version_pair",
  "application": "anyflip-downloader",
  "environment": "staging",
  "from": {
    "filename": "anyflip-web_v0.2.0-rc.115_linux_amd64.docker.tar",
    "source_commit": "3dca0d19ce18048129d7506bbba7aff98b852c06",
    "archive_digest": "sha256:a552f0ca3d854a9251daa632ad524253e69090276493731dc7281fcd3165f6d7",
    "image_id": "sha256:bc1491a6d0b2416969cad81f83a64f327dcc2bef0aeecb2b35f89d06c10f361a"
  },
  "to": {
    "filename": "anyflip-web_v0.2.0-rc.116_linux_amd64.docker.tar",
    "source_commit": "REPLACE_WITH_REAL_SECOND_MAIN_SHA",
    "archive_digest": "REPLACE_WITH_REAL_SIGNED_ARCHIVE_HASH",
    "image_id": "REPLACE_WITH_REAL_DOCKER_IMAGE_ID"
  }
}
```

The second entry intentionally contains **invalid placeholders**; this example
MUST FAIL until real second-version evidence is recorded.

```bash
python3 scripts/verify-stage-version-pair.py /private/path/version-pair.json
```

This checker invokes the existing `verify-attested-staging-archive.py` once
for **each newly distinct** candidate, independently checking exact frozen
bytes, trusted GitHub attestation/main/source, Docker OCI layout and image
identity. It rejects same source/image, missing archives, unknown fields and
Production, emits a canonical `version_pair_sha256` for a future **separate**
Host authorization, and cannot start or modify Docker. A `PASS` here is a
**source input gate only**, never Host/rollback acceptance.

## Minimum remaining operator-safe execution gates

1. **Preflight & identity:** ensure both signed archive sources and image IDs
   remain available, inspected and separately root-pinned; import neither
   archive until the Host owner explicitly authorizes the specific operation.
   Match existing Stage-only project/container/volume/loopback bind; reject
   unfamiliar resources, active jobs, unknown schema/data migration and
   insufficient disk capacity.
2. **Recovery checkpoint:** preserve the dedicated
   `rcat-anyflip-staging-jobs` Docker volume and root-only Staging token,
   without reading Production data or replacing existing secrets. Create a
   private, integrity-checked Stage-volume snapshot **before** any switch;
   verify that snapshot can be restored into an isolated scratch volume.
   Existing **same-version** stop/start/marker drill is accepted, but does
   *not* establish this data compatibility and cross-version restore.
3. **Legacy transition decision:** because the live pinned image predates
   signed Docker archives, review a separately controlled old-image rescue
   strategy before attempting any initial upgrade. Never rely on two future
   archives to claim the currently running legacy image is already protected.
4. **Versioned hook & approval:** review one narrowly scoped Stage-only
   switch/rollback implementation, binding immutable *from/to* source SHA,
   archive SHA and image ID, verified recovery snapshot identity, and one
   fresh private human approval to the exact operation. No arbitrary shell,
   mutable latest tag, implicit automation or expanded GitHub App privileges.
5. **One authorized transaction:** only when gates 1–4 pass may the operator
   execute one approved switch. Confirm health HTTP 200, unauthenticated API
   HTTP 401, the **actual** running image ID, data compatibility and audit.
   On failure, fail closed, stop only affected Stage container, restore the
   pinned previous image and compatible snapshot under reviewed conditions;
   preserve forensic evidence, do not use `docker compose down -v`.
   Cross-version rollback is COMPLETE only after one real approved
   A→B→A recovery result, not an offline test or a healthy unchanged image.

**Non-goals:** Production, cloud networking, DNS, OAuth, POS/Radio/PDF HUB,
new paid service, public package publication, background polling or re-running
accepted Diagnostics/CI/old Staging recovery evidence.

## 11 Nov 2026 — bounded Host recovery and fixed version-switch SOURCE, not installed

The new `deploy/staging/stopped-stage-snapshot.py` prepares one protected
legacy Stage volume snapshot and verifies restoration into a **new, labelled,
empty scratch Docker volume**. It is intentionally an explicit root-only Host
operation, NOT called by `rcat-stage-executor.service`. It requires the known
staging container ALREADY STOPPED, one exact Compose-owned named volume, the
unchanged root-approved Legacy image lock, approved IOT IP, root-owned token,
a trusted local Docker-volume mount path and adequate backup capacity. It
creates a deterministic root-only backup under
`/var/lib/rcat-staging-snapshots`, compares the source/copy/scratch file and
ownership receipt, and removes **only its temporary scratch volume** when
the copy is verified. Original Stage jobs volume and image/container are
never deleted or replaced. A failed or incomplete snapshot is retained for
operator inspection, not silently overwritten. It never stops or starts
Stage, so the operator must obtain a real maintenance window and confirm
no running jobs BEFORE stopping Stage.

**Do NOT run the snapshot yet.** The necessary controlled Staging stop,
backup storage choice and operator authorization must be agreed first.
After an independently reviewed maintenance stop, the exact source-only
invocation would be:

```bash
sudo python3 /path/to/reviewed-anyflip/deploy/staging/stopped-stage-snapshot.py \
  --approve-stopped-stage-snapshot
```

Accept only `STAGING_STOPPED_VOLUME_SNAPSHOT_SCRATCH_RESTORE_PASS`
and a root-only `receipt.json` with `scratch_restored: true`.
This is NEW data recovery evidence, not a repetition of the previous
same-image stop/start proof. Never stop the existing service just to rerun
an earlier acceptance gate.

The new `deploy/staging/anyflip-versioned-fixed-hook.py` is intentionally
**not runnable from CLI** and is NOT installed in
`/usr/local/libexec/rcat-staging/anyflip-downloader-stage`. It freezes the
exact three image/source identities (Legacy local-only image and signed
A/B archive receipts), rejects changed legacy lock and signed version pair,
requires a still-intact legacy backup receipt, verifies image IDs/labels,
loopback-only Stage container and volume mapping and authenticated idle
`GET /api/jobs` with no queued/running jobs. It exposes a single private
`switch_with_authenticated_approval` integration entry point for a future
**separate version-scoped executor**, not for `v1 resume` or the read-only
Agent. It rejects untrusted/manual approval JSON, unknown transitions,
non-distinct images, and missing *already-consumed* one-time GitHub approval
results bound to exact direction + signed image pair + legacy data receipt.
The existing `rcat_ops.approval.audit_live_approval` result does not by
itself contain this versioned operation binding; a separately reviewed
version-scoped adapter and host bootstrap are STILL REQUIRED. Until then
there is NO executable remote or local version switch.

Once the separate adapter and actual signed A/B image imports are accepted,
only fixed `Legacy→A`, `A→B`, `B→A`, `A→Legacy`, or `B→Legacy`
transitions are allowed. On a failed switch the fixed Hook attempts to
recreate only its exact previous Stage image; if fallback also fails it
stops only Stage and retains recovery evidence for a human operator.
Restoring the **image** is not proof of restoring compatible **data**; an
actual approved Staging data restore and A→B→A evidence must still be
observed before HDA-04 can be closed. The original legacy pin remains
untouched and the periodic Stage timer remains disabled throughout.

No Production, Port 8094, DNS/Cloudflare, current Stage secrets, GitHub
Diagnostics Agent, or existing Staging Executor permissions change in
this source-only PR.
