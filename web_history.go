package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"time"
)

const (
	webHistoryFilename = ".anyflip-history.json"
	webHistoryMaxEntries = 50
	webHistoryMaxAge = 7 * 24 * time.Hour
	webHistoryMaxFileBytes = 128 << 10
)

// Archived records deliberately contain only redacted public API status.
// In particular, the source URL, title, token, file path and retry settings
// are never written to disk. Artifacts have their own one-hour retention.
type archivedWebJob struct {
	Response webJobResponse `json:"response"`
	Finished time.Time `json:"finished"`
}

type archivedWebHistory struct {
	Version int `json:"version"`
	Entries []archivedWebJob `json:"entries"`
}

func (s *webServer) historyPath() string {
	return filepath.Join(s.root, webHistoryFilename)
}

func validArchivedJob(item archivedWebJob) bool {
	if !jobIDPattern.MatchString(item.Response.ID) ||
		item.Finished.IsZero() ||
		item.Response.Completed < 0 || item.Response.Total < 0 ||
		item.Response.Completed > item.Response.Total ||
		len(item.Response.Error) > 512 || len(item.Response.ErrorCode) > 80 {
		return false
	}
	switch item.Response.Stage {
	case "completed", "failed", "cancelled":
		return !item.Response.DownloadReady && !item.Response.Archived
	default:
		return false
	}
}

func (s *webServer) loadWebHistory(now time.Time) error {
	path := s.historyPath()
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect job history: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > webHistoryMaxFileBytes {
		return errors.New("unsafe job history file or permissions")
	}
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open job history: %w", err)
	}
	defer f.Close()
	body, err := io.ReadAll(io.LimitReader(f, webHistoryMaxFileBytes+1))
	if err != nil {
		return fmt.Errorf("read job history: %w", err)
	}
	if len(body) > webHistoryMaxFileBytes {
		return errors.New("job history exceeds maximum size")
	}
	var persisted archivedWebHistory
	if err := json.Unmarshal(body, &persisted); err != nil || persisted.Version != 1 || len(persisted.Entries) > webHistoryMaxEntries {
		return errors.New("invalid job history file")
	}
	seen := make(map[string]bool)
	for _, item := range persisted.Entries {
		if !validArchivedJob(item) || seen[item.Response.ID] || item.Finished.After(now.Add(time.Minute)) {
			return errors.New("invalid job history record")
		}
		seen[item.Response.ID] = true
		if now.Sub(item.Finished) <= webHistoryMaxAge {
			s.history = append(s.history, item)
		}
	}
	sort.Slice(s.history, func(i, j int) bool {
		return s.history[i].Finished.After(s.history[j].Finished)
	})
	return nil
}

// persistWebHistoryLocked must run with s.mu held. It replaces a bounded
// regular file using a same-directory 0600 temp + fsync + atomic rename.
// If it fails the Web job still completes, but the history failure is logged.
func (s *webServer) persistWebHistoryLocked() error {
	body, err := json.Marshal(archivedWebHistory{Version: 1, Entries: s.history})
	if err != nil || len(body) > webHistoryMaxFileBytes {
		return errors.New("job history serialization failed")
	}
	path := s.historyPath()
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() {
			return errors.New("unsafe job history destination")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return errors.New("cannot inspect job history destination")
	}
	temp, err := os.CreateTemp(s.root, ".anyflip-history-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	if _, err = temp.Write(body); err != nil {
		temp.Close()
		return err
	}
	if err = temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err = temp.Close(); err != nil {
		return err
	}
	if err = os.Rename(temp.Name(), path); err != nil {
		return err
	}
	// The content was fsynced before rename; sync the directory where supported.
	if dir, err := os.Open(s.root); err == nil {
		_ = dir.Sync()
		_ = dir.Close()
	}
	return nil
}

func (s *webServer) archiveFinishedJobLocked(job *webJob) {
	status := s.snapshot(job)
	status.DownloadReady = false // archives never confer access to retained PDFs
	status.Archived = false // stored shape is validated without this UI marker
	item := archivedWebJob{Response: status, Finished: job.finished}
	if !validArchivedJob(item) {
		log.Printf("job %s history snapshot invalid", job.id)
		return
	}
	fresh := make([]archivedWebJob, 0, webHistoryMaxEntries)
	fresh = append(fresh, item)
	for _, old := range s.history {
		if old.Response.ID == item.Response.ID || time.Since(old.Finished) > webHistoryMaxAge {
			continue
		}
		if len(fresh) == webHistoryMaxEntries {
			break
		}
		fresh = append(fresh, old)
	}
	s.history = fresh
	if err := s.persistWebHistoryLocked(); err != nil {
		log.Printf("job %s history write failed (metadata unavailable after restart)", job.id)
	}
}

func (s *webServer) archivedSnapshotLocked(id string) (webJobResponse, bool) {
	for _, item := range s.history {
		if item.Response.ID == id {
			status := item.Response
			status.Archived = true
			status.DownloadReady = false
			return status, true
		}
	}
	return webJobResponse{}, false
}
