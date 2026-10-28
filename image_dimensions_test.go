package main

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func syntheticPNGHeader(t *testing.T, width, height uint32) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(1, 1, color.RGBA{R: 255, A: 255})
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	data := append([]byte(nil), b.Bytes()...)
	// Preserve the standard PNG magic and IHDR structure, only change the
	// declared pixel dimensions and recalculate its CRC. The IDAT remains
	// deliberately tiny: DecodeConfig can read the header without inflating
	// any huge pixel buffer. No external images are used.
	if len(data) < 33 || string(data[12:16]) != "IHDR" {
		t.Fatal("unexpected PNG encoder header")
	}
	binary.BigEndian.PutUint32(data[16:20], width)
	binary.BigEndian.PutUint32(data[20:24], height)
	binary.BigEndian.PutUint32(data[29:33], crc32.ChecksumIEEE(data[12:29]))
	return data
}

func TestP3PageDimensionLimitIsOverflowSafe(t *testing.T) {
	for _, tc := range []struct {
		name string
		w, h int
		allowed bool
	}{
		{"ordinary scan", 2480, 3508, true},
		{"8K UHD", 7680, 4320, true},
		{"40MP boundary", 8000, 5000, true},
		{"one pixel over total", 8001, 5000, false},
		{"over width", 12001, 2, false},
		{"over height", 2, 12001, false},
		{"zero width", 0, 100, false},
		{"negative height", 100, -1, false},
		{"malicious huge dimensions", int(^uint(0) >> 1), int(^uint(0) >> 1), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validatePageDimensions(tc.w, tc.h)
			if (err == nil) != tc.allowed {
				t.Fatalf("dimensions %dx%d: allowed=%t; err=%v", tc.w, tc.h, tc.allowed, err)
			}
		})
	}
}

func TestP3OversizedPNGHeaderRejectedBeforePDFImport(t *testing.T) {
	for _, tc := range []struct {
		name string
		w, h uint32
	}{
		{"over total pixels", 10000, 5000},
		{"over side length", 12001, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload := syntheticPNGHeader(t, tc.w, tc.h)
			cfg, format, err := image.DecodeConfig(bytes.NewReader(payload))
			if err != nil || format != "png" || cfg.Width != int(tc.w) || cfg.Height != int(tc.h) {
				t.Fatalf("fixture header unreadable: cfg=%+v format=%s err=%v", cfg, format, err)
			}
			filename := filepath.Join(t.TempDir(), "0000.png")
			resp := &http.Response{Body: io.NopCloser(bytes.NewReader(payload)), ContentLength: int64(len(payload))}
			err = saveImageResponse(filename, resp)
			if err == nil || !strings.Contains(err.Error(), "image dimensions exceed supported limit") {
				t.Fatalf("oversized image accepted or wrong error: %v", err)
			}
			if _, statErr := os.Stat(filename); !os.IsNotExist(statErr) {
				t.Fatalf("rejected image left file on disk: %v", statErr)
			}
		})
	}
}

func TestP3NormalPNGStillAccepted(t *testing.T) {
	payload := syntheticPNGHeader(t, 2, 2)
	filename := filepath.Join(t.TempDir(), "0000.png")
	resp := &http.Response{Body: io.NopCloser(bytes.NewReader(payload)), ContentLength: int64(len(payload))}
	if err := saveImageResponse(filename, resp); err != nil {
		t.Fatalf("normal PNG unexpectedly rejected: %v", err)
	}
	got, err := os.ReadFile(filename)
	if err != nil || !bytes.Equal(got, payload) {
		t.Fatalf("accepted original image bytes changed: %v", err)
	}
}
