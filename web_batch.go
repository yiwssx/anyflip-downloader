package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sort"
	"time"
)

const maxWebBatchJobs = 8

var (
	errWebQueueFull = errors.New("job queue is full")
	errWebShuttingDown = errors.New("server is shutting down")
)

type webJobSpec struct {
	URL string `json:"url"`
	Title string `json:"title"`
	Threads int `json:"threads"`
	BestQuality bool `json:"bestQuality"`
}

func validateWebJobSpec(spec *webJobSpec) error {
	if _, err := validateBookURL(spec.URL); err != nil {
		return errors.New("invalid AnyFlip book URL")
	}
	if spec.Threads == 0 {
		spec.Threads = 3
	}
	if spec.Threads < 1 || spec.Threads > 8 {
		return errors.New("threads must be between 1 and 8")
	}
	return nil
}

type queuedWebJob struct {
	job *webJob
	ctx context.Context
}

func (s *webServer) enqueueWebJobs(specs []webJobSpec) ([]webJobResponse, int, error) {
	if len(specs) == 0 || len(specs) > maxWebBatchJobs {
		return nil, http.StatusBadRequest, errors.New("batch requires 1 to 8 jobs")
	}
	// Validate every request before scheduling any work: invalid batch
	// members must not leave a partially accepted queue.
	for i := range specs {
		if err := validateWebJobSpec(&specs[i]); err != nil {
			return nil, http.StatusBadRequest, err
		}
	}
	queued := make([]queuedWebJob, 0, len(specs))
	cancelAll := func() {
		for _, q := range queued {
			q.job.cancel()
		}
	}
	for _, spec := range specs {
		var random [16]byte
		if _, err := rand.Read(random[:]); err != nil {
			cancelAll()
			return nil, http.StatusInternalServerError, errors.New("cannot create job")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
		job := &webJob{
			id: hex.EncodeToString(random[:]), stage: "queued",
			created: time.Now(), cancel: cancel, spec: spec,
		}
		queued = append(queued, queuedWebJob{job: job, ctx: ctx})
	}

	s.mu.Lock()
	if s.stopping {
		s.mu.Unlock()
		cancelAll()
		return nil, http.StatusServiceUnavailable, errWebShuttingDown
	}
	pending := 0
	for _, other := range s.jobs {
		if other.stage != "completed" && other.stage != "failed" && other.stage != "cancelled" {
			pending++
		}
	}
	if pending+len(queued) > 12 {
		s.mu.Unlock()
		cancelAll()
		return nil, http.StatusTooManyRequests, errWebQueueFull
	}
	response := make([]webJobResponse, 0, len(queued))
	for _, q := range queued {
		if _, exists := s.jobs[q.job.id]; exists {
			s.mu.Unlock()
			cancelAll()
			return nil, http.StatusInternalServerError, errors.New("job ID collision")
		}
	}
	for _, q := range queued {
		s.jobs[q.job.id] = q.job
		response = append(response, s.snapshot(q.job))
	}
	s.jobsWG.Add(len(queued))
	s.mu.Unlock()

	for _, q := range queued {
		q := q
		go func() {
			defer s.jobsWG.Done()
			spec := q.job.spec
			s.runJob(q.ctx, q.job, spec.URL, spec.Title, spec.Threads, spec.BestQuality)
		}()
	}
	return response, http.StatusAccepted, nil
}

func (s *webServer) createBatch(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Jobs []webJobSpec `json:"jobs"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 24<<10)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil {
		http.Error(w, "invalid batch request", http.StatusBadRequest)
		return
	}
	var extra json.RawMessage
	if err := decoder.Decode(&extra); err != io.EOF {
		http.Error(w, "invalid batch request", http.StatusBadRequest)
		return
	}
	jobs, status, err := s.enqueueWebJobs(body.Jobs)
	if err != nil {
		http.Error(w, err.Error(), status)
		return
	}
	sendJSON(w, http.StatusAccepted, struct {
		Jobs []webJobResponse `json:"jobs"`
	}{Jobs: jobs})
}

func (s *webServer) listJobs(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	list := make([]*webJob, 0, len(s.jobs))
	for _, job := range s.jobs {
		list = append(list, job)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].created.Equal(list[j].created) {
			return list[i].id > list[j].id
		}
		return list[i].created.After(list[j].created)
	})
	if len(list) > 50 {
		list = list[:50]
	}
	result := make([]webJobResponse, 0, len(list))
	for _, job := range list {
		result = append(result, s.snapshot(job))
	}
	s.mu.Unlock()
	sendJSON(w, http.StatusOK, struct {
		Jobs []webJobResponse `json:"jobs"`
	}{Jobs: result})
}

func (s *webServer) retryJob(w http.ResponseWriter, r *http.Request) {
	previous, ok := s.lookupJob(w, r)
	if !ok {
		return
	}
	s.mu.Lock()
	stage := previous.stage
	spec := previous.spec
	s.mu.Unlock()
	if stage != "failed" && stage != "cancelled" {
		http.Error(w, "only failed or cancelled jobs can be retried", http.StatusConflict)
		return
	}
	result, status, err := s.enqueueWebJobs([]webJobSpec{spec})
	if err != nil {
		http.Error(w, err.Error(), status)
		return
	}
	sendJSON(w, http.StatusAccepted, result[0])
}
