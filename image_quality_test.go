package main

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
)

func qualityFixturePNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), A: 255})
		}
	}
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func qualityBook(t *testing.T) *flipbook {
	t.Helper()
	b := imageBook(t, 1)
	b.pageFallbackURLs = [][]string{{
		"https://online.anyflip.com/qrfai/tfvn/files/mobile/1.png",
	}}
	return b
}

func TestP1BestQualityPicksLargestPixelsNotMetadataOrder(t *testing.T) {
	small := qualityFixturePNG(t, 30, 20)
	large := qualityFixturePNG(t, 100, 40)
	book := qualityBook(t)
	var calls []string
	client := imageTestClient(func(req *http.Request) (*http.Response, error) {
		calls = append(calls, req.URL.Path)
		if strings.Contains(req.URL.Path, "/mobile/") {
			return responseBytes(req, 200, large), nil
		}
		return responseBytes(req, 200, small), nil
	})
	dir := t.TempDir()
	err := book.downloadPageContext(context.Background(), client, 0, dir,
		downloadOptions{retries: 0, bestQuality: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 || !strings.Contains(calls[0], "/large/") || !strings.Contains(calls[1], "/mobile/") {
		t.Fatalf("did not evaluate publisher-listed choices in order: %v", calls)
	}
	b, err := os.ReadFile(filepath.Join(dir, "0000.png"))
	if err != nil || !bytes.Equal(b, large) {
		t.Fatalf("not the highest pixel count image: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 || entries[0].Name() != "0000.png" {
		t.Fatalf("quality staging dir not removed: %v, %v", entries, err)
	}
}

func TestP1BestQualityTiePreservesFirstAdvertisedChoice(t *testing.T) {
	first := qualityFixturePNG(t, 10, 12)
	second := qualityFixturePNG(t, 12, 10)
	book := qualityBook(t)
	client := imageTestClient(func(req *http.Request) (*http.Response, error) {
		if strings.Contains(req.URL.Path, "/mobile/") {
			return responseBytes(req, 200, second), nil
		}
		return responseBytes(req, 200, first), nil
	})
	dir := t.TempDir()
	if err := book.downloadPageContext(context.Background(), client, 0, dir,
		downloadOptions{bestQuality: true}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "0000.png"))
	if err != nil || !bytes.Equal(b, first) {
		t.Fatalf("equal pixel count should retain earlier publisher choice: %v", err)
	}
}

func TestP1BestQualityMissingVariantDoesNotBlockAvailableCandidate(t *testing.T) {
	for _, missing := range []int{http.StatusNotFound, http.StatusGone} {
		t.Run(fmt.Sprint(missing), func(t *testing.T) {
			book := qualityBook(t)
			large := qualityFixturePNG(t, 50, 40)
			client := imageTestClient(func(req *http.Request) (*http.Response, error) {
				if strings.Contains(req.URL.Path, "/large/") {
					return responseBytes(req, missing, nil), nil
				}
				return responseBytes(req, 200, large), nil
			})
			dir := t.TempDir()
			if err := book.downloadPageContext(context.Background(), client, 0, dir,
				downloadOptions{bestQuality: true}); err != nil {
				t.Fatal(err)
			}
			b, err := os.ReadFile(filepath.Join(dir, "0000.png"))
			if err != nil || !bytes.Equal(b, large) {
				t.Fatalf("missing primary failed legitimate fallback: %v", err)
			}
		})
	}
}

func TestP1BestQualityNeverBypassesAccessOrThrottle(t *testing.T) {
	primary := qualityFixturePNG(t, 25, 25)
	for _, status := range []int{401, 403, 429, 503} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			book := qualityBook(t)
			var requests atomic.Int32
			client := imageTestClient(func(req *http.Request) (*http.Response, error) {
				requests.Add(1)
				if strings.Contains(req.URL.Path, "/mobile/") {
					return responseBytes(req, status, nil), nil
				}
				return responseBytes(req, 200, primary), nil
			})
			dir := t.TempDir()
			if err := book.downloadPageContext(context.Background(), client, 0, dir,
				downloadOptions{retries: 0, bestQuality: true}); err == nil {
				t.Fatalf("HTTP %d must fail closed", status)
			}
			if requests.Load() != 2 {
				t.Fatalf("unexpected retries / candidate count: %d", requests.Load())
			}
			entries, err := os.ReadDir(dir)
			if err != nil || len(entries) != 0 {
				t.Fatalf("HTTP %d left partial pages or staging: %v %v", status, entries, err)
			}
		})
	}
}

