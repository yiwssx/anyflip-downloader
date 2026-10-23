package main

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
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
	file      string
	created   time.Time
	finished  time.Time
	cancel    context.CancelFunc
}

type webJobResponse struct {
	ID            string `json:"id"`
	Stage         string `json:"stage"`
	Completed     int    `json:"completed"`
	Total         int    `json:"total"`
	Error         string `json:"error,omitempty"`
	DownloadReady bool   `json:"downloadReady"`
}

type webServer struct {
	mu     sync.Mutex
	jobs   map[string]*webJob
	root   string
	token  string
	sem    chan struct{}
	client *http.Client // Optional controlled test transport; nil in production.
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
	return &webServer{jobs: make(map[string]*webJob), root: root, token: token, sem: make(chan struct{}, 2)}, nil
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
		value := r.Header.Get("Authorization")
		if !strings.HasPrefix(value, "Bearer ") ||
			subtle.ConstantTimeCompare([]byte(strings.TrimPrefix(value, "Bearer ")), []byte(s.token)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *webServer) snapshot(job *webJob) webJobResponse {
	return webJobResponse{
		ID: job.id, Stage: job.stage, Completed: job.completed, Total: job.total,
		Error: job.errorText, DownloadReady: job.stage == "completed",
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
	var body struct {
		URL     string `json:"url"`
		Title   string `json:"title"`
		Threads int    `json:"threads"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8192)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	if _, err := validateBookURL(body.URL); err != nil {
		http.Error(w, "invalid AnyFlip book URL", http.StatusBadRequest)
		return
	}
	if body.Threads == 0 {
		body.Threads = 3
	}
	if body.Threads < 1 || body.Threads > 8 {
		http.Error(w, "threads must be between 1 and 8", http.StatusBadRequest)
		return
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		http.Error(w, "cannot create job", http.StatusInternalServerError)
		return
	}
	id := hex.EncodeToString(random[:])
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	job := &webJob{id: id, stage: "queued", created: time.Now(), cancel: cancel}
	s.mu.Lock()
	pending := 0
	for _, other := range s.jobs {
		if other.stage != "completed" && other.stage != "failed" && other.stage != "cancelled" {
			pending++
		}
	}
	if pending >= 12 {
		s.mu.Unlock()
		cancel()
		http.Error(w, "job queue is full", http.StatusTooManyRequests)
		return
	}
	s.jobs[id] = job
	response := s.snapshot(job)
	s.mu.Unlock()
	go s.runJob(ctx, job, body.URL, body.Title, body.Threads)
	sendJSON(w, http.StatusAccepted, response)
}

func (s *webServer) runJob(ctx context.Context, job *webJob, url, title string, threads int) {
	defer job.cancel()
	select {
	case s.sem <- struct{}{}:
		defer func() { <-s.sem }()
	case <-ctx.Done():
		s.finishJob(job, "", ctx.Err())
		return
	}
	output := filepath.Join(s.root, job.id+".pdf")
	workspace := filepath.Join(s.root, job.id+".images")
	result, err := RunDownloadJob(ctx, DownloadJobOptions{
		URL: url, Title: title, OutputFile: output, TempDownloadFolder: workspace,
		Threads: threads, Retries: 1, RetryDelay: time.Second, ChunkSize: 10,
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
		job.errorText = "ไม่สามารถดาวน์โหลดหรือสร้าง PDF ได้ กรุณาตรวจสอบสิทธิ์การเข้าถึงเอกสารและลองอีกครั้ง"
		log.Printf("job %s failed: %v", job.id, err)
		return
	}
	job.stage = "completed"
	job.file = file
	job.completed = job.total
}

func (s *webServer) getJob(w http.ResponseWriter, r *http.Request) {
	job, ok := s.lookupJob(w, r)
	if !ok {
		return
	}
	s.mu.Lock()
	value := s.snapshot(job)
	s.mu.Unlock()
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
	w.Header().Set("Content-Disposition", "attachment; filename=\"anyflip-"+job.id+".pdf\"")
	http.ServeFile(w, r, file)
}

// purgeOldJobs removes only server-generated job-id files from this job root.
func (s *webServer) purgeOldJobs(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, job := range s.jobs {
		if job.finished.IsZero() || now.Sub(job.finished) < time.Hour {
			continue
		}
		delete(s.jobs, id)
		_ = os.Remove(filepath.Join(s.root, id+".pdf"))
		_ = os.RemoveAll(filepath.Join(s.root, id+".images"))
	}
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
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for now := range ticker.C {
			server.purgeOldJobs(now)
		}
	}()
	log.Printf("Web GUI listening at %s (token authentication required)", addr)
	return http.ListenAndServe(addr, server.handler())
}
