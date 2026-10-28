package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
)

func TestP1PublisherAlternativesPreservedAndBounded(t *testing.T) {
	const metadata = `{"title":"Sample","totalPageCount":2,"pages":[{"n":["../files/large/p1.webp","../files/mobile/p1.jpg","../files/mobile/p1.jpg"]},{"n":["../files/large/p2.png"]}]}`
	client := imageTestClient(func(req *http.Request) (*http.Response, error) {
		return mockResponse(req, http.StatusOK, metadata), nil
	})
	u, _ := url.Parse("https://anyflip.com/qrfai/tfvn")
	book, err := prepareDownloadContext(context.Background(), u, "", client)
	if err != nil {
		t.Fatal(err)
	}
	if book.pageCount != 2 || len(book.pageURLs) != 2 {
		t.Fatalf("wrong page count or primary URL count: %+v", book)
	}
	if !strings.HasSuffix(book.pageURLs[0], "/files/large/p1.webp") ||
		!strings.HasSuffix(book.pageFallbackURLs[0][0], "/files/mobile/p1.jpg") ||
		len(book.pageFallbackURLs[0]) != 1 || len(book.pageFallbackURLs[1]) != 0 {
		t.Fatalf("primary or fallback candidates wrong: %+v", book)
	}
}

func TestP1RejectsUnsafeAndExcessiveAlternatives(t *testing.T) {
	cases := []string{
		`{"totalPageCount":1,"pages":[{"n":["good.png","https://evil.example/a.png"]}]}`,
		`{"totalPageCount":1,"pages":[{"n":["good.png","../files/large/../../../../other/secret.png"]}]}`,
		`{"totalPageCount":1,"pages":[{"n":["good.png",""]}]}`,
		`{"totalPageCount":1,"pages":[{"n":["1.png","2.png","3.png","4.png","5.png","6.png","7.png","8.png","9.png"]}]}`,
	}
	u, _ := url.Parse("https://anyflip.com/qrfai/tfvn")
	for i, config := range cases {
		t.Run(fmt.Sprintf("invalid-%d", i), func(t *testing.T) {
			var requested atomic.Int32
			client := imageTestClient(func(req *http.Request) (*http.Response, error) {
				requested.Add(1)
				return mockResponse(req, http.StatusOK, config), nil
			})
			if _, err := prepareDownloadContext(context.Background(), u, "", client); err == nil {
				t.Fatal("unsafe or excessive candidate unexpectedly accepted")
			}
			if requested.Load() != 1 {
				t.Fatalf("unexpected image request: %d", requested.Load())
			}
		})
	}
}

func TestP1FallbackOnlyOnDefinitiveMissingPage(t *testing.T) {
	for _, missingStatus := range []int{http.StatusNotFound, http.StatusGone} {
		t.Run(http.StatusText(missingStatus), func(t *testing.T) {
			book := imageBook(t, 1)
			book.pageFallbackURLs = [][]string{{"https://online.anyflip.com/qrfai/tfvn/files/mobile/1.png"}}
			png := imageFixture(t)
			var requested []string
			client := imageTestClient(func(req *http.Request) (*http.Response, error) {
				requested = append(requested, req.URL.Path)
				if strings.Contains(req.URL.Path, "/files/large/") {
					return responseBytes(req, missingStatus, nil), nil
				}
				return responseBytes(req, http.StatusOK, png), nil
			})
			dir := t.TempDir()
			err := book.downloadPageContext(context.Background(), client, 0, dir, downloadOptions{retries: 0})
			if err != nil {
				t.Fatal(err)
			}
			if len(requested) != 2 || !strings.Contains(requested[0], "/large/") ||
				!strings.Contains(requested[1], "/mobile/") {
				t.Fatalf("unexpected fallback order: %v", requested)
			}
			if info, err := os.Stat(filepath.Join(dir, "0000.png")); err != nil || info.Size() == 0 {
				t.Fatalf("missing fallback page: %v %v", info, err)
			}
		})
	}
}

