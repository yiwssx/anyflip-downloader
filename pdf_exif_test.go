package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
)

// makeEXIFJPEG creates original, synthetic JPEG bytes and a minimal standards-
// compliant Exif IFD0 orientation field, without using copyrighted fixtures.
func makeEXIFJPEG(t *testing.T, orientation int, order binary.ByteOrder) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 30, 12))
	for y := 0; y < 12; y++ {
		for x := 0; x < 30; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x*7), G: uint8(y*19), B: uint8((x+y)*5), A: 255})
		}
	}
	var base bytes.Buffer
	if err := jpeg.Encode(&base, img, &jpeg.Options{Quality: 96}); err != nil {
		t.Fatal(err)
	}
	tiff := make([]byte, 26) // TIFF header, one IFD0 entry, next IFD pointer
	if order == binary.LittleEndian {
		copy(tiff[:2], "II")
	} else {
		copy(tiff[:2], "MM")
	}
	order.PutUint16(tiff[2:4], 42)
	order.PutUint32(tiff[4:8], 8)
	order.PutUint16(tiff[8:10], 1)
	order.PutUint16(tiff[10:12], 0x0112) // Orientation
	order.PutUint16(tiff[12:14], 3)      // SHORT
	order.PutUint32(tiff[14:18], 1)
	order.PutUint16(tiff[18:20], uint16(orientation))
	payload := append([]byte("Exif\x00\x00"), tiff...)
	size := len(payload) + 2
	app1 := append([]byte{0xff, 0xe1, byte(size >> 8), byte(size)}, payload...)
	raw := base.Bytes()
	return append(append(append([]byte{}, raw[:2]...), app1...), raw[2:]...)
}

func TestP1EXIFOrientationParsingBothEndianFormats(t *testing.T) {
	for _, order := range []binary.ByteOrder{binary.LittleEndian, binary.BigEndian} {
		for orientation := 1; orientation <= 8; orientation++ {
			encoded := makeEXIFJPEG(t, orientation, order)
			got, err := jpegEXIFOrientation(encoded)
			if err != nil || got != orientation {
				t.Fatalf("Exif orientation got %d, error %v; want %d", got, err, orientation)
			}
		}
	}
}

func TestP1EXIFOrientationNormalizationPreservesSource(t *testing.T) {
	for orientation := 1; orientation <= 8; orientation++ {
		t.Run(string(rune('0'+orientation)), func(t *testing.T) {
			root := t.TempDir()
			input := filepath.Join(root, "0000.jpg")
			source := makeEXIFJPEG(t, orientation, binary.LittleEndian)
			if err := os.WriteFile(input, source, 0600); err != nil {
				t.Fatal(err)
			}
			sourceSum := sha256.Sum256(source)
			normalized, err := preparePDFPageOrientation(context.Background(), []string{input}, t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			if len(normalized) != 1 {
				t.Fatalf("unexpected normalized pages: %v", normalized)
			}
			if orientation == 1 {
				if normalized[0] != input {
					t.Fatal("normal orientation should preserve original JPEG")
				}
			} else {
				if normalized[0] == input || filepath.Ext(normalized[0]) != ".png" {
					t.Fatalf("rotated JPEG should use isolated PNG, got %q", normalized[0])
				}
				f, err := os.Open(normalized[0])
				if err != nil {
					t.Fatal(err)
				}
				out, decodeErr := png.Decode(f)
				_ = f.Close()
				if decodeErr != nil {
					t.Fatal(decodeErr)
				}
				w, h := 30, 12
				if orientation >= 5 {
					w, h = 12, 30
				}
				if out.Bounds().Dx() != w || out.Bounds().Dy() != h {
					t.Fatalf("orientation %d wrong dimensions: %v", orientation, out.Bounds())
				}
				// This source corner has an unambiguous destination for
				// each EXIF transform, including mirrors.
				corners := [9]image.Point{
					{}, {0, 0}, {29, 0}, {29, 11}, {0, 11},
					{0, 0}, {11, 0}, {11, 29}, {0, 29},
				}
				original, err := jpeg.Decode(bytes.NewReader(source))
				if err != nil {
					t.Fatal(err)
				}
				expectedR, expectedG, expectedB, expectedA := original.At(0, 0).RGBA()
				gotR, gotG, gotB, gotA := out.At(corners[orientation].X, corners[orientation].Y).RGBA()
				if gotR != expectedR || gotG != expectedG || gotB != expectedB || gotA != expectedA {
					t.Fatalf("orientation %d mapped corner to wrong pixel", orientation)
				}
			}
			check, err := os.ReadFile(input)
			if err != nil || sha256.Sum256(check) != sourceSum {
				t.Fatalf("source JPEG was modified: %v", err)
			}
		})
	}
}

func TestP1EXIFInvalidOrientationFailsWithoutOutput(t *testing.T) {
	for _, orientation := range []int{0, 9} {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "0000.jpg"), makeEXIFJPEG(t, orientation, binary.LittleEndian), 0600); err != nil {
			t.Fatal(err)
		}
		outputDir := t.TempDir()
		output := filepath.Join(outputDir, "bad.pdf")
		if err := createPDFVerified(output, root, 1, 1); err == nil {
			t.Fatalf("orientation %d should fail closed", orientation)
		}
		if _, err := os.Stat(output); !os.IsNotExist(err) {
			t.Fatalf("partial output exists: %v", err)
		}
	}
}

func TestP1EXIFRotatedPDFMediaBox(t *testing.T) {
	root := t.TempDir()
	for i, orientation := range []int{1, 6, 8} {
		filename := filepath.Join(root, []string{"0000.jpg", "0001.jpg", "0002.jpg"}[i])
		if err := os.WriteFile(filename, makeEXIFJPEG(t, orientation, binary.BigEndian), 0600); err != nil {
			t.Fatal(err)
		}
	}
	outputDir := t.TempDir()
	output := filepath.Join(outputDir, "rotated.pdf")
	if err := createPDFVerified(output, root, 3, 2); err != nil {
		t.Fatalf("EXIF-aware PDF conversion failed: %v", err)
	}
	if err := api.ValidateFile(output, nil); err != nil {
		t.Fatal(err)
	}
	dims, err := api.PageDimsFile(output)
	if err != nil || len(dims) != 3 {
		t.Fatalf("PDF page dimensions: %v %v", dims, err)
	}
	for i, want := range []float64{30.0 / 12.0, 12.0 / 30.0, 12.0 / 30.0} {
		got := dims[i].Width / dims[i].Height
		if math.Abs(got-want) > 0.02 {
			t.Errorf("PDF page %d ratio %.4f, want %.4f", i+1, got, want)
		}
	}
	files, err := os.ReadDir(outputDir)
	if err != nil || len(files) != 1 || files[0].Name() != "rotated.pdf" {
		t.Fatalf("staging artifacts leaked: files=%v, error=%v", files, err)
	}
}

func TestP1EXIFNormalizationCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := preparePDFPageOrientation(ctx, []string{"unused.jpg"}, t.TempDir()); err != context.Canceled {
		t.Fatalf("expected cancellation, got %v", err)
	}
}
