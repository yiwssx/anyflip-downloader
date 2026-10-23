package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeRetentionFile(t *testing.T, path string, old time.Time) {
	t.Helper()
	if err := os.WriteFile(path, []byte("sample"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
}

func TestJobArtifactIDStrictNames(t *testing.T) {
	for _, name := range []string{
		strings.Repeat("a", 32) + ".pdf",
		strings.Repeat("f", 32) + ".images",
	} {
		if _, _, ok := jobArtifactID(name); !ok {
			t.Errorf("expected %q to match managed artifact format", name)
		}
	}
	for _, name := range []string{
		"important.pdf", "README", ".env",
		strings.Repeat("A", 32) + ".pdf", strings.Repeat("0", 31) + ".pdf",
		strings.Repeat("0", 32) + ".pdf.bak",
		strings.Repeat("0", 32) + ".images-old",
		"../" + strings.Repeat("0", 32) + ".pdf",
	} {
		if _, _, ok := jobArtifactID(name); ok {
			t.Errorf("unexpectedly accepted %q as a job artifact", name)
		}
	}
}

func TestRestartReclaimsOnlyStaleJobArtifacts(t *testing.T) {
	root := t.TempDir()
	now := time.Now()
	stale := now.Add(-3 * time.Hour)
	id := strings.Repeat("a", 32)
	freshID := strings.Repeat("b", 32)
	oldPDF := filepath.Join(root, id+".pdf")
	oldFolder := filepath.Join(root, id+".images")
	if err := os.Mkdir(oldFolder, 0700); err != nil {
		t.Fatal(err)
	}
	writeRetentionFile(t, filepath.Join(oldFolder, "page.jpg"), stale)
	if err := os.Chtimes(oldFolder, stale, stale); err != nil {
		t.Fatal(err)
	}
	writeRetentionFile(t, oldPDF, stale)
	fresh := filepath.Join(root, freshID+".pdf")
	writeRetentionFile(t, fresh, now)
	unrelated := filepath.Join(root, "important.pdf")
	writeRetentionFile(t, unrelated, stale)
	s, err := newWebServer(testWebToken, root)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.jobs) != 0 {
		t.Fatal("restart should not pretend to restore in-memory jobs")
	}
	for _, removed := range []string{oldPDF, oldFolder} {
		if _, err := os.Lstat(removed); !os.IsNotExist(err) {
			t.Errorf("stale orphan was retained: %q (%v)", removed, err)
		}
	}
	for _, preserved := range []string{fresh, unrelated} {
		if _, err := os.Lstat(preserved); err != nil {
			t.Errorf("fresh/unrelated file was removed: %q (%v)", preserved, err)
		}
	}
}

func TestJanitorProtectsActiveJobs(t *testing.T) {
	root := t.TempDir()
	s, err := newWebServer(testWebToken, root)
	if err != nil {
		t.Fatal(err)
	}
	id := strings.Repeat("c", 32)
	path := filepath.Join(root, id+".pdf")
	old := time.Now().Add(-3 * time.Hour)
	writeRetentionFile(t, path, old)
	s.jobs[id] = &webJob{id: id, stage: "downloading"}
	s.purgeOldJobs(time.Now())
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("active job artifact removed: %v", err)
	}
	if _, ok := s.jobs[id]; !ok {
		t.Fatal("active in-memory job removed")
	}
	s.mu.Lock()
	s.jobs[id].stage = "failed"
	s.jobs[id].finished = old
	s.mu.Unlock()
	s.purgeOldJobs(time.Now())
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expired job output retained: %v", err)
	}
	if _, ok := s.jobs[id]; ok {
		t.Fatal("expired terminal job still tracked")
	}
}

func TestJanitorNeverTraversesSymlinks(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "important.pdf")
	writeRetentionFile(t, outside, time.Now().Add(-3*time.Hour))
	id := strings.Repeat("d", 32)
	link := filepath.Join(root, id+".pdf")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	s, err := newWebServer(testWebToken, root)
	if err != nil {
		t.Fatal(err)
	}
	s.purgeOldJobs(time.Now().Add(2 * time.Hour))
	content, err := os.ReadFile(outside)
	if err != nil || string(content) != "sample" {
		t.Fatalf("symlink cleanup followed link outside managed dir: %v", err)
	}
}
