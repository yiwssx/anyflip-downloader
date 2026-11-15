# Upstream reconciliation — 2026-11-15

## Decision and immutable comparison

- Repository: `yiwssx/anyflip-downloader`, branch `main`.
- RCAT main at review: `399e731b9551f8ecca34317f81ff72884b9d3f6d`.
- Upstream: `Lofter1/anyflip-downloader`, main `ce37d170fc6bdbc3cd8a3f7f65cd458bee3b3a52`.
- Merge base: `509b70d8f31c207177daaeb574b2fcf5e1035dce`.
- GitHub compare at review: fork **190 ahead / 12 behind**, diverged. The 12 upstream commits together touch nine files: `.github/workflows/build.yml`, `Dockerfile`, `README.md`, `anyflip.go`, `anyflip_test.go`, `go.mod`, `go.sum`, `main.go`, and `webp.go`.
- Scope authorized: selective compatibility review and focused PR, without changing the production host, authorization, secrets, deployment, or unrelated CI.

This is a **selective reconciliation, not a Git-history sync**. Do not merge `upstream/main` wholesale, rebase RCAT `main`, force-push, or use `git merge -s ours` to hide 12 missing ancestors. GitHub can continue to show 12 behind even when applicable upstream behaviors are already present.

## Upstream change disposition

| Upstream changes | Local implementation/evidence | Decision |
| --- | --- | --- |
| PR #94: invalid URL paths and missing CLI arguments (`0c4d002`, `0c6b446`) | `validateBookURL` in `validation.go` rejects malformed paths, off-host URLs, traversal and ambiguous escapes; `validation_test.go` covers accepted/rejected inputs. RCAT CLI already reports missing URLs with a nonzero exit rather than the upstream's successful return. | Keep RCAT's stricter behavior; no cherry-pick. |
| PR #81: `-pages-offset` and interrupted downloads (`114b353`, `b1130c1`) | `-resume` is implemented with verified private resume manifest, validated images and interruption checks in `resume.go`, `resume_test.go`, `image_download.go`. | Do not reintroduce unverified page skipping, which could reuse unrelated/stale page files. |
| PR #126: washed-out lossy WebP / BT.601 color range (`c6a9e9f`, `8e0ac47`) | `createPDFVerifiedWithBudget` invokes `preparePDFPageWebP` in isolated staging. `pdf_image_formats_test.go` already checks lossless WebP plus JPEG/PNG PDF generation and unchanged source images. | Add a **single lossy white WebP regression test** verifying color range, staged PNG and source immutability. Do not transplant upstream's separate `webp.go` path. |
| PR #125: pdfcpu 0.16.3, Go 1.26, import API context (`f5da874`, `ed48d5d`, `ce37d17`) | `go.mod` already pins `pdfcpu v0.16.3` and Go `1.26.11`; `main.go` and `pdf_output.go` pass a context to PDF import. | Already covered; no dependency churn. |
| PR #128: Docker Go builder compatibility (`dbbb735`, `e4e9008`) | RCAT `Dockerfile` uses `golang:1.26.11-alpine`, a separate non-root runtime, and certificates. Workflows read the version from `go.mod`. | Preserve RCAT's hardened images and CI; no Docker rewrite. |
| README and build-workflow edits | RCAT has separate Web GUI, auth/RBAC, release and Host runbooks. | Do not replace local documentation or workflows with upstream CLI-only examples. |

## Delivered, verification, and stop condition

- Changed surface: test-only `pdf_webp_upstream_regression_test.go` plus this decision record.
- New focused regression: synthetic 16×16 **lossy** WebP white page, color channels at least 250 after private PNG staging, expected dimensions, and byte-for-byte original image preservation. No external book downloads or new production permissions.
- Gate: inspect exact-head PR checks. For Go changes the `AGENTS.md` delivery contract remains applicable: gofmt, `go build ./...`, `go vet ./...`, `go test -count=1 ./...`, and `go test -race ./...` through CI. Reuse prior unrelated browser, Docker, RBAC, quota and host evidence; no new acceptance campaign.
- Only merge after exact-head CI succeeds and the change is reviewed; if CI fails, correct only the specific regression and recheck. Creating a PR is not completion.
- No Production deploy, restart, OAuth change, Docker/volume action, DNS change, role change, or local host checkout modification is part of this task.
- The persisted fork behind count represents **ancestry**, not a definitive unresolved functional defect. Reassess future upstream commits against the same trust boundaries.

Reviewed against the live `AGENTS.md` and `docs/DEVELOPMENT_TRACKER.md`; completed operator acceptance is not reopened.
