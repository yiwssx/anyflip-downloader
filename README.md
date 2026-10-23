# anyflip-downloader

[![build](https://github.com/Lofter1/anyflip-downloader/actions/workflows/build.yml/badge.svg)](https://github.com/Lofter1/anyflip-downloader/actions/workflows/build.yml)
[![goreleaser](https://github.com/Lofter1/anyflip-downloader/actions/workflows/release.yml/badge.svg)](https://github.com/Lofter1/anyflip-downloader/actions/workflows/release.yml)

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
jobs retain temporary images on failure for diagnostics. This code does not
yet provide a Web GUI, authentication, quotas, queue persistence or a hosted
deployment. Only download content you are authorized to reproduce.

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
