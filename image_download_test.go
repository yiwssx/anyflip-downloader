package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func imageFixture(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func imageBook(t *testing.T, pageCount int) *flipbook {
	t.Helper()
	base, err := url.Parse("https://online.anyflip.com/qrfai/tfvn")
	if err != nil {
		t.Fatal(err)
	}
	fb := &flipbook{URL: base, pageCount: pageCount}
	for i := 1; i <= pageCount; i++ {
		fb.pageURLs = append(fb.pageURLs,
			fmt.Sprintf("https://online.anyflip.com/qrfai/tfvn/files/large/%d.png", i))
	}
	return fb
}

func imageTestClient(fn mockRoundTrip) *http.Client {
	return &http.Client{Transport: fn, Timeout: 2 * time.Second}
}

func responseBytes(req *http.Request, status int, content []byte) *http.Response {
	return &http.Response{
		Status:        fmt.Sprintf("%d %s", status, http.StatusText(status)),
		StatusCode:    status,
		Header:        http.Header{"Content-Type": {"image/png"}},
		ContentLength: int64(len(content)),
		Body:          io.NopCloser(bytes.NewReader(content)),
		Request:       req,
	}
}

func TestDownloadImagesConcurrentSuccess(t *testing.T) {
	fb := imageBook(t, 3)
	png := imageFixture(t)
	var count atomic.Int32
	client := imageTestClient(func(req *http.Request) (*http.Response, error) {
		count.Add(1)
		if req.Header.Get("Referer") != fb.URL.String() {
			t.Errorf("unexpected referer: %s", req.Header.Get("Referer"))
		}
		return responseBytes(req, http.StatusOK, png), nil
	})
	dir := filepath.Join(t.TempDir(), "images")
	err := fb.downloadImagesContext(context.Background(), client, dir,
		downloadOptions{threads: 3, retries: 0})
	if err != nil {
		t.Fatal(err)
	}
	if n := count.Load(); n != 3 {
		t.Fatalf("got %d requests, expected 3", n)
	}
	for page := 0; page < 3; page++ {
		name := filepath.Join(dir, fmt.Sprintf("%04d.png", page))
		content, err := os.ReadFile(name)
		if err != nil || !bytes.Equal(content, png) {
			t.Fatalf("invalid downloaded page %d: %v", page+1, err)
		}
	}
}

func TestDownloadPageRetriesTemporaryStatus(t *testing.T) {
	png := imageFixture(t)
	var attempts atomic.Int32
	client := imageTestClient(func(req *http.Request) (*http.Response, error) {
		if attempts.Add(1) == 1 {
			return responseBytes(req, http.StatusServiceUnavailable, nil), nil
		}
		return responseBytes(req, http.StatusOK, png), nil
	})
	folder := t.TempDir()
	err := imageBook(t, 1).downloadPageContext(context.Background(), client, 0, folder,
		downloadOptions{retries: 1, retryDelay: time.Millisecond})
	if err != nil || attempts.Load() != 2 {
		t.Fatalf("got error %v after %d requests", err, attempts.Load())
	}
}

func TestDownloadPageDoesNotRetryPermanentStatus(t *testing.T) {
	var attempts atomic.Int32
	client := imageTestClient(func(req *http.Request) (*http.Response, error) {
		attempts.Add(1)
		return responseBytes(req, http.StatusNotFound, nil), nil
	})
	err := imageBook(t, 1).downloadPageContext(context.Background(), client, 0, t.TempDir(),
		downloadOptions{retries: 3})
	if err == nil || attempts.Load() != 1 {
		t.Fatalf("expected immediate failure, got %v (%d requests)", err, attempts.Load())
	}
}

func TestDownloadPageRejectsInvalidImageAndRemovesPartialFile(t *testing.T) {
	client := imageTestClient(func(req *http.Request) (*http.Response, error) {
		return responseBytes(req, http.StatusOK, []byte("<html>not an image</html>")), nil
	})
	dir := t.TempDir()
	err := imageBook(t, 1).downloadPageContext(context.Background(), client, 0, dir,
		downloadOptions{retries: 0})
	if err == nil {
		t.Fatal("expected corrupt image to fail")
	}
	if _, err := os.Stat(filepath.Join(dir, "0000.png")); !os.IsNotExist(err) {
		t.Fatalf("invalid image file retained: %v", err)
	}
}

func TestDownloadPageRejectsOversizedResponse(t *testing.T) {
	client := imageTestClient(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode:    200,
			Status:        "200 OK",
			ContentLength: maxPageBytes + 1,
			Body:          io.NopCloser(strings.NewReader("x")),
			Header:        make(http.Header),
			Request:       req,
		}, nil
	})
	dir := t.TempDir()
	err := imageBook(t, 1).downloadPageContext(context.Background(), client, 0, dir,
		downloadOptions{retries: 0})
	if err == nil {
		t.Fatal("expected oversized image to fail")
	}
	if _, err := os.Stat(filepath.Join(dir, "0000.png")); !os.IsNotExist(err) {
		t.Fatalf("invalid image retained: %v", err)
	}
}

