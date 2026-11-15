package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// Upstream 2026-10-28 fixed a lossy WebP BT.601-range conversion issue.
// The existing mixed-format test uses lossless WebP, so this fixture checks
// the color regression without downloading any publisher content.
func TestUpstreamLossyWebPWhiteColorRangePreserved(t *testing.T) {
	// Original 16x16 white image encoded as lossy WebP, quality 70.
	// Without limited-range correction, white can appear near RGB(235,235,235).
	const fixture = "UklGRiQAAABXRUJQVlA4IBgAAAAwAQCdASoQABAAAkA4JaQAA3AA/vshgAA="
	sourceBytes, err := base64.StdEncoding.DecodeString(fixture)
	if err != nil {
		t.Fatal(err)
	}
	sourceDir := t.TempDir()
	sourcePath := filepath.Join(sourceDir, "0000.webp")
	if err := os.WriteFile(sourcePath, sourceBytes, 0600); err != nil {
		t.Fatal(err)
	}
	before := sha256.Sum256(sourceBytes)

	stagingDir := t.TempDir()
	pages, err := preparePDFPageWebP(context.Background(), []string{sourcePath}, stagingDir, func() error {
		return nil
	})
	if err != nil {
		t.Fatalf("prepare lossy WebP: %v", err)
	}
	if len(pages) != 1 || pages[0] == sourcePath {
		t.Fatalf("lossy WebP was not staged as a distinct image: %v", pages)
	}
	f, err := os.Open(pages[0])
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatalf("staged WebP conversion is not a valid PNG: %v", err)
	}
	if bounds := img.Bounds(); bounds.Dx() != 16 || bounds.Dy() != 16 {
		t.Fatalf("converted dimensions = %v, want 16x16", bounds)
	}
	got := color.NRGBAModel.Convert(img.At(8, 8)).(color.NRGBA)
	if got.R < 250 || got.G < 250 || got.B < 250 || got.A != 255 {
		t.Fatalf("white WebP lost color range during PDF staging: got %+v", got)
	}
	afterBytes, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(sourceBytes, afterBytes) || sha256.Sum256(afterBytes) != before {
		t.Fatal("WebP conversion changed the publisher image")
	}
}
