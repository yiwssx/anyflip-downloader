package main

import (
	"context"
	"errors"
	"fmt"
	"image"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// downloadPageBestQualityContext is an explicit opt-in. It evaluates only
// publisher-listed choices for the requested book. It never guesses a URL,
// bypasses a denial or silently changes the default first-choice behavior.
// Ranking uses decoded width*height; ties keep publisher order.
func (fb *flipbook) downloadPageBestQualityContext(ctx context.Context, client *http.Client, page int, folder string, opts downloadOptions) error {
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
	raw := []string{fb.pageURLs[page]}
	if page < len(fb.pageFallbackURLs) {
		raw = append(raw, fb.pageFallbackURLs[page]...)
	}
	if len(raw) < 1 || len(raw) > 8 {
		return errors.New("invalid image quality candidate count")
	}

	// Validate ALL candidates before contacting the publisher.
	candidates := make([]*url.URL, 0, len(raw))
	for _, candidate := range raw {
		u, err := validatePageImageURL(bookURL, candidate)
		if err != nil {
			return err
		}
		candidates = append(candidates, u)
	}
	if client == nil {
		client = newAnyFlipHTTPClient()
	}
	safeClient := *client
	originRedirect := newAnyFlipHTTPClient().CheckRedirect
	safeClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if err := originRedirect(req, via); err != nil {
			return err
		}
		if _, err := validatePageImageURL(bookURL, req.URL.String()); err != nil {
			return fmt.Errorf("image redirect escapes selected book: %w", err)
		}
		return nil
	}
	if safeClient.Timeout <= 0 || safeClient.Timeout > 45*time.Second {
		safeClient.Timeout = 45 * time.Second
	}
	// All candidate files live inside a private, disposable subdirectory
	// on the same filesystem as the output image. No third-party path controls it.
	stageDir, err := os.MkdirTemp(folder, ".anyflip-quality-")
	if err != nil {
		return fmt.Errorf("create private quality workspace: %w", err)
	}
	defer os.RemoveAll(stageDir)

	var bestFile, bestExt string
	var bestPixels int64
	var lastMissing error
	for i, candidate := range candidates {
		if err := ctx.Err(); err != nil {
			return err
		}
		ext := strings.ToLower(path.Ext(candidate.Path))
		staging := filepath.Join(stageDir, fmt.Sprintf("choice-%02d%s", i, ext))
		unavailable, err := downloadImageCandidate(ctx, &safeClient, bookURL, candidate, staging, opts)
		if unavailable {
			lastMissing = err // 404/410 on an explicitly listed file.
			continue
		}
		// No lower quality candidate is permitted after 401/403, 429, 5xx,
		// TLS/network failure, content corruption or a rejected redirect.
		if err != nil {
			return fmt.Errorf("quality candidate %d: %w", i+1, err)
		}
		f, err := os.Open(staging)
		if err != nil {
			return err
		}
		cfg, _, cfgErr := image.DecodeConfig(f)
		closeErr := f.Close()
		if cfgErr != nil {
			return fmt.Errorf("inspect quality candidate %d: %w", i+1, cfgErr)
		}
		if closeErr != nil {
			return closeErr
		}
		// saveImageResponse already validates positive dimensions.
		pixels := int64(cfg.Width) * int64(cfg.Height)
		if pixels > bestPixels {
			if bestFile != "" {
				if err := os.Remove(bestFile); err != nil {
					return fmt.Errorf("remove obsolete candidate: %w", err)
				}
			}
			bestPixels, bestFile, bestExt = pixels, staging, ext
		} else if err := os.Remove(staging); err != nil {
			return fmt.Errorf("remove lower resolution candidate: %w", err)
		}
	}
	if bestFile == "" {
		return fmt.Errorf("no publisher-listed image candidate is available for page %d: %w", page+1, lastMissing)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// Hard-link publishing makes it impossible to overwrite a user file,
	// and the normal PDF collector sees exactly one file for this page.
	target := filepath.Join(folder, fmt.Sprintf("%04d%s", page, bestExt))
	if err := os.Link(bestFile, target); err != nil {
		return fmt.Errorf("publish selected page without overwrite: %w", err)
	}
	return nil
}
