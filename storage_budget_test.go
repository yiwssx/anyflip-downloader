package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestJobStorageBudgetCountsOwnedRegularFiles(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "images")
	if err := os.Mkdir(sub, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "0000.png"), []byte("1234567"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "0001.png"), []byte("12345"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := checkJobStorageBudget(12, root); err != nil {
		t.Fatalf("exact limit should be allowed: %v", err)
	}
	if err := checkJobStorageBudget(11, root); !errors.Is(err, errJobStorageBudgetExceeded) {
		t.Fatalf("over-limit workspace should fail closed: %v", err)
	}
	if err := checkJobStorageBudget(0, root); err != nil {
		t.Fatalf("CLI/unlimited mode must be unchanged: %v", err)
	}
}

func TestJobStorageBudgetRejectsSymlink(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "private-file")
	if err := os.WriteFile(outside, []byte("untouched"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "link.png")); err != nil {
		t.Skipf("symlinks not available: %v", err)
	}
	if err := checkJobStorageBudget(1024, root); err == nil {
		t.Fatal("symlink must never be followed or accepted as owned file")
	}
	got, err := os.ReadFile(outside)
	if err != nil || string(got) != "untouched" {
		t.Fatalf("unrelated target was modified: %v", err)
	}
}

func TestRunDownloadJobWebPageLimitPrecedesImageDownload(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "output.pdf")
	workspace := filepath.Join(root, "workspace.images")
	_, err := RunDownloadJob(context.Background(), DownloadJobOptions{
		URL: "https://anyflip.com/qrfai/tfvn",
		OutputFile: target, TempDownloadFolder: workspace,
		Threads: 2, ChunkSize: 1, MaxPages: 1, MaxJobBytes: 1 << 20,
		HTTPClient: jobFixtureClient(t),
	}, nil)
	if !errors.Is(err, errJobPageLimitExceeded) {
		t.Fatalf("expected 2-page fixture to exceed 1-page Web cap: %v", err)
	}
	if _, err := os.Lstat(workspace); !os.IsNotExist(err) {
		t.Fatalf("page count rejected too late: workspace exists: %v", err)
	}
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Fatalf("page count rejection produced output: %v", err)
	}
}

func TestRunDownloadJobWebStorageLimitCleansWorkspace(t *testing.T) {
	root := t.TempDir()
	output := filepath.Join(root, "bounded.pdf")
	workspace := filepath.Join(root, "bounded.images")
	_, err := RunDownloadJob(context.Background(), DownloadJobOptions{
		URL: "https://anyflip.com/qrfai/tfvn",
		OutputFile: output, TempDownloadFolder: workspace,
		Threads: 2, ChunkSize: 1, MaxPages: webMaxPages, MaxJobBytes: 1,
		HTTPClient: jobFixtureClient(t),
	}, nil)
	if !errors.Is(err, errJobStorageBudgetExceeded) {
		t.Fatalf("expected storage quota exceeded, got: %v", err)
	}
	for _, path := range []string{output, workspace} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("quota violation should not retain %s: %v", path, err)
		}
	}
}

func TestPDFStagingBudgetRejectsBeforePublication(t *testing.T) {
	root := t.TempDir()
	pages := filepath.Join(root, "images")
	if err := os.Mkdir(pages, 0700); err != nil {
		t.Fatal(err)
	}
	writePNG(t, pages, "0000.png")
	writePNG(t, pages, "0001.png")
	info, err := os.Stat(filepath.Join(pages, "0000.png"))
	if err != nil {
		t.Fatal(err)
	}
	info2, err := os.Stat(filepath.Join(pages, "0001.png"))
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(root, "book.pdf")
	err = createPDFVerifiedWithBudget(context.Background(), output, pages, 2, 1, nil, info.Size()+info2.Size()+1)
	if !errors.Is(err, errJobStorageBudgetExceeded) {
		t.Fatalf("PDF staging should exceed image-only budget: %v", err)
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("over-budget PDF must not be published: %v", err)
	}
	staging, err := filepath.Glob(filepath.Join(root, ".anyflip-pdf-stage-*"))
	if err != nil || len(staging) != 0 {
		t.Fatalf("PDF staging not cleaned: files=%v err=%v", staging, err)
	}
}

func TestWebJobBudgetErrorCodesDoNotLeakDetails(t *testing.T) {
	for _, tc := range []struct {
		err  error
		code string
	}{
		{fmt.Errorf("sensitive/path: %w", errJobStorageBudgetExceeded), "storage_quota_exceeded"},
		{fmt.Errorf("secret-source: %w", errJobPageLimitExceeded), "page_limit_exceeded"},
	} {
		code, detail := describeWebJobError(tc.err)
		if code != tc.code || strings.Contains(detail, "secret") || strings.Contains(detail, "sensitive") {
			t.Fatalf("unsafe quota classification code=%q message=%q", code, detail)
		}
	}
}
