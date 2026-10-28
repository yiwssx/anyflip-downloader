package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
)

var acceptedPageExt = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".webp": true,
}

// collectOrderedPageImages does not use lexical ordering of arbitrary
// directory contents. Every expected page must be present exactly once.
func collectOrderedPageImages(dir string, expectedPages int) ([]string, error) {
	if expectedPages < 1 || expectedPages > 10000 {
		return nil, fmt.Errorf("invalid expected page count %d", expectedPages)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read page images: %w", err)
	}
	ordered := make([]string, expectedPages)
	for _, entry := range entries {
		name := entry.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if !acceptedPageExt[ext] {
			continue
		}
		stem := strings.TrimSuffix(name, filepath.Ext(name))
		index, convErr := strconv.Atoi(stem)
		if len(stem) != 4 || convErr != nil || index < 0 ||
			index >= expectedPages || fmt.Sprintf("%04d", index) != stem {
			return nil, fmt.Errorf("unexpected image filename %q", name)
		}
		if ordered[index] != "" {
			return nil, fmt.Errorf("duplicate image for page %d", index+1)
		}
		info, err := os.Lstat(filepath.Join(dir, name))
		if err != nil {
			return nil, fmt.Errorf("inspect image %q: %w", name, err)
		}
		if !info.Mode().IsRegular() || info.Size() == 0 {
			return nil, fmt.Errorf("page %d is not a non-empty regular file", index+1)
		}
		ordered[index] = filepath.Join(dir, name)
	}
	for i, file := range ordered {
		if file == "" {
			return nil, fmt.Errorf("missing downloaded image for page %d of %d", i+1, expectedPages)
		}
	}
	return ordered, nil
}

// publishVerifiedFile uses a hard link instead of os.Rename so that an
// existing user-owned file can never be replaced, including on Unix.
// The temporary and final files must reside on the same filesystem.
func publishVerifiedFile(staging, output string) error {
	if err := os.Link(staging, output); err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("refusing to overwrite existing PDF %q: %w", output, err)
		}
		return fmt.Errorf("publish PDF atomically: %w", err)
	}
	return nil
}

// createPDFVerified imports a complete image sequence to an isolated
// staging folder and checks the resulting page count before publishing.
// A failed conversion never creates a partial final output file.
func createPDFVerified(outputFile, imageDir string, expectedPages, chunkSize int) error {
	return createPDFVerifiedContext(context.Background(), outputFile, imageDir, expectedPages, chunkSize, nil)
}

// createPDFVerifiedContext checks cancellation between import chunks, validates
// the completed PDF and publishes only after every check passes.
func createPDFVerifiedContext(ctx context.Context, outputFile, imageDir string, expectedPages, chunkSize int, onProgress func(completed int)) error {
	if ctx == nil {
		return fmt.Errorf("PDF conversion context is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if chunkSize < 1 || chunkSize > 1000 {
		return fmt.Errorf("chunk size must be between 1 and 1000")
	}
	pages, err := collectOrderedPageImages(imageDir, expectedPages)
	if err != nil {
		return err
	}
	destination, err := filepath.Abs(outputFile)
	if err != nil {
		return fmt.Errorf("invalid output filename: %w", err)
	}
	stageDir, err := os.MkdirTemp(filepath.Dir(destination), ".anyflip-pdf-stage-")
	if err != nil {
		return fmt.Errorf("create PDF staging folder: %w", err)
	}
	defer os.RemoveAll(stageDir)
	stagingFile := filepath.Join(stageDir, "result.pdf")
	// Apply JPEG EXIF rotations to isolated lossless PNG copies before
	// importing; the source images are never edited or replaced.
	pages, err = preparePDFPageOrientation(ctx, pages, stageDir)
	if err != nil {
		return err
	}
	configuration := pdfcpu.DefaultImportConfig()
	for i := 0; i < len(pages); i += chunkSize {
		if err := ctx.Err(); err != nil {
			return err
		}
		end := min(i+chunkSize, len(pages))
		if err := api.ImportImagesFile(pages[i:end], stagingFile, configuration, nil); err != nil {
			return fmt.Errorf("PDF conversion failed at pages %d-%d: %w", i+1, end, err)
		}
		if onProgress != nil {
			onProgress(end)
		}
	}
	actualPages, err := api.PageCountFile(stagingFile)
	if err != nil {
		return fmt.Errorf("generated PDF is invalid: %w", err)
	}
	if actualPages != expectedPages {
		return fmt.Errorf("generated PDF has %d pages, expected %d", actualPages, expectedPages)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.Chmod(stagingFile, 0600); err != nil {
		return fmt.Errorf("secure generated PDF: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return publishVerifiedFile(stagingFile, destination)
}
