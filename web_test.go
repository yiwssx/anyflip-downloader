package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"strings"
	"testing"
	"time"

	"github.com/pdfcpu/pdfcpu/pkg/api"
)

const testWebToken = "test-token-very-long-and-random"

func webRequest(t *testing.T, server *httptest.Server, method, path string, payload []byte, authorized bool) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, server.URL+path, bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	if authorized {
		req.Header.Set("Authorization", "Bearer "+testWebToken)
	}
	if len(payload) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestWebRequiresAuthentication(t *testing.T) {
	s, err := newWebServer(testWebToken, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(s.handler())
	defer ts.Close()
	for _, tc := range []struct{ method, path string }{
		{"POST", "/api/jobs"}, {"GET", "/api/jobs/00000000000000000000000000000000"},
	} {
		res := webRequest(t, ts, tc.method, tc.path, nil, false)
		res.Body.Close()
		if res.StatusCode != http.StatusUnauthorized {
			t.Fatalf("%s %s returned %d instead of 401", tc.method, tc.path, res.StatusCode)
		}
	}
	res := webRequest(t, ts, "GET", "/healthz", nil, false)
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("health check = %d", res.StatusCode)
	}
}

func TestWebJobLifecycleWithMockedImages(t *testing.T) {
	s, err := newWebServer(testWebToken, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s.client = jobFixtureClient(t)
	ts := httptest.NewServer(s.handler())
	defer ts.Close()
	payload := []byte(`{"url":"https://anyflip.com/qrfai/tfvn","threads":2}`)
	res := webRequest(t, ts, "POST", "/api/jobs", payload, true)
	if res.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(res.Body)
		t.Fatalf("job creation status=%d body=%s", res.StatusCode, body)
	}
	var created webJobResponse
	if err := json.NewDecoder(res.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if !jobIDPattern.MatchString(created.ID) {
		t.Fatalf("invalid job ID: %q", created.ID)
	}

	deadline := time.Now().Add(5 * time.Second)
	var job webJobResponse
	for {
		state := webRequest(t, ts, "GET", "/api/jobs/"+created.ID, nil, true)
		if err := json.NewDecoder(state.Body).Decode(&job); err != nil {
			t.Fatal(err)
		}
		state.Body.Close()
		if job.Stage == "completed" || job.Stage == "failed" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("job did not finish: %+v", job)
		}
		time.Sleep(15 * time.Millisecond)
	}
	if job.Stage != "completed" || !job.DownloadReady || job.Total != 2 || job.Completed != 2 {
		t.Fatalf("invalid completed job: %+v", job)
	}
	pdf := webRequest(t, ts, "GET", "/api/jobs/"+created.ID+"/download", nil, true)
	if pdf.StatusCode != http.StatusOK {
		t.Fatalf("PDF download status=%d", pdf.StatusCode)
	}
	b, err := io.ReadAll(pdf.Body)
	pdf.Body.Close()
	if err != nil || len(b) < 100 || !bytes.HasPrefix(b, []byte("%PDF-")) {
		t.Fatalf("invalid downloaded PDF bytes=%d error=%v", len(b), err)
	}
	// Only completed server-generated files may be deleted by the janitor.
	s.mu.Lock()
	s.jobs[created.ID].finished = time.Now().Add(-2 * time.Hour)
	s.mu.Unlock()
	s.purgeOldJobs(time.Now())
	after := webRequest(t, ts, "GET", "/api/jobs/"+created.ID, nil, true)
	after.Body.Close()
	if after.StatusCode != http.StatusNotFound {
		t.Fatalf("job remained accessible after expiry: %d", after.StatusCode)
	}
}

