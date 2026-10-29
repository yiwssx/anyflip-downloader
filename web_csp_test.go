package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Browser assets must remain embedded and same-origin so that the Web GUI
// can run without unsafe-inline, external CDNs or writable static directories.
func TestWebStrictCSPAndEmbeddedAssets(t *testing.T) {
	s, err := newWebServer(testWebToken, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(s.handler())
	defer ts.Close()

	html := webRequest(t, ts, http.MethodGet, "/", nil, false)
	if html.StatusCode != http.StatusOK {
		html.Body.Close()
		t.Fatalf("Web UI HTTP %d", html.StatusCode)
	}
	csp := html.Header.Get("Content-Security-Policy")
	if strings.Contains(csp, "'unsafe-inline'") || strings.Contains(csp, "'unsafe-eval'") {
		t.Fatalf("CSP permits unsafe inline/eval execution: %q", csp)
	}
	for _, directive := range []string{
		"default-src 'none'", "script-src 'self'", "style-src 'self'",
		"connect-src 'self'", "base-uri 'none'", "form-action 'self'",
		"frame-ancestors 'none'",
	} {
		if !strings.Contains(csp, directive) {
			t.Errorf("missing CSP directive %q: %q", directive, csp)
		}
	}
	if got := html.Header.Get("Cache-Control"); got != "no-store" {
		t.Errorf("Web UI cache policy changed: %q", got)
	}
	body, err := io.ReadAll(html.Body)
	html.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	source := string(body)
	for _, required := range []string{`href="/style.css"`, `src="/app.js"`, `id="jobForm"`, `id="batchURLs"`, `id="historyList"`} {
		if !strings.Contains(source, required) {
			t.Errorf("required Web GUI markup missing %s", required)
		}
	}
	for _, forbidden := range []string{"<style>", "<script>", " onclick=", " onload="} {
		if strings.Contains(strings.ToLower(source), forbidden) {
			t.Errorf("inline executable HTML still present %q", forbidden)
		}
	}

	for _, asset := range []struct {
		path, contentType, expected string
	}{
		{"/style.css", "text/css; charset=utf-8", "@media(max-width:630px)"},
		{"/app.js", "text/javascript; charset=utf-8", "jobForm"},
	} {
		res := webRequest(t, ts, http.MethodGet, asset.path, nil, false)
		if res.StatusCode != http.StatusOK {
			res.Body.Close()
			t.Errorf("%s returned HTTP %d", asset.path, res.StatusCode)
			continue
		}
		if got := res.Header.Get("Content-Type"); got != asset.contentType {
			t.Errorf("%s content type=%q", asset.path, got)
		}
		if got := res.Header.Get("Content-Security-Policy"); got != csp {
			t.Errorf("%s CSP differs from HTML", asset.path)
		}
		if got := res.Header.Get("Cache-Control"); got != "no-store" {
			t.Errorf("%s must not be cached across token/session changes: %q", asset.path, got)
		}
		data, err := io.ReadAll(res.Body)
		res.Body.Close()
		if err != nil || len(data) < 100 || !strings.Contains(string(data), asset.expected) {
			t.Errorf("%s contents unavailable or invalid: bytes=%d err=%v", asset.path, len(data), err)
		}
	}

	missing := webRequest(t, ts, http.MethodGet, "/private-config.js", nil, false)
	missing.Body.Close()
	if missing.StatusCode != http.StatusNotFound {
		t.Fatalf("unexpected file path served: HTTP %d", missing.StatusCode)
	}
	protected := webRequest(t, ts, http.MethodGet, "/api/jobs", nil, false)
	protected.Body.Close()
	if protected.StatusCode != http.StatusUnauthorized {
		t.Fatalf("static asset change bypassed API authentication: HTTP %d", protected.StatusCode)
	}
}
