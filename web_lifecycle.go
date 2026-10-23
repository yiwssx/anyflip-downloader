package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// newWebHTTPServer configures network limits without limiting the (separate)
// 20-minute per-job execution context. Long PDF downloads have a bounded
// WriteTimeout and are never exposed on a public listener by default.
func newWebHTTPServer(addr string, web *webServer) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           web.handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       20 * time.Second,
		WriteTimeout:      30 * time.Minute,
		IdleTimeout:       75 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}
}

// beginShutdown is synchronized with job creation. No WaitGroup.Add calls can
// occur after this method returns, making jobsWG.Wait safe during shutdown.
func (s *webServer) beginShutdown() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopping = true
	for _, job := range s.jobs {
		if job.cancel != nil && job.finished.IsZero() {
			job.cancel()
		}
	}
}

// serveWebUntilContext is separated from OS signals so shutdown can be
// simulated with an injected context in tests.
func serveWebUntilContext(ctx context.Context, addr string, web *webServer) error {
	server := newWebHTTPServer(addr, web)
	serverErr := make(chan error, 1)

	janitorCtx, stopJanitor := context.WithCancel(context.Background())
	defer stopJanitor()
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-janitorCtx.Done():
				return
			case now := <-ticker.C:
				web.purgeOldJobs(now)
			}
		}
	}()

	go func() {
		serverErr <- server.ListenAndServe()
	}()
	select {
	case err := <-serverErr:
		web.beginShutdown()
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		web.beginShutdown()
		shutdownCtx, stop := context.WithTimeout(context.Background(), 25*time.Second)
		defer stop()
		err := server.Shutdown(shutdownCtx)
		if err != nil {
			log.Printf("HTTP shutdown exceeded deadline: %v", err)
		}

		drained := make(chan struct{})
		go func() {
			web.jobsWG.Wait()
			close(drained)
		}()
		select {
		case <-drained:
		case <-shutdownCtx.Done():
			log.Printf("download workers did not finish before shutdown deadline")
			if err == nil {
				err = shutdownCtx.Err()
			}
		}
		return err
	}
}

func serveWebUntilSignal(addr string, web *webServer) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	log.Printf("Web GUI listening at %s (token authentication required)", addr)
	return serveWebUntilContext(ctx, addr, web)
}
