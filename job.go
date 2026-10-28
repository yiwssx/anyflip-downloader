package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"time"
)

// DownloadJobOptions isolates every downloader invocation from CLI global flags.
// A web server must choose OutputFile and TempDownloadFolder itself rather than
// accepting arbitrary client-supplied filesystem paths.
type DownloadJobOptions struct {
	URL                string
	Title              string
	OutputFile         string
	TempDownloadFolder string
	KeepImages         bool
	Threads            int
	Retries            int
	RetryDelay         time.Duration
	ChunkSize          uint
	BestQuality        bool // inspect every publisher-listed candidate; default false
	MaxPages           int // web-only page count guard; 0 keeps CLI behavior
	MaxJobBytes        int64 // web-only checkpointed storage budget; 0 disables it
	HTTPClient         *http.Client
}

// DownloadProgress is independent of terminal-based progress indicators.
// Stage is preparing, downloading, converting, completed, failed or cancelled.
type DownloadProgress struct {
	Stage     string
	Completed int
	Total     int
	Page      int
}

// RunDownloadJob is a reusable entry point for the CLI and future job-based
// HTTP handler. The caller supplies a context to cancel network requests and
// PDF conversion between chunks. A callback may receive progress updates from
// the download workers; it must be nonblocking or quickly return.
func RunDownloadJob(ctx context.Context, opts DownloadJobOptions, report func(DownloadProgress)) (result string, runErr error) {
	emit := func(progress DownloadProgress) {
		if report != nil {
			report(progress)
		}
	}
	defer func() {
		if runErr != nil {
			stage := "failed"
			if errors.Is(runErr, context.Canceled) || errors.Is(runErr, context.DeadlineExceeded) {
				stage = "cancelled"
			}
			emit(DownloadProgress{Stage: stage})
		}
	}()
	if ctx == nil {
		return "", fmt.Errorf("download context is required")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := validateDownloadSettings(opts.Threads, opts.Retries, opts.RetryDelay, opts.ChunkSize); err != nil {
		return "", err
	}
	if opts.MaxPages < 0 || opts.MaxJobBytes < 0 {
		return "", errors.New("job resource bounds must not be negative")
	}
	bookURL, err := url.Parse(opts.URL)
	if err != nil {
		return "", fmt.Errorf("invalid book URL: %w", err)
	}
	emit(DownloadProgress{Stage: "preparing"})
	book, err := prepareDownloadContext(ctx, bookURL, opts.Title, opts.HTTPClient)
	if err != nil {
		return "", err
	}
	if opts.MaxPages > 0 && book.pageCount > opts.MaxPages {
		return "", fmt.Errorf("%w: %d pages exceeds %d", errJobPageLimitExceeded, book.pageCount, opts.MaxPages)
	}

	outputFile := opts.OutputFile
	if outputFile == "" {
		outputFile = book.title + ".pdf"
	}
	if _, err := os.Stat(outputFile); err == nil {
		return "", fmt.Errorf("output PDF %q already exists", outputFile)
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("inspect output PDF: %w", err)
	}

	workspace, cleanup, err := createDownloadWorkspace(opts.TempDownloadFolder)
	if err != nil {
		return "", err
	}
	// Quota failures delete this job's exclusively owned files immediately;
	// preserving an oversized failed workspace would defeat the storage guard.
	defer func() {
		if errors.Is(runErr, errJobStorageBudgetExceeded) {
			_ = cleanup()
		}
	}()
	// Failed jobs otherwise retain their isolated image workspace for
	// diagnostics. Successful jobs remove it unless KeepImages was requested.
	jobCtx, stopForBudget := context.WithCancelCause(ctx)
	defer stopForBudget(nil)
	emit(DownloadProgress{Stage: "downloading", Total: book.pageCount})
	var pageReporter func(page, done int)
	if report != nil || opts.MaxJobBytes > 0 {
		pageReporter = func(page, done int) {
			if err := checkJobStorageBudget(opts.MaxJobBytes, workspace); err != nil {
				stopForBudget(err)
			}
			emit(DownloadProgress{
				Stage: "downloading", Page: page, Completed: done, Total: book.pageCount,
			})
		}
	}
	if err := book.downloadImagesContextWithProgress(jobCtx, opts.HTTPClient, workspace,
		downloadOptions{threads: opts.Threads, retries: opts.Retries, retryDelay: opts.RetryDelay, bestQuality: opts.BestQuality}, pageReporter); err != nil {
		if cause := context.Cause(jobCtx); cause != nil && !errors.Is(cause, context.Canceled) && !errors.Is(cause, context.DeadlineExceeded) {
			return "", fmt.Errorf("image job budget check: %w", cause)
		}
		return "", fmt.Errorf("download images (workspace %s): %w", workspace, err)
	}
	if err := checkJobStorageBudget(opts.MaxJobBytes, workspace); err != nil {
		return "", err
	}
	emit(DownloadProgress{Stage: "converting", Total: book.pageCount})
	if err := createPDFVerifiedWithBudget(jobCtx, outputFile, workspace,
		book.pageCount, int(opts.ChunkSize), func(done int) {
			emit(DownloadProgress{Stage: "converting", Completed: done, Total: book.pageCount})
		}, opts.MaxJobBytes); err != nil {
		return "", fmt.Errorf("convert downloaded images to PDF (workspace %s): %w", workspace, err)
	}
	if !opts.KeepImages {
		if err := cleanup(); err != nil {
			// Retained files are not a failed conversion; do not misrepresent
			// an already-published, validated PDF as missing.
			fmt.Fprintf(os.Stderr, "warning: cannot clean temporary workspace: %v\n", err)
		}
	}
	emit(DownloadProgress{Stage: "completed", Completed: book.pageCount, Total: book.pageCount})
	return outputFile, nil
}
