package main

import (
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/schollz/progressbar/v3"
	_ "golang.org/x/image/webp"
)

const maxPageBytes int64 = 32 << 20

// downloadImages retains the existing CLI API. Web jobs can use the
// cancellation-aware downloadImagesContext with a per-job context.
func (fb *flipbook) downloadImages(folder string, options downloadOptions) error {
	return fb.downloadImagesContext(context.Background(), nil, folder, options)
}

func (fb *flipbook) downloadImagesContext(parent context.Context, client *http.Client, folder string, opts downloadOptions) error {
	return fb.downloadImagesContextWithProgress(parent, client, folder, opts, nil)
}

// onPage receives the one-based page index and monotonic completed-page count.
// Callbacks are serialized and may safely forward updates to a job event channel.
func (fb *flipbook) downloadImagesContextWithProgress(parent context.Context, client *http.Client, folder string, opts downloadOptions, onPage func(page, completed int)) error {
	if parent == nil {
		return errors.New("download context is required")
	}
	if fb == nil || fb.URL == nil || fb.pageCount < 1 || fb.pageCount > 10000 ||
		len(fb.pageURLs) != fb.pageCount {
		return errors.New("invalid or incomplete book page metadata")
	}
	if opts.threads < 1 || opts.threads > 16 || opts.retries < 0 || opts.retries > 10 ||
		opts.retryDelay < 0 || opts.retryDelay > time.Minute {
		return errors.New("invalid image download settings")
	}
	if err := parent.Err(); err != nil {
		return err
	}
	absFolder, err := filepath.Abs(folder)
	if err != nil {
		return fmt.Errorf("invalid image workspace: %w", err)
	}
	if err := os.MkdirAll(absFolder, 0700); err != nil {
		return fmt.Errorf("create image workspace: %w", err)
	}
	if client == nil {
		client = newAnyFlipHTTPClient()
	}
	// Preserve an injected transport for testing, but enforce the same
	// timeout and redirect policy for all clients.
	boundedClient := *client
	boundedClient.CheckRedirect = newAnyFlipHTTPClient().CheckRedirect
	if boundedClient.Timeout == 0 || boundedClient.Timeout > 45*time.Second {
		boundedClient.Timeout = 45 * time.Second
	}

	var bar *progressbar.ProgressBar
	if onPage == nil {
		bar = progressbar.NewOptions(fb.pageCount,
			progressbar.OptionFullWidth(),
			progressbar.OptionSetPredictTime(false),
			progressbar.OptionShowCount(),
			progressbar.OptionSetDescription("Downloading"),
		)
		defer bar.Close()
	}
	var progressMu sync.Mutex
	completed := 0

	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	pages := make(chan int)
	failures := make(chan error, 1)
	var workers sync.WaitGroup
	count := min(opts.threads, fb.pageCount)
	for i := 0; i < count; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case page, ok := <-pages:
					if !ok {
						return
					}
					if err := fb.downloadPageContext(ctx, &boundedClient, page, absFolder, opts); err != nil {
						select {
						case failures <- fmt.Errorf("page %d: %w", page+1, err):
						default:
						}
						cancel()
						return
					}
					if bar != nil {
						_ = bar.Add(1)
					}
					if onPage != nil {
						progressMu.Lock()
						completed++
						onPage(page+1, completed)
						progressMu.Unlock()
					}
				}
			}
		}()
	}
	dispatchDone := make(chan struct{})
	go func() {
		defer close(dispatchDone)
		defer close(pages)
		for page := 0; page < fb.pageCount; page++ {
			select {
			case <-ctx.Done():
				return
			case pages <- page:
			}
		}
	}()

	workers.Wait()
	cancel()
	<-dispatchDone
	select {
	case firstErr := <-failures:
		return firstErr
	default:
	}
	if err := parent.Err(); err != nil {
		return err
	}
	return nil
}

// downloadPage retains compatibility with the old internal caller.
func (fb *flipbook) downloadPage(page int, folder string, opts downloadOptions) error {
	return fb.downloadPageContext(context.Background(), newAnyFlipHTTPClient(), page, folder, opts)
}

