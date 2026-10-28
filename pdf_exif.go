package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
)

// jpegEXIFOrientation reads the first JPEG Exif IFD0 orientation tag.
// No external libraries or third-party URL requests are involved.
// A corrupt Exif structure fails closed rather than emitting a rotated page.
func jpegEXIFOrientation(data []byte) (int, error) {
	if len(data) < 4 || data[0] != 0xff || data[1] != 0xd8 {
		return 0, fmt.Errorf("invalid JPEG header")
	}
	for p := 2; p < len(data); {
		if data[p] != 0xff {
			break
		}
		for p < len(data) && data[p] == 0xff {
			p++
		}
		if p >= len(data) {
			return 0, fmt.Errorf("truncated JPEG marker")
		}
		marker := data[p]
		p++
		if marker == 0xda || marker == 0xd9 {
			break // compressed image data / end of image
		}
		if marker == 0x01 || marker == 0xd8 || (marker >= 0xd0 && marker <= 0xd7) {
			continue
		}
		if p+2 > len(data) {
			return 0, fmt.Errorf("truncated JPEG segment")
		}
		segmentLen := int(binary.BigEndian.Uint16(data[p : p+2]))
		if segmentLen < 2 || segmentLen > len(data)-p {
			return 0, fmt.Errorf("invalid JPEG segment length")
		}
		payload := data[p+2 : p+segmentLen]
		if marker == 0xe1 && bytes.HasPrefix(payload, []byte("Exif\x00\x00")) {
			return parseEXIFOrientation(payload[6:])
		}
		p += segmentLen
	}
	return 1, nil
}

func parseEXIFOrientation(tiff []byte) (int, error) {
	if len(tiff) < 8 {
		return 0, fmt.Errorf("truncated Exif TIFF header")
	}
	var order binary.ByteOrder
	switch string(tiff[:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return 0, fmt.Errorf("invalid Exif byte order")
	}
	if order.Uint16(tiff[2:4]) != 42 {
		return 0, fmt.Errorf("invalid Exif TIFF magic")
	}
	ifd := uint64(order.Uint32(tiff[4:8]))
	if ifd > uint64(len(tiff)) || uint64(len(tiff))-ifd < 2 {
		return 0, fmt.Errorf("invalid Exif IFD offset")
	}
	count := uint64(order.Uint16(tiff[ifd : ifd+2]))
	entryStart := ifd + 2
	if count > (uint64(len(tiff))-entryStart)/12 {
		return 0, fmt.Errorf("truncated Exif IFD entries")
	}
	for i := uint64(0); i < count; i++ {
		p := entryStart + i*12
		entry := tiff[p : p+12]
		if order.Uint16(entry[:2]) != 0x0112 {
			continue
		}
		// Orientation is a single SHORT stored inline in the IFD entry.
		if order.Uint16(entry[2:4]) != 3 || order.Uint32(entry[4:8]) != 1 {
			return 0, fmt.Errorf("invalid Exif orientation type")
		}
		value := int(order.Uint16(entry[8:10]))
		if value < 1 || value > 8 {
			return 0, fmt.Errorf("invalid Exif orientation value")
		}
		return value, nil
	}
	return 1, nil
}

// orientImage applies all eight standard EXIF orientation transformations,
// including mirrored orientations; the input image is never mutated.
func orientImage(src image.Image, orientation int) (*image.RGBA, error) {
	bounds := src.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if w < 1 || h < 1 {
		return nil, fmt.Errorf("invalid JPEG dimensions")
	}
	dw, dh := w, h
	if orientation >= 5 && orientation <= 8 {
		dw, dh = h, w
	}
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var dx, dy int
			switch orientation {
			case 1:
				dx, dy = x, y
			case 2:
				dx, dy = w-1-x, y
			case 3:
				dx, dy = w-1-x, h-1-y
			case 4:
				dx, dy = x, h-1-y
			case 5:
				dx, dy = y, x
			case 6:
				dx, dy = h-1-y, x
			case 7:
				dx, dy = h-1-y, w-1-x
			case 8:
				dx, dy = y, w-1-x
			default:
				return nil, fmt.Errorf("unsupported Exif orientation %d", orientation)
			}
			dst.Set(dx, dy, src.At(bounds.Min.X+x, bounds.Min.Y+y))
		}
	}
	return dst, nil
}

// preparePDFPageOrientation normalizes only JPEGs that explicitly request
// orientation 2-8. Other images and normal JPEGs preserve their original bytes.
// The lossless rotated PNG lives solely in the already-owned staging directory.
// Bounded pixels prevent malicious compressed images from exhausting memory.
func preparePDFPageOrientation(ctx context.Context, pages []string, stageDir string) ([]string, error) {
	normalized := make([]string, len(pages))
	copy(normalized, pages)
	for i, filename := range pages {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		ext := strings.ToLower(filepath.Ext(filename))
		if ext != ".jpg" && ext != ".jpeg" {
			continue
		}
		data, err := os.ReadFile(filename)
		if err != nil {
			return nil, fmt.Errorf("read JPEG page %d: %w", i+1, err)
		}
		if len(data) > int(maxPageBytes) {
			return nil, fmt.Errorf("JPEG page %d exceeds allowed bytes", i+1)
		}
		orientation, err := jpegEXIFOrientation(data)
		if err != nil {
			return nil, fmt.Errorf("inspect EXIF page %d: %w", i+1, err)
		}
		if orientation == 1 {
			continue
		}
		cfg, err := jpeg.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("decode JPEG dimensions for page %d: %w", i+1, err)
		}
		const maxOrientedPixels int64 = 25000000
		if cfg.Width < 1 || cfg.Height < 1 || int64(cfg.Width)*int64(cfg.Height) > maxOrientedPixels {
			return nil, fmt.Errorf("oriented JPEG page %d exceeds pixel limit", i+1)
		}
		src, err := jpeg.Decode(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("decode JPEG page %d: %w", i+1, err)
		}
		rotated, err := orientImage(src, orientation)
		if err != nil {
			return nil, fmt.Errorf("orient JPEG page %d: %w", i+1, err)
		}
		out := filepath.Join(stageDir, fmt.Sprintf("oriented-%04d.png", i))
		f, err := os.OpenFile(out, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return nil, err
		}
		encodeErr := png.Encode(f, rotated)
		closeErr := f.Close()
		if encodeErr != nil {
			return nil, fmt.Errorf("encode oriented page %d: %w", i+1, encodeErr)
		}
		if closeErr != nil {
			return nil, fmt.Errorf("close oriented page %d: %w", i+1, closeErr)
		}
		normalized[i] = out
	}
	return normalized, nil
}
