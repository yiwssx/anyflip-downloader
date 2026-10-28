package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func createPDFStageFixture(t *testing.T, root, name string, fileTime, directoryTime time.Time) string {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	page := filepath.Join(path, "result.pdf")
	if err := os.WriteFile(page, []byte("synthetic staged output"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(page, fileTime, fileTime); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, directoryTime, directoryTime); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestOrphanPDFStageCleanupAfterRestart(t *testing.T) {
	root := t.TempDir()
	now := time.Now()
	stale := now.Add(-3 * time.Hour)
	oldStage := createPDFStageFixture(t, root, pdfStagePrefix+"ab12345678", stale, stale)
	freshStage := createPDFStageFixture(t, root, pdfStagePrefix+"cd12345678", now, stale)
	unrelated := createPDFStageFixture(t, root, ".anyflip-pdf-stage-not managed", stale, stale)
	outside := filepath.Join(t.TempDir(), "protected")
	if err := os.WriteFile(outside, []byte("outside data"), 0600); err != nil {
		t.Fatal(err)
	}
	symlink := filepath.Join(root, pdfStagePrefix+"ef12345678")
	if err := os.Symlink(outside, symlink); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	s, err := newWebServer(testWebToken, root)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.jobs) != 0 {
		t.Fatal("restart must not restore running jobs")
	}
	if _, err := os.Lstat(oldStage); !os.IsNotExist(err) {
		t.Fatalf("old orphaned PDF stage was not reclaimed: %v", err)
	}
	for _, protected := range []string{freshStage, unrelated, symlink, outside} {
		if _, err := os.Lstat(protected); err != nil {
			t.Fatalf("unexpected deletion of %s: %v", protected, err)
		}
	}
	if data, err := os.ReadFile(outside); err != nil || string(data) != "outside data" {
		t.Fatalf("symlink target was modified: %q, %v", data, err)
	}
}

func TestOrphanPDFStageCleanupRequiresNoActiveJobs(t *testing.T) {
	root := t.TempDir()
	old := time.Now().Add(-3 * time.Hour)
	stage := createPDFStageFixture(t, root, pdfStagePrefix+"gh12345678", old, old)
	s, err := newWebServer(testWebToken, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// The fixture must be created after server startup; in-flight stages
	// cannot exist when a fresh server runs restart-time cleanup.
	stage = createPDFStageFixture(t, s.root, pdfStagePrefix+"ij12345678", old, old)
	id := strings.Repeat("a", 32)
	s.mu.Lock()
	s.jobs[id] = &webJob{id: id, stage: "converting"}
	s.mu.Unlock()
	s.purgeOldJobs(time.Now())
	if _, err := os.Lstat(stage); err != nil {
		t.Fatalf("removed stage while conversion active: %v", err)
	}
	s.mu.Lock()
	s.jobs[id].finished = old
	s.jobs[id].stage = "failed"
	s.mu.Unlock()
	s.purgeOldJobs(time.Now())
	if _, err := os.Lstat(stage); !os.IsNotExist(err) {
		t.Fatalf("failed to reclaim stale stage after worker finished: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(root, pdfStagePrefix+"gh12345678")); err != nil {
		t.Fatalf("unrelated outside-root stage modified: %v", err)
	}
}

func TestOrphanPDFStageCleanupRejectsNestedSymlink(t *testing.T) {
	s, err := newWebServer(testWebToken, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-3 * time.Hour)
	stage := createPDFStageFixture(t, s.root, pdfStagePrefix+"kl12345678", old, old)
	outside := filepath.Join(t.TempDir(), "important")
	if err := os.WriteFile(outside, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(stage, "link")); err != nil {
		t.Skip(err)
	}
	if err := os.Chtimes(stage, old, old); err != nil {
		t.Fatal(err)
	}
	s.purgeOldJobs(time.Now())
	if _, err := os.Lstat(stage); err != nil {
		t.Fatalf("unexpected cleanup of stage containing special entry: %v", err)
	}
	if data, err := os.ReadFile(outside); err != nil || string(data) != "keep" {
		t.Fatalf("outside target modified: %q, %v", data, err)
	}
}

func TestManagedPDFStageNameRejectsUnknownFiles(t *testing.T) {
	for _, name := range []string{pdfStagePrefix + "ab12345678", pdfStagePrefix + "AZ01234567"} {
		if !managedPDFStageName(name) {
			t.Fatalf("stage name %q was rejected", name)
		}
	}
	for _, name := range []string{pdfStagePrefix, pdfStagePrefix + "ab12345", pdfStagePrefix + "ab 123456", pdfStagePrefix + "../outside", "important.pdf"} {
		if managedPDFStageName(name) {
			t.Fatalf("unmanaged name %q was treated as PDF stage", name)
		}
	}
}