func TestP1BestQualityCorruptImageDoesNotSelectLowerResolution(t *testing.T) {
	book := qualityBook(t)
	first := qualityFixturePNG(t, 25, 25)
	client := imageTestClient(func(req *http.Request) (*http.Response, error) {
		if strings.Contains(req.URL.Path, "/mobile/") {
			return responseBytes(req, 200, []byte("invalid image bytes")), nil
		}
		return responseBytes(req, 200, first), nil
	})
	dir := t.TempDir()
	if err := book.downloadPageContext(context.Background(), client, 0, dir,
		downloadOptions{retries: 0, bestQuality: true}); err == nil {
		t.Fatal("corrupt alternative must not silently degrade quality")
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		t.Fatalf("corrupt alternative left temporary pages: %v, %v", entries, err)
	}
}

func TestP1BestQualityRejectsCrossBookBeforeNetwork(t *testing.T) {
	book := qualityBook(t)
	book.pageFallbackURLs[0][0] = "https://online.anyflip.com/other/book/files/large/private.png"
	var requests atomic.Int32
	client := imageTestClient(func(req *http.Request) (*http.Response, error) {
		requests.Add(1)
		return responseBytes(req, 200, qualityFixturePNG(t, 12, 14)), nil
	})
	if err := book.downloadPageContext(context.Background(), client, 0, t.TempDir(),
		downloadOptions{bestQuality: true}); err == nil {
		t.Fatal("expected cross-book choice rejection")
	}
	if requests.Load() != 0 {
		t.Fatalf("made network requests before validating every choice: %d", requests.Load())
	}
}

func TestP1BestQualityDoesNotOverwriteExistingPage(t *testing.T) {
	book := qualityBook(t)
	dir := t.TempDir()
	original := []byte("user supplied file")
	target := filepath.Join(dir, "0000.png")
	if err := os.WriteFile(target, original, 0600); err != nil {
		t.Fatal(err)
	}
	imageData := qualityFixturePNG(t, 25, 25)
	client := imageTestClient(func(req *http.Request) (*http.Response, error) {
		return responseBytes(req, 200, imageData), nil
	})
	if err := book.downloadPageContext(context.Background(), client, 0, dir,
		downloadOptions{bestQuality: true}); err == nil {
		t.Fatal("expected refusal to overwrite existing page")
	}
	b, err := os.ReadFile(target)
	if err != nil || !bytes.Equal(b, original) {
		t.Fatalf("existing page overwritten: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("staging not removed after refusal: %v %v", entries, err)
	}
}

func TestP1BestQualityJobCreatesVerifiedPDF(t *testing.T) {
	const config = `{"title":"Quality Fixture","totalPageCount":2,"pages":[{"n":["one-small.png","one-large.png"]},{"n":["two.png"]}]}`
	first := qualityFixturePNG(t, 20, 20)
	larger := qualityFixturePNG(t, 80, 40)
	second := qualityFixturePNG(t, 15, 30)
	client := imageTestClient(func(req *http.Request) (*http.Response, error) {
		switch {
		case strings.HasSuffix(req.URL.Path, "/config.js"):
			return mockResponse(req, 200, config), nil
		case strings.HasSuffix(req.URL.Path, "/one-small.png"):
			return responseBytes(req, 200, first), nil
		case strings.HasSuffix(req.URL.Path, "/one-large.png"):
			return responseBytes(req, 200, larger), nil
		case strings.HasSuffix(req.URL.Path, "/two.png"):
			return responseBytes(req, 200, second), nil
		default:
			return responseBytes(req, 404, nil), nil
		}
	})
	parent := t.TempDir()
	output := filepath.Join(parent, "quality.pdf")
	workspace := filepath.Join(parent, "download")
	_, err := RunDownloadJob(context.Background(), DownloadJobOptions{
		URL: "https://anyflip.com/qrfai/tfvn",
		Threads: 1, ChunkSize: 1, HTTPClient: client,
		BestQuality: true, OutputFile: output, TempDownloadFolder: workspace,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := api.ValidateFile(output, nil); err != nil {
		t.Fatal(err)
	}
	dims, err := api.PageDimsFile(output)
	if err != nil || len(dims) != 2 {
		t.Fatalf("unexpected PDF page dimensions: %v %v", dims, err)
	}
	if dims[0].Width/dims[0].Height < 1.9 || dims[0].Width/dims[0].Height > 2.1 {
		t.Fatalf("page 1 was not selected from 80x40 candidate: %+v", dims[0])
	}
	if _, err := os.Stat(workspace); !os.IsNotExist(err) {
		t.Fatalf("job workspace retained unexpectedly: %v", err)
	}
}
