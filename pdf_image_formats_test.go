package main

import (
	"crypto/sha256"
	"encoding/base64"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
)

// These are real, small image files, not placeholder bytes. They exercise
// pdfcpu's actual image import path without fetching an external book.
func writeP1JPEG(t *testing.T, dir, name string) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 17, 23))
	for y := 0; y < 23; y++ {
		for x := 0; x < 17; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 10), G: uint8(y * 8), B: 90, A: 255})
		}
	}
	f, err := os.OpenFile(filepath.Join(dir, name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if err := jpeg.Encode(f, img, &jpeg.Options{Quality: 92}); err != nil {
		_ = f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

func writeP1WebP(t *testing.T, dir, name string) {
	t.Helper()
	// 16 x 12 lossless WebP, generated from an original solid-color test image.
	const fixture = "UklGRh4AAABXRUJQVlA4TBEAAAAvD8ACAAdQlO5Vuv+BiOh/AAA="
	data, err := base64.StdEncoding.DecodeString(fixture)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestP1MixedJPEGPNGWebPCreatesValidPDFWithoutChangingSources(t *testing.T) {
	dir := t.TempDir()
	writeP1JPEG(t, dir, "0000.jpg")
	writePNG(t, dir, "0001.png")
	writeP1WebP(t, dir, "0002.webp")

	var sourceHashes [3][32]byte
	for i, name := range []string{"0000.jpg", "0001.png", "0002.webp"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		sourceHashes[i] = sha256.Sum256(data)
	}

	outDir := t.TempDir()
	out := filepath.Join(outDir, "mixed.pdf")
	// Force a multi-chunk import, as used for larger books.
	if err := createPDFVerified(out, dir, 3, 2); err != nil {
		t.Fatalf("mixed JPG/PNG/WebP PDF import failed: %v", err)
	}
	if err := api.ValidateFile(out, nil); err != nil {
		t.Fatalf("generated PDF failed structural validation: %v", err)
	}
	count, err := api.PageCountFile(out)
	if err != nil || count != 3 {
		t.Fatalf("generated PDF pages=%d, err=%v; want 3 pages", count, err)
	}
	info, err := os.Stat(out)
	if err != nil || info.Size() < 500 {
		t.Fatalf("generated PDF missing or unexpectedly small: info=%v, err=%v", info, err)
	}

	for i, name := range []string{"0000.jpg", "0001.png", "0002.webp"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if sha256.Sum256(data) != sourceHashes[i] {
			t.Errorf("source image %q was modified during PDF conversion", name)
		}
	}
	entries, err := os.ReadDir(outDir)
	if err != nil || len(entries) != 1 || entries[0].Name() != "mixed.pdf" {
		t.Fatalf("temporary PDF staging files leaked: %v, err=%v", entries, err)
	}
}

func TestP1CorruptedFormatsNeverPublishPartialPDF(t *testing.T) {
	for _, extension := range []string{".jpg", ".png", ".webp"} {
		t.Run(extension, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "0000"+extension), []byte("not a valid image"), 0600); err != nil {
				t.Fatal(err)
			}
			outDir := t.TempDir()
			out := filepath.Join(outDir, "corrupt.pdf")
			if err := createPDFVerified(out, dir, 1, 1); err == nil {
				t.Fatal("corrupt image unexpectedly produced a PDF")
			}
			if _, err := os.Stat(out); !os.IsNotExist(err) {
				t.Fatalf("partial PDF was published: %v", err)
			}
			entries, err := os.ReadDir(outDir)
			if err != nil || len(entries) != 0 {
				t.Fatalf("PDF staging artifacts remain: %v, err=%v", entries, err)
			}
		})
	}
}
