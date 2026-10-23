package main

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pdfcpu/pdfcpu/pkg/api"
)

func jobFixtureClient(t *testing.T) *http.Client {
	t.Helper()
	pngData := imageFixture(t)
	const config = `{"title":"Fixture Book","totalPageCount":2,"pages":[{"n":["1.png"]},{"n":["2.png"]}]}`
	return &http.Client{
		Timeout: time.Second,
		Transport: mockRoundTrip(func(req *http.Request) (*http.Response, error) {
			switch {
			case strings.HasSuffix(req.URL.Path, "/config.js"):
				return mockResponse(req, http.StatusOK, config), nil
			case strings.HasSuffix(req.URL.Path, "/1.png"), strings.HasSuffix(req.URL.Path, "/2.png"):
				return responseBytes(req, http.StatusOK, pngData), nil
			default:
				return mockResponse(req, http.StatusNotFound, "not found"), nil
			}
		}),
	}
}

func TestRunDownloadJobFullPDFAndProgress(t *testing.T) {
	parent := t.TempDir()
	workspace := filepath.Join(parent, "download-work")
	output := filepath.Join(parent, "downloaded.pdf")
	var mu sync.Mutex
	var events []DownloadProgress
	result, err := RunDownloadJob(context.Background(), DownloadJobOptions{
		URL:                "https://anyflip.com/qrfai/tfvn",
		Title:              "Test",
		OutputFile:         output,
		TempDownloadFolder: workspace,
		Threads:            2,
		Retries:            0,
		ChunkSize:          1,
		HTTPClient:         jobFixtureClient(t),
	}, func(p DownloadProgress) {
		mu.Lock()
		events = append(events, p)
		mu.Unlock()
	})
	if err != nil {
		t.Fatalf("download job failed: %v", err)
	}
	if result != output {
		t.Fatalf("output file = %q; expected %q", result, output)
	}
	if pageCount, err := api.PageCountFile(output); err != nil || pageCount != 2 {
		t.Fatalf("PDF page count = %d, error %v", pageCount, err)
	}
	if _, err := os.Stat(workspace); !os.IsNotExist(err) {
		t.Fatalf("successful job did not remove image workspace: %v", err)
	}
	if len(events) == 0 || events[0].Stage != "preparing" || events[len(events)-1].Stage != "completed" {
		t.Fatalf("missing job start/completion events: %+v", events)
	}
	seen := map[string]bool{}
	lastDownload := 0
	for _, p := range events {
		seen[p.Stage] = true
		if p.Stage == "downloading" && p.Completed > 0 {
			if p.Completed < lastDownload {
				t.Fatalf("download progress went backwards: %d -> %d", lastDownload, p.Completed)
			}
			lastDownload = p.Completed
		}
	}
	if !seen["downloading"] || !seen["converting"] || lastDownload != 2 {
		t.Fatalf("missing download/conversion events: %+v", events)
	}
}

func TestRunDownloadJobRejectsPreCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	output := filepath.Join(t.TempDir(), "never.pdf")
	var stages []string
	_, err := RunDownloadJob(ctx, DownloadJobOptions{
		URL: "https://anyflip.com/qrfai/tfvn", Threads: 1, ChunkSize: 1,
		OutputFile: output, HTTPClient: jobFixtureClient(t),
	}, func(p DownloadProgress) { stages = append(stages, p.Stage) })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation; got %v", err)
	}
	if len(stages) == 0 || stages[len(stages)-1] != "cancelled" {
		t.Fatalf("expected cancelled progress event; got %v", stages)
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("cancelled job unexpectedly created PDF: %v", err)
	}
}

func TestPrepareDownloadContextUsesIndependentTitles(t *testing.T) {
	client := jobFixtureClient(t)
	u, _ := url.Parse("https://anyflip.com/qrfai/tfvn")
	for _, title := range []string{"First", "Second"} {
		book, err := prepareDownloadContext(context.Background(), u, title, client)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.EqualFold(book.title, title) {
			t.Fatalf("got book title %q, want %q", book.title, title)
		}
	}
}

func TestRunDownloadJobRejectsExistingOutput(t *testing.T) {
	output := filepath.Join(t.TempDir(), "existing.pdf")
	if err := os.WriteFile(output, []byte("untouched"), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := RunDownloadJob(context.Background(), DownloadJobOptions{
		URL:     "https://anyflip.com/qrfai/tfvn",
		Threads: 2, ChunkSize: 1,
		OutputFile: output, HTTPClient: jobFixtureClient(t),
	}, nil)
	if err == nil {
		t.Fatal("expected existing output to be rejected")
	}
	content, readErr := os.ReadFile(output)
	if readErr != nil || string(content) != "untouched" {
		t.Fatalf("existing output modified: %q %v", content, readErr)
	}
}

func TestCreatePDFVerifiedContextCancellationDoesNotPublish(t *testing.T) {
	imageDir := t.TempDir()
	writePNG(t, imageDir, "0000.png")
	writePNG(t, imageDir, "0001.png")
	output := filepath.Join(t.TempDir(), "book.pdf")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err := createPDFVerifiedContext(ctx, output, imageDir, 2, 1,
		func(completed int) {
			if completed == 1 {
				cancel()
			}
		})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation between import chunks, got %v", err)
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("cancelled PDF output exists: %v", err)
	}
}

func TestRunDownloadJobInvalidOptions(t *testing.T) {
	_, err := RunDownloadJob(context.Background(), DownloadJobOptions{
		URL: "https://anyflip.com/qrfai/tfvn", Threads: 0, ChunkSize: 0,
	}, nil)
	if err == nil {
		t.Fatal("expected zero threads/chunk size to fail before accessing network")
	}
}
