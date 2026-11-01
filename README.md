# anyflip-downloader

[![build](https://github.com/yiwssx/anyflip-downloader/actions/workflows/build.yml/badge.svg)](https://github.com/yiwssx/anyflip-downloader/actions/workflows/build.yml)

Download anyflip books as PDF

![Demo](/assets/demo.gif)

## Disclaimer

Only use this tool to download books that officially allow PDFs to be downloaded.

## Installation
You can install this tool in multiple ways. Using the installation script or the go install command.

The install scripts are the suggested installation method for most users. 

### Install scripts

#### Linux/MacOS
Open the terminal and execute
```sh
curl -L https://raw.githubusercontent.com/Lofter1/anyflip-downloader/main/scripts/install.sh | /usr/bin/env bash
```
##### Setup path
When encountering the error "Command not found": make sure your path variable contains `$HOME/.local/bin` (edit this in your .zshrc or .bashrc depending on your editor)

#### Windows
Open PowerShell and execute
```PowerShell
. { iwr -useb https://raw.githubusercontent.com/Lofter1/anyflip-downloader/main/scripts/install.ps1 } | iex;
```

### Go install
For `go install`, the [go tools](https://go.dev/doc/install) are required.

```sh
go install github.com/Lofter1/anyflip-downloader@latest
```

## Usage

```sh
anyflip-downloader <url to book>
```

### Optional: best available publisher-listed image resolution

Default: use the first image listed in publisher metadata, with explicit 404/410 fallback. For books you have permission to copy, add `-best-quality` to compare **actual decoded width × height** for publisher-listed variants (up to 8 per page), select the largest, and keep the first-listed image on a tie:

```sh
anyflip-downloader -best-quality https://anyflip.com/publisher/book
```

The Web GUI also offers an **unchecked** quality-selection checkbox. This mode can download multiple images per page, increasing network use and time. It never guesses unlisted URLs or bypasses 401/403 access controls, rate limiting or third-party restrictions; any restricted, malformed or network-failed candidate causes a fail-closed job. Largest pixel dimensions are not proof of best perceptual quality or of access to the publisher's original PDF. Verify the result with an authorized book before accepting quality claims. [P1 verification matrix](docs/P1_VERIFICATION.md).

### Set title manually

If you do not want to use the book title from anyflip, you can change it using the `-title` flag.

```sh
anyflip-downloader -title <your book title> <url to book>
```

### Specify temporary download folder path

The default temporary download folder path will be the title of the book. However, in certain situations, you might want to change the temporary download folder. For this, the `-temp-download-folder` flag exists. This folder will be deleted after a successful download.

```sh
anyflip-downloader -temp-download-folder <temp folder name> <url to book>
```

### Define converting chunk size

By default, anyflip downloader will convert 10 images at a time. You can tell anyflip to convert more or less images at a time.

A lower number will result in less memory usage, but more writes to the drive, and therefore might increase time to convert.
A higher number will result in more memory usage, but less writes and might increase converting speed. If the number is higher than the total amount of pages, the amount of pages currently being converted is automatically taken instead.

```sh
anyflip-downloader -chunksize <chunkzise> <url to book>
```

### Advanced file download options

#### Parallel retrieval
By default, downloads are performed in a single thread. To improve performance, multiple pages can be downloaded simultaneously. Use the `-threads` flag to control the number of parallel jobs.
```sh
anyflip-downloader -threads <number of parallel jobs> <url to book>
```

#### Download retries
Occasionally, a request may fail due to temporary issues such as timeouts. Use the `-retries` flag to specify how many times a failed page should be retried before giving up:
```sh
anyflip-downloader -retires <number of attempts> <url to book>
```

#### Retry delay
To avoid overwhelming the server or triggering rate limits, you can introduce a delay between retry attempts. The `-waitretry` flag accepts any valid Go duration format (e.g., 500ms, 2s, 1m).
```sh
anyflip-downloader -waitretry <duration> <url to book>
```


### Docker usage:

If you are familiar with docker you can always execute


```sh
docker build -t anyflip-downloader .
```

And run it by doing

```sh
docker run --rm -v "$(pwd)":/data anyflip-downloader <url to book>
```

You can also combine this comand with any of the above for example:

```sh
docker run --rm -v "$(pwd)":/data anyflip-downloader -title <your book title> <url to book>
```

### Resume an interrupted CLI download (opt-in)

For downloads started with this version, use a dedicated directory and the
same book URL to resume after an interruption. An authenticated local manifest
binds the folder to the publisher's page map and quality setting; completed
page images are published atomically. Files from older versions without this
manifest cannot be resumed automatically.

```sh
anyflip-downloader -temp-download-folder ./book-pages -keep-download-folder https://anyflip.com/publisher/book
# If interrupted, run with the SAME folder and SAME quality option:
anyflip-downloader -resume -temp-download-folder ./book-pages https://anyflip.com/publisher/book
```

The first invocation needs a folder that does not yet exist. Resume accepts
only a previously created private folder. Existing valid pages are skipped,
and missing pages are downloaded. A changed book, an untrusted/corrupt page,
or mismatched quality settings cause an explicit error without overwriting
existing data. Resumed folders are preserved even after successful conversion;
remove them yourself when you no longer need the images. The PDF output path
must not already exist. This opt-in feature is **CLI-only**: browser jobs still
use isolated new workspaces, job Retry, and the existing storage quota.

### Safe temporary workspaces and parameter limits

Each run creates a new temporary directory (under the system temporary directory by default).
If you set `-temp-download-folder`, the specified directory **must not already exist**;
this prevents deleting files that were not created by the downloader. The workspace is
removed after successful conversion unless `-keep-download-folder` is set; if a download
or conversion fails, the image files are retained for troubleshooting.

If the output PDF already exists, the downloader exits with an error instead of
treating the operation as a success or overwriting an existing file.

For predictable resource usage, `-threads` accepts 1–16, `-chunksize`
accepts 1–1000, `-retries` accepts 0–10, and `-waitretry` accepts
durations from 0 to 1 minute. This tool creates an **image-based PDF**,
not the original text/vector PDF; use it only for content you are authorized to download.

### Reusable job engine (P0 Core)

The Go code now provides `RunDownloadJob(ctx, DownloadJobOptions, onProgress)`,
which is separate from CLI global flags and can be invoked by future job-based
web services. Supported progress phases: `preparing`, `downloading`,
`converting`, `completed`, `failed` and `cancelled`. Download updates
include a monotonic completed-page count. Every job can be cancelled using
`context.Context` (the PDF importer checks cancellation between chunks);
the CLI also responds to Ctrl+C/SIGTERM.

Example from another Go file **within the same module**:

```go
ctx, cancel := context.WithCancel(context.Background())
defer cancel()
output, err := RunDownloadJob(ctx, DownloadJobOptions{
    URL: "https://anyflip.com/publisher/book",
    Threads: 2,
    Retries: 1,
    RetryDelay: time.Second,
    ChunkSize: 10,
    OutputFile: "/a/server-controlled/directory/book.pdf",
}, func(event DownloadProgress) {
    log.Printf("%s %d/%d", event.Stage, event.Completed, event.Total)
})
_ = output
_ = err
```

The web backend must **never** accept arbitrary output/workspace paths from
unauthenticated clients; use per-user/per-job, server-owned directories. Do
not make an unauthenticated endpoint public. Restrict outbound network access
and verify DNS/private-network protections before hosted deployment. Current
jobs retain temporary images on failure for diagnostics. The core engine alone does not provide authentication or multi-user job
ownership. This fork includes a separate Web GUI with opt-in trusted-network browser
sessions, a server-side administrator API token, a bounded in-memory queue
and limited metadata history, but no approved public production deployment. Only download content
you are authorized to reproduce.

### Outbound network safety

By default, downloader HTTP requests use TLS certificate verification, an
HTTPS-only AnyFlip origin, bounded timeouts and redirect checks. The resolver
rejects private, loopback, link-local, documentation, carrier-grade NAT and
other reserved destination IP addresses before connecting, and outbound
proxies are disabled in the built-in HTTP client. Retries use exponential
backoff capped at 60 seconds.

The legacy `-insecure` CLI option is now explicitly **rejected** rather than
allowing certificate verification to be disabled. This is an intentional
security change for server readiness. A future hosted service must still
enforce user authentication, permission checks, resource limits and job
ownership; a secure HTTP client alone does not make a public endpoint safe.

## Self-hosted Web GUI (MVP)

The same Go binary includes a browser interface with optional automatic sessions on trusted networks. A LAN-only
self-hosted deployment was operator-confirmed working on 2026-10-28 after
[pdfcpu's writable-config fix (PR #18)](https://github.com/yiwssx/anyflip-downloader/pull/18).
This does **not** verify an entire authorized book, mobile/desktop acceptance,
or security readiness for public access. A public deployment should use a
trusted HTTPS reverse proxy (such as Apache) only after the remaining gates.

1. Set a long, unique `ANYFLIP_WEB_TOKEN` value (at least 16 characters).
2. Run `ANYFLIP_WEB_TOKEN=your-long-random-token ANYFLIP_BROWSER_ACCESS=trusted-network ./anyflip-downloader -web`
   only on an operator-restricted LAN or loopback (or use the included Compose example).
3. Open `http://127.0.0.1:8080` and paste the URL of a book you are authorized to
   download. No Access Token is entered: the server sets an HttpOnly browser cookie.
4. Monitor preparation, page download and PDF conversion; cancel or retrieve
   the **image-based PDF** once complete.

By default, the web listener is **127.0.0.1:8080** (not publicly reachable).
The `-listen` flag changes that address. Do not expose a public listener
without strict firewall/VPN/reverse-proxy access controls and
server-side monitoring. Programmatic API calls can still use `Authorization: Bearer <token>`; trusted-network browsers use a signed 12-hour HttpOnly, SameSite=Strict cookie.
The administrator token is never placed in URLs or browser JavaScript. Completed job files expire after one hour
while the server is running; set `ANYFLIP_JOB_DIR` for the job storage folder.

HTTP API: `POST /api/jobs` with JSON `{"url":"...","title":"...","threads":3}`
creates a job; `GET /api/jobs/{id}` retrieves progress;
`POST /api/jobs/{id}/cancel` requests cancellation; and
`GET /api/jobs/{id}/download` returns the verified PDF. The health endpoint
is `GET /healthz`. Direct API access remains token-protected unless the client
has a valid trusted-network browser session. The shared in-memory queue is limited
to 12 pending jobs / 2 active downloads. It does **not**
yet provide separate per-user accounts, durable queue persistence, OCR, or
production deployment. See `docs/DEVELOPMENT_TRACKER.md` for remaining work.

### Docker Compose + Apache production deployment recipe

See [Ubuntu deployment runbook](docs/WEB_DEPLOYMENT.md) for a non-root Docker image,
Compose with a **127.0.0.1:8094-only host port**, HTTPS Apache reverse proxy,
strong token creation, health checks, volume persistence, rollbacks and security
limitations. The included `deploy/apache/anyflip-web.conf.example` is a **template**:
it does not provision a domain, DNS or TLS certificate.

Start with `docker compose -f compose.web.yaml up -d --build` after securely
creating `.env`. The repository's stock Compose binds loopback; an existing
operator-customized LAN bind must not be overwritten by copying the template
blindly. In trusted-network mode, any client able to reach the Web GUI can
receive a browser session and operate the shared queue; this is **not user login**.
Restrict access at the network perimeter and use HTTPS when deployed behind a
reverse proxy. The administrator token is held only by the server and API clients.
Do not open a public endpoint without completing the production verification
gates documented in the runbook.
