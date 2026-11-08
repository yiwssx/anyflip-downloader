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
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

//go:embed web/index.html web/tokens.css web/style.css web/readability.css web/admin.css web/workspace.js web/app.js web/google.html web/google.js web/admin-api.js web/admin.js web/favicon.svg web/logo.svg
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
	cancelRequested bool // protected by webServer.mu; cancellation is idempotent
	spec      webJobSpec // in-memory only; never serialized in API output
	owner     string // stable hashed Google subject; empty for legacy jobs
}

type webJobResponse struct {
	ID            string `json:"id"`
	Owner         string `json:"owner,omitempty"`
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
	google   *googleAuth // nil for backward-compatible token-only mode
	sem      chan struct{}
	client   *http.Client // Optional controlled test transport; nil in production.
	authThrottle *authThrottle
	activeReservations int // running Web jobs with reserved volume headroom
	rbacLimiter rbacRateLimiter
}

func newWebServer(token, root string) (*webServer, error) {
	return newWebServerForAuthMode(token, root, "token")
}

// Google Workspace mode intentionally requires no legacy shared Bearer token.
// Token mode still fails closed without a sufficiently strong token.
func newWebServerForAuthMode(token, root, mode string) (*webServer, error) {
	if mode != "token" && mode != "google" {
		return nil, fmt.Errorf("unsupported ANYFLIP_AUTH_MODE %q", mode)
	}
	if mode == "token" && len(token) < 16 {
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
		if s.google != nil {
			// Serve interactive SSO only on the canonical HTTPS hostname.
			// The alternate Tunnel hostname remains available for /healthz.
			canonical, err := url.Parse(s.google.redirectURI)
			if err != nil || canonical.Host == "" {
				http.Error(w, "SSO not configured", http.StatusServiceUnavailable)
				return
			}
			if r.Host != canonical.Host {
				http.Redirect(w, r, canonical.Scheme+"://"+canonical.Host+"/", http.StatusSeeOther)
				return
			}
			serveEmbeddedWebAsset(w, "web/google.html", "text/html; charset=utf-8")
		} else {
			serveEmbeddedWebAsset(w, "web/index.html", "text/html; charset=utf-8")
		}
	})
	mux.HandleFunc("GET /favicon.svg", func(w http.ResponseWriter, _ *http.Request) {
		serveEmbeddedWebAsset(w, "web/favicon.svg", "image/svg+xml")
	})
	mux.HandleFunc("GET /logo.svg", func(w http.ResponseWriter, _ *http.Request) {
		serveEmbeddedWebAsset(w, "web/logo.svg", "image/svg+xml")
	})
	mux.HandleFunc("GET /tokens.css", func(w http.ResponseWriter, _ *http.Request) {
		serveEmbeddedWebAsset(w, "web/tokens.css", "text/css; charset=utf-8")
	})
	mux.HandleFunc("GET /style.css", func(w http.ResponseWriter, _ *http.Request) {
		serveEmbeddedWebAsset(w, "web/style.css", "text/css; charset=utf-8")
	})
	mux.HandleFunc("GET /readability.css", func(w http.ResponseWriter, _ *http.Request) {
		serveEmbeddedWebAsset(w, "web/readability.css", "text/css; charset=utf-8")
	})
	mux.HandleFunc("GET /workspace.js", func(w http.ResponseWriter, _ *http.Request) {
		serveEmbeddedWebAsset(w, "web/workspace.js", "text/javascript; charset=utf-8")
	})
	mux.HandleFunc("GET /app.js", func(w http.ResponseWriter, _ *http.Request) {
		if s.google != nil {
			serveEmbeddedWebAsset(w, "web/google.js", "text/javascript; charset=utf-8")
		} else {
			serveEmbeddedWebAsset(w, "web/app.js", "text/javascript; charset=utf-8")
		}
	})
	if s.google != nil {
        // The asset itself is public; RBAC still protects all admin data APIs.
        // Serve in Google mode even when the role feature is disabled so the
        // bundled SSO page never requests a missing script.
        mux.HandleFunc("GET /admin.css",func(w http.ResponseWriter,_ *http.Request) {
            serveEmbeddedWebAsset(w,"web/admin.css","text/css; charset=utf-8")
        })
        mux.HandleFunc("GET /admin-api.js",func(w http.ResponseWriter,_ *http.Request) {
            serveEmbeddedWebAsset(w,"web/admin-api.js","text/javascript; charset=utf-8")
        })
        mux.HandleFunc("GET /admin.js",func(w http.ResponseWriter,_ *http.Request) {
            serveEmbeddedWebAsset(w,"web/admin.js","text/javascript; charset=utf-8")
        })
		mux.HandleFunc("GET /auth/google", s.google.start)
		mux.HandleFunc("GET /auth/google/callback", s.google.callback)
		mux.HandleFunc("GET /auth/me", s.google.me)
		mux.HandleFunc("POST /auth/logout", s.google.logout)
	}
	mux.Handle("POST /api/jobs", s.authorize(http.HandlerFunc(s.createJob)))
	mux.Handle("GET /api/jobs", s.authorize(http.HandlerFunc(s.listJobs)))
	mux.Handle("POST /api/jobs/batch", s.authorize(http.HandlerFunc(s.createBatch)))
	mux.Handle("POST /api/jobs/{id}/retry", s.authorize(http.HandlerFunc(s.retryJob)))
	mux.Handle("GET /api/jobs/{id}", s.authorize(http.HandlerFunc(s.getJob)))
	mux.Handle("POST /api/jobs/{id}/cancel", s.authorize(http.HandlerFunc(s.cancelJob)))
	mux.Handle("GET /api/jobs/{id}/download", s.authorize(http.HandlerFunc(s.downloadJob)))
	// Admin APIs are registered only in opt-in Google RBAC mode.
	// Authentication AND authorization are enforced server-side, never by UI.
	if s.google!=nil && s.google.rbac!=nil {
		mux.Handle("GET /api/admin/summary",s.authorize(http.HandlerFunc(s.adminSummary)))
		mux.Handle("GET /api/admin/jobs",s.authorize(http.HandlerFunc(s.adminListJobs)))
		mux.Handle("POST /api/admin/jobs/{id}/cancel",s.authorize(http.HandlerFunc(s.adminCancelJob)))
		mux.Handle("GET /api/admin/users",s.authorize(http.HandlerFunc(s.adminListUsers)))
		mux.Handle("PUT /api/admin/users/{id}/role",s.authorize(http.HandlerFunc(s.adminSetRole)))
		mux.Handle("GET /api/admin/audit",s.authorize(http.HandlerFunc(s.adminAudit)))
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; base-uri 'none'; form-action 'self'; img-src 'self' data:; script-src 'self'; style-src 'self'; connect-src 'self'; frame-ancestors 'none'")
		mux.ServeHTTP(w, r)
	})
}