func TestP1AuthRateLimitAndServerFailureNeverDowngrade(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusTooManyRequests, http.StatusServiceUnavailable} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			book := imageBook(t, 1)
			book.pageFallbackURLs = [][]string{{"https://online.anyflip.com/qrfai/tfvn/files/mobile/1.png"}}
			var requests []string
			client := imageTestClient(func(req *http.Request) (*http.Response, error) {
				requests = append(requests, req.URL.Path)
				return responseBytes(req, status, nil), nil
			})
			dir := t.TempDir()
			err := book.downloadPageContext(context.Background(), client, 0, dir,
				downloadOptions{retries: 1})
			if err == nil {
				t.Fatal("expected failure")
			}
			for _, requested := range requests {
				if strings.Contains(requested, "/mobile/") {
					t.Fatalf("unsafe fallback after HTTP %d: %v", status, requests)
				}
			}
		})
	}
}

func TestP1InvalidImageDoesNotSilentlyDowngrade(t *testing.T) {
	book := imageBook(t, 1)
	book.pageFallbackURLs = [][]string{{"https://online.anyflip.com/qrfai/tfvn/files/mobile/1.png"}}
	var requests atomic.Int32
	client := imageTestClient(func(req *http.Request) (*http.Response, error) {
		requests.Add(1)
		return responseBytes(req, http.StatusOK, []byte("not an image")), nil
	})
	dir := t.TempDir()
	if err := book.downloadPageContext(context.Background(), client, 0, dir, downloadOptions{retries: 0}); err == nil {
		t.Fatal("corrupt publisher image must fail closed")
	}
	if requests.Load() != 1 {
		t.Fatalf("corrupt primary unexpectedly retried through fallback: %d", requests.Load())
	}
	if entries, err := os.ReadDir(dir); err != nil || len(entries) != 0 {
		t.Fatalf("invalid downloaded image retained: %v, %v", entries, err)
	}
}

func TestP1MaliciousFallbackRejectedBeforeNetwork(t *testing.T) {
	book := imageBook(t, 1)
	book.pageFallbackURLs = [][]string{{"https://online.anyflip.com/another/book/files/large/private.png"}}
	var attempts atomic.Int32
	client := imageTestClient(func(req *http.Request) (*http.Response, error) {
		attempts.Add(1)
		return responseBytes(req, http.StatusOK, imageFixture(t)), nil
	})
	if err := book.downloadPageContext(context.Background(), client, 0, t.TempDir(), downloadOptions{retries: 0}); err == nil {
		t.Fatal("cross-book alternate must not be reachable")
	}
	if attempts.Load() != 0 {
		t.Fatalf("made %d network requests before checking fallback paths", attempts.Load())
	}
}

func TestP1PublisherFallbackFullTwoPagePDF(t *testing.T) {
	const config = `{"title":"Fallback Fixture","totalPageCount":2,"pages":[{"n":["../files/large/first.png","../files/mobile/first.png"]},{"n":["../files/large/second.png"]}]}`
	png := imageFixture(t)
	var missing atomic.Int32
	client := imageTestClient(func(req *http.Request) (*http.Response, error) {
		switch {
		case strings.HasSuffix(req.URL.Path, "/config.js"):
			return mockResponse(req, http.StatusOK, config), nil
		case strings.HasSuffix(req.URL.Path, "/files/large/first.png"):
			missing.Add(1)
			return responseBytes(req, http.StatusNotFound, nil), nil
		case strings.HasSuffix(req.URL.Path, "/files/mobile/first.png"),
			strings.HasSuffix(req.URL.Path, "/files/large/second.png"):
			return responseBytes(req, http.StatusOK, png), nil
		default:
			t.Errorf("unexpected URL: %s", req.URL.Redacted())
			return responseBytes(req, http.StatusNotFound, nil), nil
		}
	})
	output := filepath.Join(t.TempDir(), "book.pdf")
	_, err := RunDownloadJob(context.Background(), DownloadJobOptions{
		URL: "https://anyflip.com/qrfai/tfvn", OutputFile: output,
		TempDownloadFolder: filepath.Join(t.TempDir(), "images"), Threads: 2,
		ChunkSize: 1, HTTPClient: client,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if missing.Load() != 1 {
		t.Fatalf("primary page should be tried once, got %d", missing.Load())
	}
	if count, err := api.PageCountFile(output); err != nil || count != 2 {
		t.Fatalf("invalid final PDF: pages=%d error=%v", count, err)
	}
}
