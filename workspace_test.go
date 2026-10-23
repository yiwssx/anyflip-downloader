package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestValidateDownloadSettings(t *testing.T) {
	tests := []struct {
		name       string
		threads    int
		retries    int
		retryDelay time.Duration
		chunkSize  uint
		wantErr    bool
	}{
		{"defaults", 1, 1, time.Second, 10, false},
		{"maximum accepted", 16, 10, time.Minute, 1000, false},
		{"zero threads", 0, 1, time.Second, 10, true},
		{"too many threads", 17, 1, time.Second, 10, true},
		{"negative retries", 1, -1, time.Second, 10, true},
		{"too many retries", 1, 11, time.Second, 10, true},
		{"negative delay", 1, 1, -time.Second, 10, true},
		{"excessive delay", 1, 1, 61 * time.Second, 10, true},
		{"zero chunk size", 1, 1, time.Second, 0, true},
		{"excessive chunk size", 1, 1, time.Second, 1001, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDownloadSettings(tt.threads, tt.retries, tt.retryDelay, tt.chunkSize)
			if (err != nil) != tt.wantErr {
				t.Fatalf("got error %v, want error=%v", err, tt.wantErr)
			}
		})
	}
}

func TestCreateDownloadWorkspaceAuto(t *testing.T) {
	dir, cleanup, err := createDownloadWorkspace("")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cleanup() }()
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		t.Fatalf("workspace not created: info=%v err=%v", info, err)
	}
	if !strings.HasPrefix(filepath.Base(dir), "anyflip-downloader-") {
		t.Fatalf("unexpected workspace name: %s", dir)
	}
	if err := cleanup(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("workspace still exists after cleanup: %v", err)
	}
}

func TestCreateDownloadWorkspaceRejectsExistingFolder(t *testing.T) {
	existing := t.TempDir()
	keep := filepath.Join(existing, "important.txt")
	if err := os.WriteFile(keep, []byte("keep this"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := createDownloadWorkspace(existing); err == nil {
		t.Fatal("expected existing directory to be rejected")
	}
	if data, err := os.ReadFile(keep); err != nil || string(data) != "keep this" {
		t.Fatalf("existing files modified or deleted: %q %v", data, err)
	}
}

func TestCreateDownloadWorkspaceExplicitFolder(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "new-job")
	got, cleanup, err := createDownloadWorkspace(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cleanup() }()
	if got != dir {
		t.Fatalf("got %q want %q", got, dir)
	}
	if err := os.WriteFile(filepath.Join(dir, "page.jpg"), []byte("page"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := cleanup(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("workspace still exists: %v", err)
	}
}

func TestCleanupRejectsReplacementDirectory(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "download")
	_, cleanup, err := createDownloadWorkspace(dir)
	if err != nil {
		t.Fatal(err)
	}
	moved := filepath.Join(parent, "original")
	if err := os.Rename(dir, moved); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	important := filepath.Join(dir, "keep.txt")
	if err := os.WriteFile(important, []byte("important"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := cleanup(); err == nil {
		t.Fatal("expected cleanup to reject replacement")
	}
	if _, err := os.Stat(important); err != nil {
		t.Fatalf("cleanup touched replacement: %v", err)
	}
}
