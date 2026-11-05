---
name: anyflip-go-pdf
description: Use for Go downloader core, AnyFlip config or image URL parsing, JPEG PNG WebP image quality, pdfcpu conversion, missing pages, retries, cancellation and PDF integrity fixes.
---

# anyflip-go-pdf

## Workflow
1. Read the specific entry points: anyflip.go, configjs.go, validation.go, httpclient.go, network_policy.go, image_download.go, image_quality.go, job.go, pdf_output.go and relevant tests. Do not infer behavior from the old upstream README.
2. Reproduce with synthetic, in-memory/httptest or operator-authorized book fixtures. Do not scrape unlisted assets or bypass authentication, DRM, 401/403, throttling or publisher restrictions.
3. Preserve exact page count/order, same-book path containment, 404/410-only fallback where declared, bounds on images and page count, context cancellation, and all-or-nothing PDF publication.
4. Distinguish default image choice from opt-in highest decoded pixel count; larger pixel dimensions do not guarantee text fidelity. Preserve original source files and EXIF orientation handling.
5. Treat pdfcpu initialization and writable configuration paths as a production concern. The container root is read-only; approved writable paths live on the job volume.
6. Add regression cases for the failure, including corrupted or missing late pages, mixed format/geometry and concurrent cancellation where relevant.

## Validation
- gofmt -l on touched Go files; go build ./...; go vet ./...
- go test -count=1 ./... and go test -race ./...
- Consult docs/P1_VERIFICATION.md for the separate viewer/real-book acceptance matrix. CI fixtures do not prove visual fidelity on a real device.
