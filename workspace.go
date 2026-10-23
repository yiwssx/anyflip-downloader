package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// validateDownloadSettings rejects values that can hang conversion, create an
// unbounded number of workers, or cause excessive retries.
func validateDownloadSettings(threads, retries int, retryDelay time.Duration, chunkSize uint) error {
	if threads < 1 || threads > 16 {
		return fmt.Errorf("threads must be between 1 and 16 (got %d)", threads)
	}
	if retries < 0 || retries > 10 {
		return fmt.Errorf("retries must be between 0 and 10 (got %d)", retries)
	}
	if retryDelay < 0 || retryDelay > time.Minute {
		return fmt.Errorf("waitretry must be between 0 and 1m (got %s)", retryDelay)
	}
	if chunkSize < 1 || chunkSize > 1000 {
		return fmt.Errorf("chunksize must be between 1 and 1000 (got %d)", chunkSize)
	}
	return nil
}

// createDownloadWorkspace creates a new, exclusively owned directory.
// An explicitly selected folder MUST NOT already exist. Its cleanup closure
// checks that the directory has not been replaced before removing anything.
func createDownloadWorkspace(requested string) (string, func() error, error) {
	var dir string
	var err error
	if requested == "" {
		dir, err = os.MkdirTemp("", "anyflip-downloader-")
	} else {
		dir, err = filepath.Abs(requested)
		if err == nil {
			err = os.Mkdir(dir, 0700)
		}
	}
	if err != nil {
		return "", nil, fmt.Errorf("create temporary workspace (existing folders cannot be reused): %w", err)
	}

	createdInfo, err := os.Lstat(dir)
	if err != nil {
		_ = os.Remove(dir)
		return "", nil, fmt.Errorf("inspect temporary workspace: %w", err)
	}
	cleanup := func() error {
		currentInfo, err := os.Lstat(dir)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("inspect workspace before cleanup: %w", err)
		}
		if !os.SameFile(createdInfo, currentInfo) {
			return fmt.Errorf("refusing to remove replaced workspace %q", dir)
		}
		return os.RemoveAll(dir)
	}
	return dir, cleanup, nil
}
