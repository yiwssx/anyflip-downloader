# P1 — Image fidelity and PDF acceptance matrix

**Status (2026-10-28): CI binary regressions are merged; target-host book fidelity is NOT accepted.**
Related PRs: [#19](https://github.com/yiwssx/anyflip-downloader/pull/19),
[#22](https://github.com/yiwssx/anyflip-downloader/pull/22),
[#23](https://github.com/yiwssx/anyflip-downloader/pull/23).
Master tracker: [DEVELOPMENT_TRACKER.md](DEVELOPMENT_TRACKER.md).

This project creates **image-based PDFs**, not the original AnyFlip PDF with
editable text/vector layers. Only perform live checks against books for which
the operator has permission to download/reproduce pages. Do not bypass DRM,
access controls, authentication or publisher restrictions.

## Verification matrix

| Gate | Expected evidence | Current status |
| --- | --- | --- |
| P1-A1 JPEG, PNG, WebP | Real file bytes imported via pdfcpu, 3-page PDF validates, no damaged output | CI passed #19 |
| P1-A2 Corrupt/missing page | Broken data fails closed; no incomplete final PDF | CI passed #19/#23 |
| P1-C1 Declared alternate | Metadata advertises variant; 404/410 primary falls back to explicitly declared same-book URL | CI passed #22 |
| P1-C2 Access control | 401/403, throttling, network errors and unsafe redirect never select a lower variant | CI passed #22 |
| P1-C3 Highest available resolution | Opt-in mode downloads only up to 8 publisher-listed candidates per page; selects greatest decoded width × height (ties keep metadata order) | **CI passed #27; actual authorized-book verification OPEN** |
| P1-C4 Web opt-in job | Thai checkbox defaults off; authenticated request selects publisher-listed high-pixel images and creates a valid PDF in mock-origin Web API integration | **CI passed #28; physical browsers/host OPEN** |
| P1-D1 Mixed geometry | Real PDF MediaBox ratios correspond to 4 ordered portrait/landscape/square PNG pages, including across chunk boundaries | CI passed #23 |
| P1-D2 EXIF rotation | EXIF 1–8, little/big-endian TIFF, actual PDF geometry and input immutability | **CI passed #25; physical viewer QA OPEN** |
| P1-D3 Visual fidelity | Compare legibility, margins, page color, crop, and embedded-image clarity on desktop and mobile | **OPEN** |
| P1-E1 Complete authorized book | One entire publisher-authorized book, correct source page count, valid PDF, first/middle/last page opened | **OPEN** |
| P1-E2 Target host | On actual Ubuntu/Docker, check 20-minute job timeout, disk/volume usage, restart cleanup, safe errors | **OPEN** |
| P1-E3 Release | Operator records test evidence, reviews security/HTTPS requirements, approves release separately | **OPEN** |

**Important:** Metadata choice order is preserved. The first advertised image
is not proven to be the highest pixel-resolution variant. A 404/410-only
fallback is a resilience feature, not a quality guarantee. Missing permission
is never grounds for fallback.

## Explicit high-resolution selection

- Default mode keeps publisher metadata order and uses alternate image URLs only after a 404/410 (PR #22).
- Optional `-best-quality` (CLI) or the unchecked **เลือกภาพความละเอียดสูงสุด** checkbox (Web GUI) downloads and checks every publisher-declared image candidate (at most eight) for each page, ranks successful images by decoded pixel count `width × height`, and keeps the first declared candidate on a tie. It does **not** generate, guess, scrape or probe unlisted image URLs.
- A declared 404/410 image may be skipped. An authentication denial, rate limit, redirect outside the book, network failure or corrupt image fails the quality job without publishing a partial result. This is **not** an access-control bypass.
- This opt-in can increase publisher requests, network transfer, storage I/O, and job duration by up to the number of publisher-listed variants per page. It is not automatically enabled for existing LAN clients. Larger pixel count does not always guarantee better legibility or less compression, and does not establish access to an original PDF.
- Compare actual output, not source names (`large`, `mobile`) or HTTP Content-Length; do not close the P1 live quality gate without authorized real-book and viewer evidence.

## Authorized acceptance walkthrough

1. Identify a book whose publisher allows PDF download or reproduction.
   Privately record the book URL, permission basis, expected page count,
   actual publisher metadata choices, and whether those variants are authorized.
   Do not commit a copy of the pages, token, LAN address, screenshots with
   credentials, or private host logs.
2. Use the existing LAN-only GUI from a controlled network, preferably through
   an SSH tunnel or private HTTPS endpoint. **Plain HTTP exposes Bearer tokens
   to anyone able to inspect that traffic**, even on a LAN. Do not change
   reverse proxy or host-port mappings for this acceptance test.
3. Submit one authorized book; record redacted Job ID, start/end time,
   completed/expected page totals, whether the fallback was used, and the
   completed PDF's size and SHA-256. Test cancellation separately with another
   operator-approved fixture.
4. Download the PDF to a trusted client and use locally installed tools:

   ```sh
   file authorized-book.pdf
   sha256sum authorized-book.pdf
   pdfinfo authorized-book.pdf
   pdfimages -list authorized-book.pdf
   ```

   `pdfinfo`/`pdfimages` are optional Poppler tools; install them on the
   **client** if needed, not as an unreviewed production-host dependency.
   Verify **Pages** exactly matches publisher-authorized metadata. Inspect
   MediaBox/aspect information from output and compare a portrait, landscape,
   and any rotated source against the rendered pages.
5. Open first, middle and last page in two independent viewers. Check page
   order, absence of blanks/crops, readability of Thai type and fine diagrams,
   consistent orientation, and no unexplained quality drop. Record redacted
   results, not copyrighted page screenshots in the public repository.
6. Confirm the original authorized source had no accessible higher resolution
   than the chosen candidate. Compare pixel dimensions, not just filenames
   such as `large`/`mobile` or content length. Do not probe unlisted URLs.
7. For a failed job, record the safe GUI `errorCode`, not secrets or raw
   server paths. Only a publisher-listed page returning 404/410 may trigger
   alternate selection; 401/403/429 must never become a bypass.
8. Record environment, Go/pdfcpu image versions, PDF page-count evidence,
   duration and resource observations, regression results and operator decision
   in an internal test record. Update the master tracker only when each
   corresponding gate has evidence.

## Release guardrails

**JPEG orientation implementation limit:** For EXIF orientations 2–8, JPEGs are decoded and transformed into lossless PNG files inside a private PDF staging directory; source JPEG bytes remain unchanged. Rotated images above 25 million pixels return an error instead of risking excessive memory use. The CI test is not proof that real books appear correct in physical PDF viewers.

- Keep the existing operator-customized LAN binding and Docker named volume.
  Do not overwrite the local Compose with the loopback-only repo example.
- Do not activate public Apache/DNS/HTTPS, extra users, OCR or arbitrary
  outbound hosts as a consequence of passing P1 checks.
- The CI runner's mock-origin PDF fixture is deliberately **not** evidence of
  an end-to-end download of a real authorized AnyFlip publication.
- Keep P3 public-access, authorization, quotas, retention and physical-browser
  gates OPEN until separately reviewed and tested.