// serveEmbeddedWebAsset serves only constant application-owned paths. It
// does not take filenames from requests, expose disk paths or read a writable
// host directory; all UI resources are compiled into the same Go binary.
func serveEmbeddedWebAsset(w http.ResponseWriter, name, contentType string) {
	data, err := webUI.ReadFile(name)
	if err != nil {
		http.Error(w, "web UI unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", contentType)
	_, _ = w.Write(data)
}

func (s *webServer) authorize(next http.Handler) http.Handler {
	if s.google != nil { return s.authorizeGoogle(next) }
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
		ID: job.id, Owner: job.owner, Stage: job.stage, Completed: job.completed, Total: job.total,
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
	if !ok || (s.google != nil && !mayAccessOwner(r, job.owner)) {
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
	jobs, status, err := s.enqueueWebJobsForOwner([]webJobSpec{body}, principalOwner(r))
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
	if !ok || (s.google != nil && !mayAccessOwner(r, value.Owner)) {
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
	if job.stage != "completed" && job.stage != "failed" && job.stage != "cancelled" && !job.cancelRequested {
		job.cancelRequested = true
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
	mode := os.Getenv("ANYFLIP_AUTH_MODE")
	if mode == "" {
		mode = "token"
	}
	var google *googleAuth
	if mode == "google" {
		var err error
		google, err = newGoogleAuth(
			os.Getenv("GOOGLE_CLIENT_ID"), os.Getenv("GOOGLE_CLIENT_SECRET"),
			os.Getenv("GOOGLE_REDIRECT_URI"))
		if err != nil {
			return err
		}
	}
	server, err := newWebServerForAuthMode(token, root, mode)
	if err != nil {
		return err
	}
	if mode=="google" && (os.Getenv("ANYFLIP_RBAC_ENABLED")=="1" || strings.EqualFold(os.Getenv("ANYFLIP_RBAC_ENABLED"),"true")) {
		roles,err:=openRBACStore(root)
		if err!=nil {return fmt.Errorf("RBAC enabled but private role store failed validation: %w",err)}
		google.rbac=roles
	}
	server.google = google
	return serveWebUntilSignal(addr, server)
}
