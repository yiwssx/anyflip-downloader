package main

import (
    "context"
    "errors"
    "net/http"
    "net/url"
    "os"
    "path/filepath"
    "strings"
    "sync/atomic"
    "testing"
    "time"

    "github.com/pdfcpu/pdfcpu/pkg/api"
)

func TestRunDownloadJobResumeInterruptedCLI(t *testing.T) {
    folder := filepath.Join(t.TempDir(), "interrupted")
    output := filepath.Join(t.TempDir(), "resumed.pdf")
    baseClient := jobFixtureClient(t)
    first := &http.Client{
        Timeout: time.Second,
        Transport: mockRoundTrip(func(req *http.Request) (*http.Response, error) {
            if strings.HasSuffix(req.URL.Path, "/2.png") {
                return responseBytes(req, http.StatusNotFound, []byte("not found")), nil
            }
            return baseClient.Transport.RoundTrip(req)
        }),
    }
    opts := DownloadJobOptions{
        URL: "https://anyflip.com/qrfai/tfvn",
        TempDownloadFolder: folder,
        OutputFile: output,
        Threads: 1,
        Retries: 0,
        ChunkSize: 1,
        HTTPClient: first,
    }
    if _, err := RunDownloadJob(context.Background(), opts, nil); err == nil {
        t.Fatal("expected simulated interruption on second page")
    }
    if _, err := os.Stat(filepath.Join(folder, "0000.png")); err != nil {
        t.Fatalf("completed first page missing: %v", err)
    }
    if _, err := os.Stat(filepath.Join(folder, resumeManifestName)); err != nil {
        t.Fatalf("resume manifest missing: %v", err)
    }

    var pageOneRequests, pageTwoRequests atomic.Int32
    second := &http.Client{
        Timeout: time.Second,
        Transport: mockRoundTrip(func(req *http.Request) (*http.Response, error) {
            if strings.HasSuffix(req.URL.Path, "/1.png") { pageOneRequests.Add(1) }
            if strings.HasSuffix(req.URL.Path, "/2.png") { pageTwoRequests.Add(1) }
            return baseClient.Transport.RoundTrip(req)
        }),
    }
    opts.Resume = true
    opts.HTTPClient = second
    var completed int
    got, err := RunDownloadJob(context.Background(), opts, func(event DownloadProgress) {
        if event.Stage == "downloading" && event.Completed > completed {
            completed = event.Completed
        }
    })
    if err != nil { t.Fatalf("resume failed: %v", err) }
    if got != output { t.Fatalf("output=%q want %q", got, output) }
    if pageOneRequests.Load() != 0 || pageTwoRequests.Load() != 1 {
        t.Fatalf("resume unexpectedly re-fetched completed page: p1=%d p2=%d", pageOneRequests.Load(), pageTwoRequests.Load())
    }
    if completed != 2 { t.Fatalf("resume progress=%d expected 2 completed pages", completed) }
    if n, err := api.PageCountFile(context.Background(), output); err != nil || n != 2 {
        t.Fatalf("resumed PDF count=%d err=%v", n, err)
    }
    if _, err := os.Stat(folder); err != nil { t.Fatalf("resume must preserve workspace: %v", err) }
}

func TestResumeRejectsUntrustedOrMismatchedWorkspace(t *testing.T) {
    parent := t.TempDir()
    dir := filepath.Join(parent, "other-data")
    if err := os.Mkdir(dir, 0700); err != nil { t.Fatal(err) }
    important := filepath.Join(dir, "important.txt")
    if err := os.WriteFile(important, []byte("untouched"), 0600); err != nil { t.Fatal(err) }

    _, err := RunDownloadJob(context.Background(), DownloadJobOptions{
        URL: "https://anyflip.com/qrfai/tfvn",
        TempDownloadFolder: dir,
        OutputFile: filepath.Join(parent, "output.pdf"),
        Threads: 1, Retries: 0, ChunkSize: 1,
        Resume: true, HTTPClient: jobFixtureClient(t),
    }, nil)
    if err == nil || !strings.Contains(err.Error(), "manifest") {
        t.Fatalf("expected rejection of untrusted existing folder, got %v", err)
    }
    data, err := os.ReadFile(important)
    if err != nil || string(data) != "untouched" { t.Fatal("untrusted directory was modified") }
}

func TestResumeRejectsCorruptCachedImageAndDoesNotOverwrite(t *testing.T) {
    base := jobFixtureClient(t)
    book, err := prepareDownloadContext(context.Background(), mustResumeURL(t), "", base)
    if err != nil { t.Fatal(err) }
    dir, cleanup, err := createDownloadWorkspace(filepath.Join(t.TempDir(), "cache"))
    if err != nil { t.Fatal(err) }
    defer cleanup()
    if err := writeResumeManifest(dir, book, false); err != nil { t.Fatal(err) }
    cached := filepath.Join(dir, "0000.png")
    contents := []byte("incomplete download")
    if err := os.WriteFile(cached, contents, 0600); err != nil { t.Fatal(err) }
    complete, err := checkExistingResumePage(book, dir, 0)
    if err == nil || complete { t.Fatal("corrupt cached image was accepted") }
    original, err := os.ReadFile(cached)
    if err != nil || string(original) != string(contents) { t.Fatal("corrupt cached file was modified") }

    if err := verifyResumeManifest(dir, book, false); err != nil { t.Fatal(err) }
    if err := verifyResumeManifest(dir, book, true); err == nil { t.Fatal("different quality mode accepted") }
}

func TestResumeRejectsCancelledContext(t *testing.T) {
    ctx, cancel := context.WithCancel(context.Background())
    cancel()
    _, err := RunDownloadJob(ctx, DownloadJobOptions{
        Resume: true, URL: "https://anyflip.com/qrfai/tfvn",
        TempDownloadFolder: "/unused",
        Threads: 1, ChunkSize: 1,
    }, nil)
    if !errors.Is(err, context.Canceled) { t.Fatalf("cancelled resume error = %v", err) }
}

func mustResumeURL(t *testing.T) *url.URL {
    t.Helper()
    u, err := url.Parse("https://anyflip.com/qrfai/tfvn")
    if err != nil { t.Fatal(err) }
    return u
}