func TestDownloadPageDoesNotOverwriteExistingFile(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "0000.png")
	if err := os.WriteFile(target, []byte("user-data"), 0600); err != nil {
		t.Fatal(err)
	}
	client := imageTestClient(func(req *http.Request) (*http.Response, error) {
		return responseBytes(req, 200, imageFixture(t)), nil
	})
	err := imageBook(t, 1).downloadPageContext(context.Background(), client, 0, dir,
		downloadOptions{retries: 0})
	if err == nil {
		t.Fatal("expected existing file to block download")
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != "user-data" {
		t.Fatalf("existing file was changed: %q %v", data, err)
	}
}

func TestDownloadImagesCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := imageTestClient(func(req *http.Request) (*http.Response, error) {
		cancel()
		<-req.Context().Done()
		return nil, req.Context().Err()
	})
	err := imageBook(t, 3).downloadImagesContext(ctx, client, t.TempDir(),
		downloadOptions{threads: 2, retries: 2, retryDelay: time.Second})
	if !errors.Is(err, context.Canceled) && !strings.Contains(fmt.Sprint(err), "context canceled") {
		t.Fatalf("expected cancellation; got %v", err)
	}
}

func TestDownloadImagesRejectsMissingPages(t *testing.T) {
	book := imageBook(t, 2)
	book.pageURLs = book.pageURLs[:1]
	if err := book.downloadImagesContext(context.Background(), nil, t.TempDir(),
		downloadOptions{threads: 1}); err == nil {
		t.Fatal("expected incomplete page list to fail")
	}
}

func TestDownloadPageRejectsOtherBooks(t *testing.T) {
	book := imageBook(t, 1)
	book.pageURLs[0] = "https://online.anyflip.com/another/book/files/large/1.png"
	err := book.downloadPageContext(context.Background(), nil, 0, t.TempDir(),
		downloadOptions{retries: 0})
	if err == nil {
		t.Fatal("expected foreign-book URL to be rejected")
	}
}

func TestDownloadPageHTTPTimeout(t *testing.T) {
	client := &http.Client{
		Timeout: 15 * time.Millisecond,
		Transport: mockRoundTrip(func(req *http.Request) (*http.Response, error) {
			<-req.Context().Done()
			return nil, req.Context().Err()
		}),
	}
	err := imageBook(t, 1).downloadPageContext(context.Background(), client, 0, t.TempDir(),
		downloadOptions{retries: 0})
	if err == nil {
		t.Fatal("expected timeout to fail")
	}
}

func TestDownloadPageCancellationDuringRetryWait(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var attempts atomic.Int32
	client := imageTestClient(func(req *http.Request) (*http.Response, error) {
		attempts.Add(1)
		cancel()
		return responseBytes(req, http.StatusServiceUnavailable, nil), nil
	})
	err := imageBook(t, 1).downloadPageContext(ctx, client, 0, t.TempDir(),
		downloadOptions{retries: 3, retryDelay: time.Minute})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancelled retry to stop, got %v", err)
	}
	if attempts.Load() != 1 {
		t.Fatalf("unexpected extra request count: %d", attempts.Load())
	}
}