func TestWebRejectsInvalidJobRequests(t *testing.T) {
	s, err := newWebServer(testWebToken, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(s.handler())
	defer ts.Close()
	for _, body := range []string{
		`{"url":"https://evil.example/a/b","threads":2}`,
		`{"url":"https://anyflip.com/a/b","threads":15}`,
		`{"url":"https://anyflip.com/a/b","unknown":true}`,
	} {
		res := webRequest(t, ts, "POST", "/api/jobs", []byte(body), true)
		res.Body.Close()
		if res.StatusCode != http.StatusBadRequest {
			t.Fatalf("unexpected status %d for %s", res.StatusCode, body)
		}
	}
}

func TestWebDownloadUnavailableUntilReady(t *testing.T) {
	s, err := newWebServer(testWebToken, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var id = strings.Repeat("a", 32)
	s.jobs[id] = &webJob{id: id, stage: "downloading"}
	ts := httptest.NewServer(s.handler())
	defer ts.Close()
	res := webRequest(t, ts, "GET", "/api/jobs/"+id+"/download", nil, true)
	defer res.Body.Close()
	if res.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409, got %d", res.StatusCode)
	}
}

func TestWebRequiresStrongToken(t *testing.T) {
	if _, err := newWebServer("short", t.TempDir()); err == nil {
		t.Fatal("expected short token to be rejected")
	}
}

func TestWebBestQualityOptInUsesLargestPublisherListedImage(t *testing.T) {
	s, err := newWebServer(testWebToken, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const metadata = `{"title":"Web Quality","totalPageCount":1,"pages":[{"n":["small.png","large.png"]]}`
	small := qualityFixturePNG(t, 20, 20)
	large := qualityFixturePNG(t, 80, 40)
	var fetched atomic.Int32
	s.client = imageTestClient(func(req *http.Request) (*http.Response, error) {
		switch {
		case strings.HasSuffix(req.URL.Path, "/config.js"):
			return mockResponse(req, 200, metadata), nil
		case strings.HasSuffix(req.URL.Path, "/small.png"):
			fetched.Add(1)
			return responseBytes(req, 200, small), nil
		case strings.HasSuffix(req.URL.Path, "/large.png"):
			fetched.Add(1)
			return responseBytes(req, 200, large), nil
		default:
			t.Errorf("unexpected upstream URL: %s", req.URL.Redacted())
			return responseBytes(req, 404, nil), nil
		}
	})
	ts := httptest.NewServer(s.handler())
	defer ts.Close()
	html := webRequest(t, ts, "GET", "/", nil, false)
	htmlBytes, readErr := io.ReadAll(html.Body)
	html.Body.Close()
	if readErr != nil || !bytes.Contains(htmlBytes, []byte(`id="bestQuality"`)) {
		t.Fatalf("Web GUI quality checkbox not present: %v", readErr)
	}
	body := []byte(`{"url":"https://anyflip.com/qrfai/tfvn","threads":1,"bestQuality":true}`)
	createdResponse := webRequest(t, ts, "POST", "/api/jobs", body, true)
	if createdResponse.StatusCode != http.StatusAccepted {
		data, _ := io.ReadAll(createdResponse.Body)
		t.Fatalf("failed to create quality job: %d %s", createdResponse.StatusCode, data)
	}
	var created webJobResponse
	if err := json.NewDecoder(createdResponse.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	createdResponse.Body.Close()
	deadline := time.Now().Add(5 * time.Second)
	for {
		resp := webRequest(t, ts, "GET", "/api/jobs/"+created.ID, nil, true)
		var status webJobResponse
		err := json.NewDecoder(resp.Body).Decode(&status)
		resp.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if status.Stage == "completed" {
			if status.Total != 1 || status.Completed != 1 || !status.DownloadReady {
				t.Fatalf("invalid completed quality job: %+v", status)
			}
			break
		}
		if status.Stage == "failed" || status.Stage == "cancelled" {
			t.Fatalf("quality job failed: %+v", status)
		}
		if time.Now().After(deadline) {
			t.Fatalf("quality job timed out: %+v", status)
		}
		time.Sleep(15 * time.Millisecond)
	}
	if fetched.Load() != 2 {
		t.Fatalf("wanted both publisher-listed choices; got %d image requests", fetched.Load())
	}
	s.mu.Lock()
	output := s.jobs[created.ID].file
	s.mu.Unlock()
	dims, err := api.PageDimsFile(output)
	if err != nil || len(dims) != 1 {
		t.Fatalf("PDF missing or corrupt: dimensions=%v err=%v", dims, err)
	}
	if ratio := dims[0].Width / dims[0].Height; ratio < 1.9 || ratio > 2.1 {
		t.Fatalf("Web quality selection did not use largest 80x40 image; PDF page=%+v", dims[0])
	}
}
