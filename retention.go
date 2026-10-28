package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const jobRetention = time.Hour

// jobArtifactID matches only files managed by this application. Arbitrary
// user-created files in the job volume must never be considered for deletion.
func jobArtifactID(name string) (id, suffix string, ok bool) {
	for _, suffix = range []string{".pdf", ".images"} {
		if strings.HasSuffix(name, suffix) {
			id = strings.TrimSuffix(name, suffix)
			return id, suffix, jobIDPattern.MatchString(id)
		}
	}
	return "", "", false
}

// removeExpiredJobArtifacts deals with interrupted processes and restarts.
// Files are eligible for deletion only when their job ID is not currently
// tracked AND their filesystem mtime is older than the retention period.
// os.Lstat and os.RemoveAll never follow a symlink to its target.
func (s *webServer) removeExpiredJobArtifacts(now time.Time) error {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return fmt.Errorf("scan managed job volume: %w", err)
	}
	for _, entry := range entries {
		id, suffix, ok := jobArtifactID(entry.Name())
		if !ok {
			continue
		}
		s.mu.Lock()
		_, tracked := s.jobs[id]
		s.mu.Unlock()
		if tracked {
			continue
		}
		filename := filepath.Join(s.root, entry.Name())
		info, err := os.Lstat(filename)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("inspect old job artifact: %w", err)
		}
		if now.Sub(info.ModTime()) < jobRetention {
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 {
			// Removing the link is safe; never follow it.
			err = os.Remove(filename)
		} else if suffix == ".pdf" && info.Mode().IsRegular() {
			err = os.Remove(filename)
		} else if suffix == ".images" && info.IsDir() {
			err = os.RemoveAll(filename)
		} else {
			// Unexpected entry type: preserve it rather than taking risks.
			continue
		}
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove old job artifact %q: %w", entry.Name(), err)
		}
	}
	return nil
}

// purgeOldJobs drops expired in-memory jobs, then prunes orphaned volume files.
// Never removes a job that is actively running, even if its images are old.
func (s *webServer) purgeOldJobs(now time.Time) {
	s.mu.Lock()
	for id, job := range s.jobs {
		if job.finished.IsZero() || now.Sub(job.finished) < jobRetention {
			continue
		}
		delete(s.jobs, id)
	}
	// History is metadata-only and has a longer retention period than files.
	// Prune it independently, including when the server stays up for days.
	kept := s.history[:0]
	for _, entry := range s.history {
		if now.Sub(entry.Finished) <= webHistoryMaxAge {
			kept = append(kept, entry)
		}
	}
	if len(kept) != len(s.history) {
		s.history = kept
		if err := s.persistWebHistoryLocked(); err != nil {
			log.Printf("job history retention write failed")
		}
	}
	s.mu.Unlock()
	if err := s.removeExpiredJobArtifacts(now); err != nil {
		log.Printf("job retention cleanup failed: %v", err)
	}
}
