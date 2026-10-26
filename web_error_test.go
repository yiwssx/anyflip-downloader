package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDescribeWebJobError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"cancel", fmt.Errorf("cancel operation: %w", context.Canceled), "job_cancelled"},
		{"deadline", fmt.Errorf("timed out: %w", context.DeadlineExceeded), "job_timeout"},
		{"forbidden", errors.New("image request returned 403 Forbidden"), "source_access_denied"},
		{"unauthorized", errors.New("received non-200 response for book config: 401 Unauthorized"), "source_access_denied"},
		{"missing", errors.New("image request returned 404 Not Found"), "source_not_found"},
		{"rate limited", errors.New("image request returned 429 Too Many Requests"), "source_rate_limited"},
		{"metadata", errors.New("invalid AnyFlip book configuration: empty page count"), "metadata_invalid"},
		{"invalid image", errors.New("invalid image data: unsupported"), "image_invalid"},
		{"storage", errors.New("create PDF staging folder: permission denied"), "storage_error"},
		{"TLS", errors.New("x509: certificate signed by unknown authority"), "tls_error"},
		{"DNS", errors.New("resolve AnyFlip hostname: lookup failed"), "network_error"},
		{"unknown", errors.New("an unexpected internal error"), "job_failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, message := describeWebJobError(tt.err)
			if code != tt.want || message == "" {
				t.Fatalf("got code=%q message=%q, want %q", code, message, tt.want)
			}
		})
	}
}

func TestWebJobFailureIsUsefulButRedacted(t *testing.T) {
	s, err := newWebServer(testWebToken, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	id := strings.Repeat("a", 32)
	j := &webJob{id: id, stage: "preparing"}
	s.jobs[id] = j
	s.finishJob(j, "", errors.New("fetch book https://secret.example/private?token=secret; workspace /mnt/private: received non-200 response for book config: 403 Forbidden"))
	ts := httptest.NewServer(s.handler())
	defer ts.Close()
	res := webRequest(t, ts, "GET", "/api/jobs/"+id, nil, true)
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status %d", res.StatusCode)
	}
	var payload webJobResponse
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.ErrorCode != "source_access_denied" || payload.Error == "" {
		t.Fatalf("missing safe diagnostic: %+v", payload)
	}
	if strings.Contains(payload.Error, "/mnt/private") || strings.Contains(payload.Error, "secret") ||
		strings.Contains(payload.Error, "https://") {
		t.Fatalf("server diagnostics exposed raw error: %q", payload.Error)
	}
	if payload.Stage != "failed" || payload.DownloadReady {
		t.Fatalf("unexpected job state: %+v", payload)
	}
}
