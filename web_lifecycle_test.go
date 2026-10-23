package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestWebServerEnforcesHTTPTimeouts(t *testing.T) {
	web, err := newWebServer(testWebToken, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	server := newWebHTTPServer("127.0.0.1:0", web)
	if server.ReadHeaderTimeout == 0 || server.ReadTimeout == 0 ||
		server.WriteTimeout == 0 || server.IdleTimeout == 0 ||
		server.MaxHeaderBytes < 1024 {
		t.Fatal("web server has unbounded HTTP operations")
	}
	if server.Addr != "127.0.0.1:0" {
		t.Fatal("web server address changed")
	}
}

func TestWebShutdownCancelsActiveJobsAndRejectsNewOnes(t *testing.T) {
	web, err := newWebServer(testWebToken, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	id := strings.Repeat("e", 32)
	web.mu.Lock()
	web.jobs[id] = &webJob{id: id, stage: "downloading", cancel: cancel}
	web.mu.Unlock()

	web.beginShutdown()
	if ctx.Err() != context.Canceled {
		t.Fatalf("active work was not cancelled: %v", ctx.Err())
	}
	if !web.stopping {
		t.Fatal("shutdown flag not set")
	}

	server := httptest.NewServer(web.handler())
	defer server.Close()
	result := webRequest(t, server, "POST", "/api/jobs",
		[]byte(`{"url":"https://anyflip.com/qrfai/tfvn","threads":2}`), true)
	defer result.Body.Close()
	if result.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected new jobs to be rejected with HTTP 503, got %d", result.StatusCode)
	}
	if len(web.jobs) != 1 {
		t.Fatal("unexpected new job registered during shutdown")
	}

	// Shutdown remains idempotent even after a worker completed.
	web.mu.Lock()
	web.jobs[id].finished = time.Now()
	web.mu.Unlock()
	web.beginShutdown()
}
