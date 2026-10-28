package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestWebAuthThrottleFailedBearerRequests(t *testing.T) {
	s, err := newWebServer(testWebToken, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	s.authThrottle.clock = func() time.Time { return now }
	h := s.handler()
	call := func(peer, token, forwarded string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/api/jobs/"+strings.Repeat("0", 32), nil)
		req.RemoteAddr = peer
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		if forwarded != "" {
			req.Header.Set("X-Forwarded-For", forwarded)
		}
		result := httptest.NewRecorder()
		h.ServeHTTP(result, req)
		return result
	}
	peer := "192.0.2.10:12345"
	for i := 0; i < authMaxFailures; i++ {
		if got := call(peer, "bad-token", "").Code; got != http.StatusUnauthorized {
			t.Fatalf("failure %d: status=%d, want 401", i, got)
		}
	}
	limited := call(peer, "bad-token", "203.0.113.200")
	if limited.Code != http.StatusTooManyRequests {
		t.Fatalf("same TCP peer bypassed throttle via forwarded header: %d", limited.Code)
	}
	if got := limited.Header().Get("Retry-After"); got != "300" {
		t.Fatalf("Retry-After=%q, want 300", got)
	}
	if got := call(peer, testWebToken, "").Code; got != http.StatusTooManyRequests {
		t.Fatalf("correct token should not bypass temporary peer cooldown: %d", got)
	}
	if got := call("192.0.2.11:54321", testWebToken, "").Code; got != http.StatusNotFound {
		t.Fatalf("independent peer blocked: %d", got)
	}
	health := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	health.RemoteAddr = peer
	healthResponse := httptest.NewRecorder()
	h.ServeHTTP(healthResponse, health)
	if healthResponse.Code != http.StatusOK {
		t.Fatalf("health probe blocked: %d", healthResponse.Code)
	}

	now = now.Add(authLockout + time.Second)
	if got := call(peer, testWebToken, "").Code; got != http.StatusNotFound {
		t.Fatalf("valid token should work after cooldown: %d", got)
	}
	if n := len(s.authThrottle.peers); n != 0 {
		t.Fatalf("successful authentication did not clear failures: %d", n)
	}
}

func TestWebAuthThrottleWindowResetAndBoundedMemory(t *testing.T) {
	a := newAuthThrottle()
	now := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	a.clock = func() time.Time { return now }
	peer := "198.51.100.10:8000"
	a.recordFailure(peer)
	now = now.Add(authFailureWindow + time.Second)
	for i := 0; i < authMaxFailures-1; i++ {
		a.recordFailure(peer)
	}
	if got := a.retryAfter(peer); got != 0 {
		t.Fatalf("stale failures must expire before threshold: %s", got)
	}
	a.recordFailure(peer)
	if got := a.retryAfter(peer); got != authLockout {
		t.Fatalf("threshold did not trigger cooldown: %s", got)
	}

	for i := 0; i < authMaxPeers+10; i++ {
		a.recordFailure(fmt.Sprintf("203.0.%d.%d:5432", i/256, i%256))
	}
	if got := len(a.peers); got > authMaxPeers {
		t.Fatalf("unbounded per-peer auth tracking: %d > %d", got, authMaxPeers)
	}
}
