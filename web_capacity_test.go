package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWebAdmissionAccountsForConcurrentReservations(t *testing.T) {
	gb := uint64(1 << 30)
	for _, tc := range []struct {
		name string
		used int64
		free uint64
		active int
		denied bool
	}{
		{"empty volume", 0, 8 * gb, 0, false},
		{"second active runner", 1 << 30, 8 * gb, 1, false},
		{"minimum usable free", 0, gb, 0, false},
		{"less than reserved free", 0, gb - 1, 0, true},
		{"second runner needs more free", 0, gb, 1, true},
		{"retained plus reservation exceeds global budget", webVolumeSoftLimit - webMaxJobBytes + 1, 8 * gb, 0, true},
		{"at global soft limit", webVolumeSoftLimit - webMaxJobBytes, 8 * gb, 0, false},
		{"too many concurrent reservations", 0, 8 * gb, 2, true},
		{"invalid negative usage", -1, 8 * gb, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := checkWebAdmission(tc.used, tc.free, tc.active)
			if (err != nil) != tc.denied {
				t.Fatalf("capacity gate error=%v denied=%v", err, tc.denied)
			}
			if err != nil && !errors.Is(err, errWebCapacityExceeded) {
				t.Fatalf("unexpected capacity error: %v", err)
			}
		})
	}
}

func TestManagedVolumeUsageCountsRetainedAndStagingButNotUnrelated(t *testing.T) {
	root := t.TempDir()
	id := strings.Repeat("a", 32)
	img := filepath.Join(root, id+".images")
	stage := filepath.Join(root, ".anyflip-pdf-stage-abc")
	for _, dir := range []string{img, stage} {
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{
		filepath.Join(root, id+".pdf"): "12345",
		filepath.Join(img, "page.png"): "1234",
		filepath.Join(stage, "result.pdf"): "123",
		filepath.Join(root, "important.pdf"): strings.Repeat("X", 1000),
	}
	for path, content := range files {
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	used, err := managedVolumeUsage(root)
	if err != nil || used != 12 {
		t.Fatalf("expected 12 owned bytes, got %d err=%v", used, err)
	}
	if got, err := os.ReadFile(filepath.Join(root, "important.pdf")); err != nil || len(got) != 1000 {
		t.Fatalf("unrelated file was modified: %v", err)
	}
}

func TestManagedVolumeRejectsOwnedSymlinksWithoutFollowing(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "sensitive")
	if err := os.WriteFile(outside, []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, strings.Repeat("b", 32)+".pdf")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := managedVolumeUsage(root); err == nil {
		t.Fatal("managed symlink should fail closed")
	}
	data, err := os.ReadFile(outside)
	if err != nil || string(data) != "secret" {
		t.Fatalf("outside file was accessed or modified: %v", err)
	}
}

func TestCapacityReservationsAreReleased(t *testing.T) {
	s, err := newWebServer(testWebToken, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := s.reserveWebCapacity(); err != nil {
		t.Skipf("test filesystem lacks 1 GiB usable free space: %v", err)
	}
	if s.activeReservations != 1 {
		t.Fatal("first reservation not recorded")
	}
	s.releaseWebCapacity()
	if s.activeReservations != 0 {
		t.Fatal("reservation leaked")
	}
}
