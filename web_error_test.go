package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
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
		{"oversized decoded image", errors.New("image dimensions exceed supported limit (12000 px per side, 40000000 total pixels)"), "image_invalid"},
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

func TestWebJobLogsContainCodesButNeverRawErrorDetails(t *testing.T) {
	// Capture process logs only within the synchronous finishJob call.
	// Never use t.Parallel: the standard logger is global.
	s, err := newWebServer(testWebToken, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for i, tc := range []struct {
		rawError string
		wantCode string
		wantStage string
		secret   []string
	}{
		{
			rawError: "fetch source https://online.anyflip.com/abcd/efgh/files/mobile/page.jpg?token=hidden-api-key; workspace /mnt/private/jobs/abc; 403 Forbidden",
			wantCode: "source_access_denied",
			wantStage: "failed",
			secret: []string{"https://", "hidden-api-key", "/mnt/private", "page.jpg", "workspace"},
		},
		{
			rawError: "create PDF staging folder: permission denied at /srv/secretuser/output and password=private-123",
			wantCode: "storage_error",
			wantStage: "failed",
			secret: []string{"/srv/secretuser", "password=", "private-123"},
		},
		{
			rawError: "lookup failed for customer-secret.example: no such host; credential=abc123",
			wantCode: "network_error",
			wantStage: "failed",
			secret: []string{"customer-secret.example", "credential=", "abc123"},
		},
	} {
		t.Run(fmt.Sprintf("classified-%d", i), func(t *testing.T) {
			job := &webJob{id: strings.Repeat("a", 32), stage: "downloading"}
			var output bytes.Buffer
			previous := log.Writer()
			log.SetOutput(&output)
			// Restore immediately even if finishJob starts returning early.
			func() {
				defer log.SetOutput(previous)
				s.finishJob(job, "", errors.New(tc.rawError))
			}()
			got := output.String()
			if !strings.Contains(got, "job "+job.id) ||
				!strings.Contains(got, "stage="+tc.wantStage) ||
				!strings.Contains(got, "code="+tc.wantCode) {
				t.Fatalf("log lacks safe correlation fields: %q", got)
			}
			for _, secret := range tc.secret {
				if strings.Contains(got, secret) {
					t.Fatalf("job log leaks %q: %q", secret, got)
				}
			}
			if job.errorCode != tc.wantCode || job.errorText == "" {
				t.Fatalf("user-visible safe classification lost: %+v", job)
			}
		})
	}

	t.Run("cancellation-logs-no-wrapped-details", func(t *testing.T) {
		job := &webJob{id: strings.Repeat("b", 32), stage: "preparing"}
		var output bytes.Buffer
		previous := log.Writer()
		log.SetOutput(&output)
		func() {
			defer log.SetOutput(previous)
			s.finishJob(job, "", fmt.Errorf("secret source /mnt/private?token=avoid-me: %w", context.Canceled))
		}()
		got := output.String()
		if job.stage != "cancelled" || job.errorCode != "job_cancelled" ||
			!strings.Contains(got, "stage=cancelled") || !strings.Contains(got, "code=job_cancelled") {
			t.Fatalf("cancelled status not safely logged: %q, %+v", got, job)
		}
		if strings.Contains(got, "avoid-me") || strings.Contains(got, "/mnt/private") {
			t.Fatalf("cancelled error details leaked: %q", got)
		}
	})
}
