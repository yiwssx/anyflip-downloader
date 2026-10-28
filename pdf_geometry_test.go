package main

import (
	"bytes"
	"crypto/sha256"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
)

func writeP1GeometryPNG(t *testing.T, dir, name string, width, height int) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{
				R: uint8(x % 256), G: uint8(y % 256),
				B: uint8((x + y) % 256), A: 255,
			})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), buf.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
}

// P1-D: each source image is a distinct orientation. With image import's
// full-page default, the PDF page MediaBox must follow the source aspect
// ratio and the original page sequence across chunk boundaries.
func TestP1MixedPageGeometryAndOrientation(t *testing.T) {
	dir := t.TempDir()
	sources := []struct {
		name string
		w, h int
	}{
		{"0000.png", 64, 128},  // portrait 1:2
		{"0001.png", 192, 64},  // landscape 3:1
		{"0002.png", 100, 100}, // square 1:1
		{"0003.png", 120, 200}, // portrait 3:5, next PDF chunk
	}
	var hashes [4][32]byte
	for i, src := range sources {
		writeP1GeometryPNG(t, dir, src.name, src.w, src.h)
		b, err := os.ReadFile(filepath.Join(dir, src.name))
		if err != nil {
			t.Fatal(err)
		}
		hashes[i] = sha256.Sum256(b)
	}
	output := filepath.Join(t.TempDir(), "geometry.pdf")
	if err := createPDFVerified(output, dir, len(sources), 3); err != nil {
		t.Fatalf("convert mixed geometry PDF: %v", err)
	}
	if err := api.ValidateFile(output, nil); err != nil {
		t.Fatalf("invalid converted PDF: %v", err)
	}
	dims, err := api.PageDimsFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if len(dims) != len(sources) {
		t.Fatalf("PDF dimensions returned %d pages, expected %d", len(dims), len(sources))
	}
	for i, src := range sources {
		if dims[i].Width <= 0 || dims[i].Height <= 0 {
			t.Errorf("page %d has invalid MediaBox: %+v", i+1, dims[i])
			continue
		}
		actualRatio := dims[i].Width / dims[i].Height
		sourceRatio := float64(src.w) / float64(src.h)
		if math.Abs(actualRatio-sourceRatio) > .02 {
			t.Errorf("page %d media box aspect %.4f does not preserve source %.4f", i+1, actualRatio, sourceRatio)
		}
		b, err := os.ReadFile(filepath.Join(dir, src.name))
		if err != nil || sha256.Sum256(b) != hashes[i] {
			t.Errorf("source image %d mutated during conversion: %v", i+1, err)
		}
	}
}

func TestP1GeometryFailureDoesNotPublishOutput(t *testing.T) {
	dir := t.TempDir()
	writeP1GeometryPNG(t, dir, "0000.png", 64, 128)
	// An invalid second image must not allow the first completed import
	// chunk to be published as a plausible but incomplete PDF.
	if err := os.WriteFile(filepath.Join(dir, "0001.png"), []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "bad.pdf")
	if err := createPDFVerified(out, dir, 2, 1); err == nil {
		t.Fatal("expected corrupt second page to reject whole PDF")
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatalf("partial PDF was published: %v", err)
	}
}
