package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"time"
)

const maxConfigJSBytes int64 = 16 << 20

// newAnyFlipHTTPClient applies a finite timeout and refuses cross-origin
// redirects and pins outbound connections to publicly routable IPs.
// TLS verification is mandatory for this safe shared client.
func newAnyFlipHTTPClient() *http.Client {
	transport := &http.Transport{
		Proxy:                 nil, // Never proxy untrusted book requests into private networks.
		ForceAttemptHTTP2:     true,
		DialContext:           dialAnyFlipContext,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 20 * time.Second,
		IdleConnTimeout:       30 * time.Second,
		MaxIdleConns:          16,
	}
	return &http.Client{
		Timeout:   45 * time.Second,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("too many AnyFlip redirects")
			}
			if req.URL.Scheme != "https" || req.URL.Hostname() != "online.anyflip.com" ||
				req.URL.Port() != "" || req.URL.User != nil {
				return fmt.Errorf("refusing unsafe or cross-origin AnyFlip redirect to %s", req.URL.Redacted())
			}
			return nil
		},
	}
}

// downloadConfigJSFileContext has a cancellation-aware API suitable for reuse
// in a future job-based web backend. It limits third-party response sizes.
func downloadConfigJSFileContext(ctx context.Context, client *http.Client, bookURL *url.URL) (string, error) {
	if ctx == nil {
		return "", errors.New("download context must not be nil")
	}
	if bookURL == nil {
		return "", errors.New("book URL is required")
	}
	canonical, err := validateBookURL(bookURL.String())
	if err != nil {
		return "", err
	}
	if client == nil {
		client = newAnyFlipHTTPClient()
	}
	configURL := &url.URL{
		Scheme: "https",
		Host:   "online.anyflip.com",
		Path:   path.Join(canonical.Path, "mobile", "javascript", "config.js"),
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, configURL.String(), nil)
	if err != nil {
		return "", fmt.Errorf("create config request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch AnyFlip book config: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("received non-200 response for book config: %s", resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxConfigJSBytes+1))
	if err != nil {
		return "", fmt.Errorf("read AnyFlip book config: %w", err)
	}
	if int64(len(b)) > maxConfigJSBytes {
		return "", fmt.Errorf("AnyFlip book config exceeds %d bytes", maxConfigJSBytes)
	}
	return string(b), nil
}
