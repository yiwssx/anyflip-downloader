package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestWebBatchRequiresAuthAndRejectsInvalidAtomically(t *testing.T) {
	s, err := newWebServer(testWebToken, t.TempDir())
	if err != nil { t.Fatal(err) }
	server := httptest.NewServer(s.handler())
	defer server.Close()
	body := []byte(`{"jobs":[{"url":"https://anyflip.com/qrfai/tfvn"},{"url":"https://example.org/not-allowed"}]}`)
	res := webRequest(t, server, "POST", "/api/jobs/batch", body, false)
	res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized { t.Fatalf("unauthenticated batch=%d", res.StatusCode) }
	res = webRequest(t, server, "POST", "/api/jobs/batch", body, true)
	res.Body.Close()
	if res.StatusCode != http.StatusBadRequest { t.Fatalf("invalid batch=%d", res.StatusCode) }
	s.mu.Lock()
	n := len(s.jobs)
	s.mu.Unlock()
	if n != 0 { t.Fatalf("invalid batch scheduled %d jobs", n) }
}

func TestWebBatchAcceptsTwoJobsAndHistorySafe(t *testing.T) {
	s, err := newWebServer(testWebToken, t.TempDir())
	if err != nil { t.Fatal(err) }
	s.client = jobFixtureClient(t)
	server := httptest.NewServer(s.handler())
	defer server.Close()
	body := []byte(`{"jobs":[{"url":"https://anyflip.com/qrfai/tfvn","threads":1},{"url":"https://anyflip.com/qrfai/tfvn","threads":2}]}`)
	res := webRequest(t, server, "POST", "/api/jobs/batch", body, true)
	if res.StatusCode != http.StatusAccepted {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("batch status=%d body=%s", res.StatusCode, b)
	}
	var payload struct { Jobs []webJobResponse `json:"jobs"` }
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil { t.Fatal(err) }
	res.Body.Close()
	if len(payload.Jobs) != 2 || payload.Jobs[0].ID == payload.Jobs[1].ID { t.Fatalf("bad batch IDs: %+v", payload) }
	done := make(chan struct{})
	go func() { s.jobsWG.Wait(); close(done) }()
	select { case <-done: case <-time.After(8*time.Second): t.Fatal("jobs did not finish") }
	history := webRequest(t, server, "GET", "/api/jobs", nil, true)
	if history.StatusCode != http.StatusOK { t.Fatalf("history=%d", history.StatusCode) }
	content, err := io.ReadAll(history.Body)
	history.Body.Close()
	if err != nil { t.Fatal(err) }
	if bytes.Contains(content, []byte("qrfai/tfvn")) || bytes.Contains(content, []byte(testWebToken)) {
		t.Fatalf("history leaked source or token: %s", content)
	}
	var listing struct { Jobs []webJobResponse `json:"jobs"` }
	if err := json.Unmarshal(content, &listing); err != nil { t.Fatal(err) }
	if len(listing.Jobs) != 2 { t.Fatalf("wanted 2 history entries: %s", content) }
	for _, j := range listing.Jobs {
		if j.Stage != "completed" || !j.DownloadReady { t.Fatalf("unexpected status: %+v", j) }
	}
}

func TestWebBatchQueueFullIsAtomic(t *testing.T) {
	s, err := newWebServer(testWebToken, t.TempDir())
	if err != nil { t.Fatal(err) }
	for i:=0;i<12;i++ {
		id := strings.Repeat(string("abcdef012345"[i]), 32)
		s.jobs[id] = &webJob{id: id, stage:"queued", created:time.Now()}
	}
	server := httptest.NewServer(s.handler())
	defer server.Close()
	body := []byte(`{"jobs":[{"url":"https://anyflip.com/qrfai/tfvn"},{"url":"https://anyflip.com/qrfai/tfvn"}]}`)
	res := webRequest(t, server, "POST", "/api/jobs/batch", body, true)
	res.Body.Close()
	if res.StatusCode != http.StatusTooManyRequests { t.Fatalf("full batch status=%d",res.StatusCode) }
	if len(s.jobs) != 12 { t.Fatalf("queue mutated: got %d jobs",len(s.jobs)) }
}

func TestWebRetryFailedJobUsesNewIDAndRejectsActive(t *testing.T) {
	s, err := newWebServer(testWebToken, t.TempDir())
	if err != nil { t.Fatal(err) }
	s.client = jobFixtureClient(t)
	id := strings.Repeat("a",32)
	s.jobs[id] = &webJob{
		id:id,stage:"downloading",created:time.Now(),
		spec:webJobSpec{URL:"https://anyflip.com/qrfai/tfvn",Threads:2},
	}
	server := httptest.NewServer(s.handler())
	defer server.Close()
	res := webRequest(t, server, "POST", "/api/jobs/"+id+"/retry", nil, true)
	res.Body.Close()
	if res.StatusCode != http.StatusConflict { t.Fatalf("active retry status=%d",res.StatusCode) }
	s.mu.Lock()
	s.jobs[id].stage="failed"
	s.mu.Unlock()
	res = webRequest(t, server, "POST", "/api/jobs/"+id+"/retry", nil, true)
	if res.StatusCode != http.StatusAccepted {
		b,_:=io.ReadAll(res.Body)
		t.Fatalf("failed retry status=%d body=%s", res.StatusCode,b)
	}
	var created webJobResponse
	if err := json.NewDecoder(res.Body).Decode(&created); err != nil { t.Fatal(err) }
	res.Body.Close()
	if created.ID == id || !jobIDPattern.MatchString(created.ID) { t.Fatalf("retry reused invalid ID %q",created.ID) }
	done:=make(chan struct{})
	go func(){s.jobsWG.Wait();close(done)}()
	select {case <-done:case <-time.After(8*time.Second):t.Fatal("retry did not finish")}
	s.mu.Lock()
	status:=s.jobs[created.ID].stage
	s.mu.Unlock()
	if status!="completed" {t.Fatalf("retried job stage=%s",status)}
}

func TestWebBatchTooLargeRejectedBeforeQueuing(t *testing.T) {
	s,err:=newWebServer(testWebToken,t.TempDir())
	if err!=nil{t.Fatal(err)}
	server:=httptest.NewServer(s.handler())
	defer server.Close()
	var jobs []webJobSpec
	for i:=0;i<maxWebBatchJobs+1;i++ {jobs=append(jobs,webJobSpec{URL:"https://anyflip.com/qrfai/tfvn"})}
	body,_:=json.Marshal(struct {Jobs []webJobSpec `json:"jobs"`}{Jobs:jobs})
	res:=webRequest(t,server,"POST","/api/jobs/batch",body,true)
	res.Body.Close()
	if res.StatusCode!=http.StatusBadRequest{t.Fatalf("large batch status=%d",res.StatusCode)}
	if len(s.jobs)!=0{t.Fatal("large batch left jobs")}
}
