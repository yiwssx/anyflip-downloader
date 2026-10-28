package main

import (
	"context"
	"crypto/subtle"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

//go:embed web/index.html
var webUI embed.FS

var jobIDPattern = regexp.MustCompile(`^[a-f0-9]{32}$`)

type webJob struct {
	id        string
	stage     string
	completed int
	total     int
	errorText string
	errorCode string
	file      string
	created   time.Time
	finished  time.Time
	cancel    context.CancelFunc
	spec      webJobSpec // in-memory only; never serialized in API output
}

type webJobResponse struct {
	ID            string `json:"id"`
	Stage         string `json:"stage"`
	Completed     int    `json:"completed"`
	Total         int    `json:"total"`
	Error         string `json:"error,omitempty"`
	ErrorCode     string `json:"errorCode,omitempty"`
	DownloadReady bool   `json:"downloadReady"`
	Archived      bool   `json:"archived,omitempty"`
}

type webServer struct {
	mu       sync.Mutex
	jobs     map[string]*webJob
	history  []archivedWebJob
	jobsWG   sync.WaitGroup
	stopping bool
	root     string
	token    string
	sem      chan struct{}
	client   *http.Client // Optional controlled test transport; nil in production.
	authThrottle *authThrottle
	activeReservations int // running Web jobs with reserved volume headroom
}

func newWebServer(token, root string) (*webServer, error) {
	if len(token) < 16 {
		return nil, fmt.Errorf("ANYFLIP_WEB_TOKEN must be at least 16 characters")
	}
	if root == "" {
		return nil, errors.New("web job directory is required")
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	server := &webServer{jobs: make(map[string]*webJob), root: root, token: token, sem: make(chan struct{}, 2), authThrottle: newAuthThrottle()}
	if err := server.loadWebHistory(time.Now()); err != nil {
		return nil, err
	}
	if err := server.removeExpiredJobArtifacts(time.Now()); err != nil {
		return nil, err
	}
	return server, nil
}

func (s *webServer) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = io.WriteString(w, "ok")
	})
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		b, err := webUI.ReadFile("web/index.html")
		if err != nil {
			http.Error(w, "web UI unavailable", 500)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(b)
	})
	mux.Handle("POST /api/jobs", s.authorize(http.HandlerFunc(s.createJob)))
	mux.Handle("GET /api/jobs", s.authorize(http.HandlerFunc(s.listJobs)))
	mux.Handle("POST /api/jobs/batch", s.authorize(http.HandlerFunc(s.createBatch)))
	mux.Handle("POST /api/jobs/{id}/retry", s.authorize(http.HandlerFunc(s.retryJob)))
	mux.Handle("GET /api/jobs/{id}", s.authorize(http.HandlerFunc(s.getJob)))
	mux.Handle("POST /api/jobs/{id}/cancel", s.authorize(http.HandlerFunc(s.cancelJob)))
	mux.Handle("GET /api/jobs/{id}/download", s.authorize(http.HandlerFunc(s.downloadJob)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; connect-src 'self'; frame-ancestors 'none'")
		mux.ServeHTTP(w, r)
	})
}

func (s *webServer) authorize(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if wait := s.authThrottle.retryAfter(r.RemoteAddr); wait > 0 {
			seconds := int((wait + time.Second - 1) / time.Second)
			w.Header().Set("Retry-After", strconv.Itoa(seconds))
			http.Error(w, "too many failed authentication attempts", http.StatusTooManyRequests)
			return
		}
		value := r.Header.Get("Authorization")
		if !strings.HasPrefix(value, "Bearer ") ||
			subtle.ConstantTimeCompare([]byte(strings.TrimPrefix(value, "Bearer ")), []byte(s.token)) != 1 {
			s.authThrottle.recordFailure(r.RemoteAddr)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		s.authThrottle.recordSuccess(r.RemoteAddr)
		next.ServeHTTP(w, r)
	})
}

func (s *webServer) snapshot(job *webJob) webJobResponse {
	return webJobResponse{
		ID: job.id, Stage: job.stage, Completed: job.completed, Total: job.total,
		Error: job.errorText, ErrorCode: job.errorCode,
		DownloadReady: job.stage == "completed",
	}
}

func sendJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func (s *webServer) lookupJob(w http.ResponseWriter, r *http.Request) (*webJob, bool) {
	id := r.PathValue("id")
	if !jobIDPattern.MatchString(id) {
		http.NotFound(w, r)
		return nil, false
	}
	s.mu.Lock()
	job, ok := s.jobs[id]
	s.mu.Unlock()
	if !ok {
		http.NotFound(w, r)
		return nil, false
	}
	return job, true
}

