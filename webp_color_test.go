package main

import (
    "context"
    "crypto/sha256"
    "encoding/base64"
    "errors"
    "image"
    "image/color"
    "image/png"
    "os"
    "path/filepath"
    "testing"
)

func TestLimitedYCbCrToNRGBAStudioRange(t *testing.T) {
    src := image.NewYCbCr(image.Rect(0, 0, 2, 1), image.YCbCrSubsampleRatio444)
    src.Y[src.YOffset(0, 0)] = 16
    src.Y[src.YOffset(1, 0)] = 235
    for i := range src.Cb {
        src.Cb[i], src.Cr[i] = 128, 128
    }
    got := limitedYCbCrToNRGBA(src, nil)
    if pixel := got.NRGBAAt(0, 0); pixel != (color.NRGBA{A: 255}) {
        t.Fatalf("studio black = %v, want RGB(0,0,0)", pixel)
    }
    if pixel := got.NRGBAAt(1, 0); pixel != (color.NRGBA{R: 255, G: 255, B: 255, A: 255}) {
        t.Fatalf("studio white = %v, want RGB(255,255,255)", pixel)
    }
}

func TestPreparePDFPageWebPLossyColorAndSourceIntegrity(t *testing.T) {
    // Real 16x16 lossy WebP, bright white. Without limited-range expansion
    // Go's default full-range YCbCr path produces approximately RGB(235).
    const fixture = "UklGRiYAAABXRUJQVlA4IBoAAAAwAQCdASoQABAAAMASJaQAA3AA/v7deAAAAA=="
    data, err := base64.StdEncoding.DecodeString(fixture)
    if err != nil { t.Fatal(err) }
    dir := t.TempDir()
    src := filepath.Join(dir, "0000.webp")
    if err := os.WriteFile(src, data, 0600); err != nil { t.Fatal(err) }
    original := sha256.Sum256(data)
    stageDir := t.TempDir()
    paths, err := preparePDFPageWebP(context.Background(), []string{src}, stageDir, nil)
    if err != nil { t.Fatal(err) }
    if len(paths) != 1 || filepath.Ext(paths[0]) != ".png" || paths[0] == src {
        t.Fatalf("WebP not normalized into isolated PNG: %v", paths)
    }
    f, err := os.Open(paths[0])
    if err != nil { t.Fatal(err) }
    img, err := png.Decode(f)
    _ = f.Close()
    if err != nil { t.Fatal(err) }
    r,g,b,_ := img.At(8,8).RGBA()
    if r < 0xfa00 || g < 0xfa00 || b < 0xfa00 {
        t.Fatalf("lossy white remained washed out: RGBA16(%d,%d,%d)",r,g,b)
    }
    info, err := os.Stat(paths[0])
    if err != nil { t.Fatal(err) }
    if info.Mode().Perm() != 0600 {
        t.Fatalf("staged PNG has insecure mode %o",info.Mode().Perm())
    }
    after, err := os.ReadFile(src)
    if err != nil { t.Fatal(err) }
    if sha256.Sum256(after) != original {
        t.Fatal("original WebP bytes changed")
    }
}

func TestPreparePDFPageWebPLosslessAndBudgetGuard(t *testing.T) {
    dir := t.TempDir()
    writeP1WebP(t, dir, "0000.webp")
    output, err := preparePDFPageWebP(context.Background(), []string{filepath.Join(dir, "0000.webp")}, t.TempDir(), nil)
    if err != nil { t.Fatal(err) }
    imageFile, err := os.Open(output[0])
    if err != nil { t.Fatal(err) }
    img, err := png.Decode(imageFile)
    _ = imageFile.Close()
    if err != nil { t.Fatal(err) }
    if img.Bounds().Dx() != 16 || img.Bounds().Dy() != 12 {
        t.Fatalf("lossless WebP dimensions changed: %v", img.Bounds())
    }
    sentinel := errors.New("quota exceeded")
    _, err = preparePDFPageWebP(context.Background(), []string{filepath.Join(dir, "0000.webp")}, t.TempDir(), func() error { return sentinel })
    if !errors.Is(err, sentinel) {
        t.Fatalf("expected budget guard error, got %v", err)
    }
    ctx, cancel := context.WithCancel(context.Background())
    cancel()
    _, err = preparePDFPageWebP(ctx, []string{filepath.Join(dir, "0000.webp")}, t.TempDir(), nil)
    if !errors.Is(err, context.Canceled) {
        t.Fatalf("expected context canceled, got %v", err)
    }
}
