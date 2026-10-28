package main

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
)

const p1LargeFixturePages = 129

// p1PagePNG is an original synthetic fixture, not a copied publisher page.
func p1PagePNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{
				R: uint8((x * 17) % 256), G: uint8((y * 13) % 256),
				B: uint8((x + y) % 256), A: 255,
			})
		}
	}
	var data bytes.Buffer
	if err := png.Encode(&data, img); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}

// The distinct first/middle/last ratios validate ordering across PDF chunks,
// including the final partial chunk. The other pages have a common square size.
func p1Create129PageFixture(t *testing.T) (string, map[int][32]byte) {
	t.Helper()
	dir := t.TempDir()
	square := p1PagePNG(t, 8, 8)
	portrait := p1PagePNG(t, 5, 9)
	landscape := p1PagePNG(t, 9, 5)
	hashes := map[int][32]byte{}
	for i := 0; i < p1LargeFixturePages; i++ {
		page := square
		switch i {
		case 0:
			page = portrait
		case 64:
			page = landscape
		case 128:
			page = p1PagePNG(t, 11, 11)
		}
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("%04d.png", i)), page, 0600); err != nil {
			t.Fatal(err)
		}
		if i == 0 || i == 64 || i == 128 {
			hashes[i] = sha256.Sum256(page)
		}
	}
	return dir, hashes
}

func TestP1Large129PagePDFIntegrityAcrossChunks(t *testing.T) {
	imageDir, hashes := p1Create129PageFixture(t)
	outputDir := t.TempDir()
	output := filepath.Join(outputDir, "129-pages.pdf")
	if err := createPDFVerified(output, imageDir, p1LargeFixturePages, 24); err != nil {
		t.Fatal(err)
	}
	if err := api.ValidateFile(output, nil); err != nil {
		t.Fatalf("large image-based PDF failed validation: %v", err)
	}
	pages, err := api.PageCountFile(output)
	if err != nil || pages != p1LargeFixturePages {
		t.Fatalf("actual PDF pages=%d, error=%v; expected %d", pages, err, p1LargeFixturePages)
	}
	dims, err := api.PageDimsFile(output)
	if err != nil || len(dims) != p1LargeFixturePages {
		t.Fatalf("PDF page dimension records=%d, error=%v", len(dims), err)
	}
	for _, tc := range []struct {
		page int
		ratio float64
	}{
		{0, 5.0 / 9.0},
		{64, 9.0 / 5.0},
		{128, 1},
	} {
		got := dims[tc.page].Width / dims[tc.page].Height
		if math.Abs(got-tc.ratio) > 0.03 {
			t.Errorf("PDF page %d ratio %.4f; expected %.4f", tc.page+1, got, tc.ratio)
		}
		content, err := os.ReadFile(filepath.Join(imageDir, fmt.Sprintf("%04d.png", tc.page)))
		if err != nil {
			t.Fatal(err)
		}
		if sha256.Sum256(content) != hashes[tc.page] {
			t.Errorf("source image for page %d was modified during PDF import", tc.page+1)
		}
	}
	entries, err := os.ReadDir(outputDir)
	if err != nil || len(entries) != 1 || entries[0].Name() != "129-pages.pdf" {
		t.Errorf("PDF staging artifacts leaked after success: %v %v", entries, err)
	}
}

func TestP1Large129PageMissingLastPageDoesNotPublish(t *testing.T) {
	imageDir, _ := p1Create129PageFixture(t)
	if err := os.Remove(filepath.Join(imageDir, "0128.png")); err != nil {
		t.Fatal(err)
	}
	outputDir := t.TempDir()
	output := filepath.Join(outputDir, "incomplete.pdf")
	if err := createPDFVerified(output, imageDir, p1LargeFixturePages, 24); err == nil {
		t.Fatal("129-page fixture with missing final image must fail closed")
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("partial PDF published with missing last page: %v", err)
	}
	files, err := os.ReadDir(outputDir)
	if err != nil || len(files) != 0 {
		t.Fatalf("staging directory leaked: %v %v", files, err)
	}
}

func TestP1Large129PageLateCorruptionDoesNotPublish(t *testing.T) {
	imageDir, _ := p1Create129PageFixture(t)
	if err := os.WriteFile(filepath.Join(imageDir, "0128.png"), []byte("not a PNG"), 0600); err != nil {
		t.Fatal(err)
	}
	outputDir := t.TempDir()
	output := filepath.Join(outputDir, "corrupt.pdf")
	if err := createPDFVerified(output, imageDir, p1LargeFixturePages, 24); err == nil {
		t.Fatal("late corrupt image should fail PDF generation")
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("partial PDF published after late corrupt image: %v", err)
	}
	files, err := os.ReadDir(outputDir)
	if err != nil || len(files) != 0 {
		t.Fatalf("partial staging PDF leaked: %v %v", files, err)
	}
}
