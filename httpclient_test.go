package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

type mockRoundTrip func(*http.Request) (*http.Response, error)

func (fn mockRoundTrip) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func mockResponse(req *http.Request, code int, text string) *http.Response {
	return &http.Response{
		Status:     http.StatusText(code),
		StatusCode: code,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(text)),
		Request:    req,
	}
}

func sampleBookURL(t *testing.T) *url.URL {
	t.Helper()
	u, err := url.Parse("https://anyflip.com/qrfai/tfvn/basic")
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func TestConfigFetchAcceptsExpectedPath(t *testing.T) {
	client := newAnyFlipHTTPClient()
	client.Transport = mockRoundTrip(func(req *http.Request) (*http.Response, error) {
		if got, want := req.URL.String(), "https://online.anyflip.com/qrfai/tfvn/mobile/javascript/config.js"; got != want {
			t.Errorf("requested %q; expected %q", got, want)
		}
		if req.Header.Get("User-Agent") == "" {
			t.Error("missing User-Agent")
		}
		return mockResponse(req, http.StatusOK, "var bookConfig={};"), nil
	})
	got, err := downloadConfigJSFileContext(context.Background(), client, sampleBookURL(t))
	if err != nil || got != "var bookConfig={};" {
		t.Fatalf("got content=%q error=%v", got, err)
	}
}

func TestConfigFetchRejectsBadStatus(t *testing.T) {
	client := newAnyFlipHTTPClient()
	client.Transport = mockRoundTrip(func(req *http.Request) (*http.Response, error) {
		return mockResponse(req, http.StatusNotFound, "not found"), nil
	})
	if _, err := downloadConfigJSFileContext(context.Background(), client, sampleBookURL(t)); err == nil {
		t.Fatal("expected non-200 response to fail")
	}
}

func TestConfigFetchRejectsLargeResponse(t *testing.T) {
	client := newAnyFlipHTTPClient()
	client.Transport = mockRoundTrip(func(req *http.Request) (*http.Response, error) {
		return mockResponse(req, http.StatusOK, strings.Repeat("x", int(maxConfigJSBytes)+1)), nil
	})
	if _, err := downloadConfigJSFileContext(context.Background(), client, sampleBookURL(t)); err == nil {
		t.Fatal("expected oversized configuration to fail")
	}
}

func TestConfigFetchHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client := newAnyFlipHTTPClient()
	client.Transport = mockRoundTrip(func(req *http.Request) (*http.Response, error) {
		<-req.Context().Done()
		return nil, req.Context().Err()
	})
	_, err := downloadConfigJSFileContext(ctx, client, sampleBookURL(t))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
}

func TestConfigFetchRejectsForeignOrigin(t *testing.T) {
	u, _ := url.Parse("https://evil.example/private")
	if _, err := downloadConfigJSFileContext(context.Background(), newAnyFlipHTTPClient(), u); err == nil {
		t.Fatal("expected foreign origin to be rejected without a request")
	}
}

func TestAnyFlipHTTPClientRedirectRestrictions(t *testing.T) {
	c := newAnyFlipHTTPClient()
	urls := []struct {
		url     string
		allowed bool
	}{
		{"https://online.anyflip.com/qrfai/tfvn/mobile/index.html", true},
		{"http://online.anyflip.com/qrfai/tfvn", false},
		{"https://evil.example/steal", false},
		{"https://online.anyflip.com.evil.example/private", false},
		{"https://online.anyflip.com:8443/private", false},
	}
	for _, tc := range urls {
		t.Run(tc.url, func(t *testing.T) {
			u, err := url.Parse(tc.url)
			if err != nil {
				t.Fatal(err)
			}
			err = c.CheckRedirect(&http.Request{URL: u}, nil)
			if (err == nil) != tc.allowed {
				t.Fatalf("redirect to %s allowed=%v, error=%v", tc.url, tc.allowed, err)
			}
		})
	}
	u, _ := url.Parse("https://online.anyflip.com/a/b")
	hops := make([]*http.Request, 5)
	if err := c.CheckRedirect(&http.Request{URL: u}, hops); err == nil {
		t.Fatal("expected redirect loop guard")
	}
}