func (s *webServer) createJob(w http.ResponseWriter, r *http.Request) {
	var body webJobSpec
	r.Body = http.MaxBytesReader(w, r.Body, 8192)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	var extra json.RawMessage
	if err := decoder.Decode(&extra); err != io.EOF {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	jobs, status, err := s.enqueueWebJobs([]webJobSpec{body})
	if err != nil {
		http.Error(w, err.Error(), status)
		return
	}
	sendJSON(w, http.StatusAccepted, jobs[0])
}

func (s *webServer) runJob(ctx context.Context, job *webJob, url, title string, threads int, bestQuality bool) {
	defer job.cancel()
	select {
	case s.sem <- struct{}{}:
		defer func() { <-s.sem }()
	case <-ctx.Done():
		s.finishJob(job, "", ctx.Err())
		return
	}
	// Admission is serialized against other running Web jobs. Do not create
	// the image workspace or fetch source pages if the host is short on space.
	if err := s.reserveWebCapacity(); err != nil {
		s.finishJob(job, "", err)
		return
	}
	defer s.releaseWebCapacity()
	output := filepath.Join(s.root, job.id+".pdf")
	workspace := filepath.Join(s.root, job.id+".images")
	result, err := RunDownloadJob(ctx, DownloadJobOptions{
		URL: url, Title: title, OutputFile: output, TempDownloadFolder: workspace,
		Threads: threads, Retries: 1, RetryDelay: time.Second, ChunkSize: 10,
		BestQuality: bestQuality,
		MaxPages: webMaxPages, MaxJobBytes: webMaxJobBytes,
		HTTPClient: s.client,
	}, func(event DownloadProgress) {
		s.mu.Lock()
		job.stage = event.Stage
		job.completed = event.Completed
		job.total = event.Total
		s.mu.Unlock()
	})
	s.finishJob(job, result, err)
}

func (s *webServer) finishJob(job *webJob, file string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job.finished = time.Now()
	if err != nil {
		job.stage = "failed"
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			job.stage = "cancelled"
		}
		job.errorCode, job.errorText = describeWebJobError(err)
		// Errors can embed signed source URLs, bearer-like query values,
		// private workspace paths and upstream transport details. Record
		// only bounded, generated identifiers and classified status.
		log.Printf("job %s finished (stage=%s code=%s)", job.id, job.stage, job.errorCode)
		s.archiveFinishedJobLocked(job)
		return
	}
	job.stage = "completed"
	job.file = file
	job.completed = job.total
	s.archiveFinishedJobLocked(job)
}

func (s *webServer) getJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !jobIDPattern.MatchString(id) {
		http.NotFound(w, r)
		return
	}
	s.mu.Lock()
	job, ok := s.jobs[id]
	var value webJobResponse
	if ok {
		value = s.snapshot(job)
	} else {
		value, ok = s.archivedSnapshotLocked(id)
	}
	s.mu.Unlock()
	if !ok {
		http.NotFound(w, r)
		return
	}
	sendJSON(w, http.StatusOK, value)
}

func (s *webServer) cancelJob(w http.ResponseWriter, r *http.Request) {
	job, ok := s.lookupJob(w, r)
	if !ok {
		return
	}
	s.mu.Lock()
	if job.stage != "completed" && job.stage != "failed" && job.stage != "cancelled" {
		job.cancel()
	}
	value := s.snapshot(job)
	s.mu.Unlock()
	sendJSON(w, http.StatusAccepted, value)
}

func (s *webServer) downloadJob(w http.ResponseWriter, r *http.Request) {
	job, ok := s.lookupJob(w, r)
	if !ok {
		return
	}
	s.mu.Lock()
	file := job.file
	ready := job.stage == "completed"
	s.mu.Unlock()
	if !ready || file == "" {
		http.Error(w, "PDF is not ready", http.StatusConflict)
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	// Explicit machine-readable artifact type for API clients.
	w.Header().Set("X-AnyFlip-Output-Type", "image-based-pdf")
	w.Header().Set("Content-Disposition", "attachment; filename=\"anyflip-"+job.id+".pdf\"")
	http.ServeFile(w, r, file)
}

func runWeb(addr string) error {
	token := os.Getenv("ANYFLIP_WEB_TOKEN")
	root := os.Getenv("ANYFLIP_JOB_DIR")
	if root == "" {
		root = ".anyflip-web-jobs"
	}
	server, err := newWebServer(token, root)
	if err != nil {
		return err
	}
	return serveWebUntilSignal(addr, server)
}
