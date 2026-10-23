package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
)

func writePageFixture(t *testing.T, dir, filename string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, filename), []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestCollectOrderedPageImages(t *testing.T) {
	dir := t.TempDir()
	writePageFixture(t, dir, "0002.png")
	writePageFixture(t, dir, "0000.jpg")
	writePageFixture(t, dir, "0001.webp")
	got, err := collectOrderedPageImages(dir, 3)
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range []string{"0000.jpg", "0001.webp", "0002.png"} {
		if filepath.Base(got[i]) != want {
			t.Fatalf("page %d = %q, want %q", i+1, got[i], want)
		}
	}
}

func TestCollectOrderedPageImagesRejectsInvalidInputs(t *testing.T) {
	tests := []struct {
		name  string
		count int
		files []string
	}{
		{"zero pages", 0, nil},
		{"too many pages", 10001, nil},
		{"missing page", 2, []string{"0000.jpg"}},
		{"duplicate page", 1, []string{"0000.jpg", "0000.png"}},
		{"extra page", 1, []string{"0000.jpg", "0001.jpg"}},
		{"unexpected name", 1, []string{"page.jpg", "0000.jpg"}},
		{"empty image", 1, []string{"0000.jpg"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, file := range tc.files {
				if tc.name == "empty image" {
					if err := os.WriteFile(filepath.Join(dir, file), nil, 0600); err != nil {
						t.Fatal(err)
					}
				} else {
					writePageFixture(t, dir, file)
				}
			}
			if _, err := collectOrderedPageImages(dir, tc.count); err == nil {
				t.Fatal("expected image sequence validation to fail")
			}
		})
	}
}

func TestCollectOrderedPageImagesRejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(t.TempDir(), "image.jpg")
	if err := os.WriteFile(target, []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dir, "0000.jpg")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := collectOrderedPageImages(dir, 1); err == nil {
		t.Fatal("expected symlink image to be rejected")
	}
}

func TestPublishVerifiedFileNoOverwrite(t *testing.T) {
	dir := t.TempDir()
	stage := filepath.Join(dir, "stage.pdf")
	final := filepath.Join(dir, "final.pdf")
	if err := os.WriteFile(stage, []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(final, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := publishVerifiedFile(stage, final); err == nil {
		t.Fatal("expected refusal to overwrite")
	}
	data, err := os.ReadFile(final)
	if err != nil || string(data) != "original" {
		t.Fatalf("existing output was modified: %q %v", data, err)
	}
}

func TestPublishVerifiedFileSuccess(t *testing.T) {
	dir := t.TempDir()
	stage := filepath.Join(dir, "stage.pdf")
	final := filepath.Join(dir, "final.pdf")
	if err := os.WriteFile(stage, []byte("verified"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := publishVerifiedFile(stage, final); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(final)
	if err != nil || string(data) != "verified" {
		t.Fatalf("published data mismatch: %q %v", data, err)
	}
}

func writePNG(t *testing.T, dir, filename string) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, filename), out.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestCreatePDFVerifiedWithRealImages(t *testing.T) {
	dir := t.TempDir()
	writePNG(t, dir, "0001.png")
	writePNG(t, dir, "0000.png")
	output := filepath.Join(t.TempDir(), "book.pdf")
	if err := createPDFVerified(output, dir, 2, 1); err != nil {
		t.Fatal(err)
	}
	count, err := api.PageCountFile(output)
	if err != nil || count != 2 {
		t.Fatalf("generated PDF page count = %d, error = %v", count, err)
	}
}

func TestCreatePDFVerifiedMissingPageDoesNotCreateOutput(t *testing.T) {
	dir := t.TempDir()
	writePNG(t, dir, "0000.png")
	output := filepath.Join(t.TempDir(), "book.pdf")
	if err := createPDFVerified(output, dir, 2, 10); err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("expected missing-page error, got %v", err)
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("partial output exists: %v", err)
	}
}

func TestCreatePDFVerifiedCorruptImageDoesNotCreateOutput(t *testing.T) {
	dir := t.TempDir()
	writePageFixture(t, dir, "0000.jpg")
	outputDir := t.TempDir()
	output := filepath.Join(outputDir, "book.pdf")
	if err := createPDFVerified(output, dir, 1, 1); err == nil {
		t.Fatal("expected corrupt image to fail conversion")
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("partial output exists: %v", err)
	}
	entries, err := os.ReadDir(outputDir)
	if err != nil || len(entries) != 0 {
		t.Fatalf("staging artifacts remain: %v %v", entries, err)
	}
}
