package main

import (
    "context"
    "fmt"
    "image"
    "image/color"
    "image/png"
    "io"
    "os"
    "path/filepath"
    "strings"

    "golang.org/x/image/webp"
)

// preparePDFPageWebP normalizes WebP images in the already-private PDF staging
// directory, never in the original image workspace. In particular, lossy WebP
// YCbCr samples use limited-range BT.601; importing them as full-range YCbCr
// washes out colors. Adapted from upstream Lofter1/anyflip-downloader PR #126.
// The optional budget guard runs after each new staging artifact.
func preparePDFPageWebP(ctx context.Context, pages []string, stageDir string, budgetCheck func() error) ([]string, error) {
    if ctx == nil {
        return nil, fmt.Errorf("WebP conversion context is required")
    }
    normalized := append([]string(nil), pages...)
    for i, srcPath := range pages {
        if !strings.EqualFold(filepath.Ext(srcPath), ".webp") {
            continue
        }
        if err := ctx.Err(); err != nil {
            return nil, err
        }
        file, err := os.Open(srcPath)
        if err != nil {
            return nil, fmt.Errorf("open WebP page %d: %w", i+1, err)
        }
        cfg, err := webp.DecodeConfig(file)
        if err == nil {
            err = validatePageDimensions(cfg.Width, cfg.Height)
        }
        if err == nil {
            _, err = file.Seek(0, io.SeekStart)
        }
        var decoded image.Image
        if err == nil {
            decoded, err = webp.Decode(file)
        }
        closeErr := file.Close()
        if err != nil {
            return nil, fmt.Errorf("decode WebP page %d: %w", i+1, err)
        }
        if closeErr != nil {
            return nil, fmt.Errorf("close WebP page %d: %w", i+1, closeErr)
        }
        if err := ctx.Err(); err != nil {
            return nil, err
        }
        switch img := decoded.(type) {
        case *image.YCbCr:
            decoded = limitedYCbCrToNRGBA(img, nil)
        case *image.NYCbCrA:
            decoded = limitedYCbCrToNRGBA(&img.YCbCr, img)
        }
        outPath := filepath.Join(stageDir, fmt.Sprintf("webp-%04d.png", i))
        out, err := os.OpenFile(outPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
        if err != nil {
            return nil, fmt.Errorf("create staged WebP page %d: %w", i+1, err)
        }
        encodeErr := (&png.Encoder{CompressionLevel: png.BestSpeed}).Encode(out, decoded)
        closeErr = out.Close()
        if encodeErr != nil || closeErr != nil {
            _ = os.Remove(outPath)
            if encodeErr != nil {
                return nil, fmt.Errorf("encode WebP page %d: %w", i+1, encodeErr)
            }
            return nil, fmt.Errorf("close staged WebP page %d: %w", i+1, closeErr)
        }
        normalized[i] = outPath
        if budgetCheck != nil {
            if err := budgetCheck(); err != nil {
                return nil, err
            }
        }
    }
    return normalized, nil
}

// limitedYCbCrToNRGBA applies BT.601 studio-range expansion to lossy WebP.
// Lossless WebP is decoded to RGB and must not be adjusted.
func limitedYCbCrToNRGBA(src *image.YCbCr, alpha *image.NYCbCrA) *image.NRGBA {
    bounds := src.Bounds()
    dst := image.NewNRGBA(bounds)
    for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
        for x := bounds.Min.X; x < bounds.Max.X; x++ {
            yIndex := src.YOffset(x, y)
            cIndex := src.COffset(x, y)
            luma := 1.164 * (float64(src.Y[yIndex]) - 16)
            cb := float64(src.Cb[cIndex]) - 128
            cr := float64(src.Cr[cIndex]) - 128
            opacity := uint8(255)
            if alpha != nil {
                opacity = alpha.A[alpha.AOffset(x, y)]
            }
            dst.SetNRGBA(x, y, color.NRGBA{
                R: clampWebPColor(luma + 1.596*cr),
                G: clampWebPColor(luma - 0.392*cb - 0.813*cr),
                B: clampWebPColor(luma + 2.017*cb),
                A: opacity,
            })
        }
    }
    return dst
}

func clampWebPColor(value float64) uint8 {
    if value <= 0 {
        return 0
    }
    if value >= 255 {
        return 255
    }
    return uint8(value + 0.5)
}