func (fb *flipbook) downloadPageContext(ctx context.Context, client *http.Client, page int, folder string, opts downloadOptions) error {
	if ctx == nil {
		return errors.New("download context is required")
	}
	if fb == nil || fb.URL == nil || page < 0 || page >= len(fb.pageURLs) {
		return errors.New("invalid image page reference")
	}
	if opts.retries < 0 || opts.retries > 10 || opts.retryDelay < 0 || opts.retryDelay > time.Minute {
		return errors.New("invalid page retry settings")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	bookURL, err := validateBookURL(fb.URL.String())
	if err != nil {
		return err
	}
	imageURL, err := url.Parse(cleanDownloadURL(fb.pageURLs[page]))
	if err != nil {
		return fmt.Errorf("invalid image URL: %w", err)
	}
	expectedPrefix := path.Clean(bookURL.Path) + "/files/"
	if imageURL.Scheme != "https" || imageURL.Hostname() != "online.anyflip.com" ||
		imageURL.Port() != "" || imageURL.User != nil ||
		!strings.HasPrefix(imageURL.Path, expectedPrefix) ||
		imageURL.RawQuery != "" || imageURL.Fragment != "" {
		return fmt.Errorf("image URL is not within the selected book")
	}
	ext := strings.ToLower(path.Ext(imageURL.Path))
	if ext != ".jpg" && ext != ".jpeg" && ext != ".png" && ext != ".webp" {
		return fmt.Errorf("unsupported image extension %q", ext)
	}
	filename := filepath.Join(folder, fmt.Sprintf("%04d%s", page, ext))
	if client == nil {
		client = newAnyFlipHTTPClient()
	}
	safeClient := *client
	safeClient.CheckRedirect = newAnyFlipHTTPClient().CheckRedirect
	if safeClient.Timeout <= 0 || safeClient.Timeout > 45*time.Second {
		safeClient.Timeout = 45 * time.Second
	}

	var lastErr error
	for attempt := 0; attempt <= opts.retries; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		if attempt > 0 {
			timer := time.NewTimer(opts.retryDelay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL.String(), nil)
		if err != nil {
			return err
		}
		req.Header.Set("Referer", bookURL.String())
		req.Header.Set("User-Agent", "Mozilla/5.0")
		resp, err := safeClient.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			lastErr = err
			continue
		}
		if resp.StatusCode != http.StatusOK {
			_ = resp.Body.Close()
			lastErr = fmt.Errorf("image request returned %s", resp.Status)
			if resp.StatusCode != http.StatusTooManyRequests && resp.StatusCode < 500 {
				return lastErr
			}
			continue
		}
		lastErr = saveImageResponse(filename, resp)
		if lastErr == nil {
			return nil
		}
		if errors.Is(lastErr, os.ErrExist) {
			return lastErr
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
	return fmt.Errorf("image download failed after %d attempts: %w", opts.retries+1, lastErr)
}

func saveImageResponse(filename string, resp *http.Response) (resultErr error) {
	defer resp.Body.Close()
	if resp.ContentLength > maxPageBytes {
		return fmt.Errorf("image exceeds %d bytes", maxPageBytes)
	}
	file, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("open new image file: %w", err)
	}
	defer func() {
		_ = file.Close()
		if resultErr != nil {
			_ = os.Remove(filename)
		}
	}()

	n, err := io.Copy(file, io.LimitReader(resp.Body, maxPageBytes+1))
	if err != nil {
		return fmt.Errorf("read image response: %w", err)
	}
	if n == 0 || n > maxPageBytes {
		return fmt.Errorf("invalid image size %d bytes", n)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close image file: %w", err)
	}
	check, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("reopen image for validation: %w", err)
	}
	defer check.Close()
	cfg, format, err := image.DecodeConfig(check)
	if err != nil {
		return fmt.Errorf("invalid image data: %w", err)
	}
	if cfg.Width < 1 || cfg.Height < 1 {
		return errors.New("invalid image dimensions")
	}
	if format != "jpeg" && format != "png" && format != "webp" {
		return fmt.Errorf("unsupported image format %q", format)
	}
	return nil
}
