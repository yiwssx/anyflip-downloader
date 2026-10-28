package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"

	"github.com/asaskevich/govalidator"
)

// prepareDownload keeps the CLI entry point without mutating global title state.
func prepareDownload(anyflipURL *url.URL) (*flipbook, error) {
	return prepareDownloadContext(context.Background(), anyflipURL, title, nil)
}

// prepareDownloadContext can be called independently by simultaneous jobs.
func prepareDownloadContext(ctx context.Context, anyflipURL *url.URL, desiredTitle string, client *http.Client) (*flipbook, error) {
	if ctx == nil {
		return nil, fmt.Errorf("download context is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if anyflipURL == nil {
		return nil, fmt.Errorf("AnyFlip URL is required")
	}
	var newFlipbook flipbook

	canonicalURL, err := validateBookURL(anyflipURL.String())
	if err != nil {
		return nil, err
	}
	newFlipbook.URL = canonicalURL

	configjs, err := downloadConfigJSFileContext(ctx, client, canonicalURL)
	if err != nil {
		return nil, err
	}

	bookName := desiredTitle
	if bookName == "" {
		bookName, err = getBookTitle(configjs)
		if err != nil {
			bookName = path.Base(canonicalURL.Path)
		}
	}

	safeTitle := govalidator.SafeFileName(bookName)
	if safeTitle == "" {
		safeTitle = path.Base(canonicalURL.Path)
	}
	if safeTitle == "" || safeTitle == "." {
		safeTitle = "anyflip-download"
	}
	// Trim trailing dots/spaces - Windows strips these and causes "path not found"
	safeTitle = strings.Trim(strings.TrimSpace(safeTitle), ".")
	if safeTitle == "" {
		safeTitle = "anyflip-download"
	}
	newFlipbook.title = safeTitle
	newFlipbook.pageCount, err = getPageCount(configjs)
	if err != nil {
		return nil, fmt.Errorf("invalid AnyFlip book configuration: %w", err)
	}
	pageChoices, err := getPageImageChoices(configjs)
	if err != nil {
		return nil, err
	}
	var primaryNames []string
	for _, choices := range pageChoices {
		primaryNames = append(primaryNames, choices[0])
	}
	if err := validateBookPages(newFlipbook.pageCount, primaryNames); err != nil {
		return nil, err
	}

	downloadURL, _ := url.Parse("https://online.anyflip.com/")
	newFlipbook.pageFallbackURLs = make([][]string, newFlipbook.pageCount)
	if len(pageChoices) == 0 {
		for i := 1; i <= newFlipbook.pageCount; i++ {
			downloadURL.Path = path.Join(canonicalURL.Path, "files", "mobile", strconv.Itoa(i)+".jpg")
			newFlipbook.pageURLs = append(newFlipbook.pageURLs, downloadURL.String())
		}
	} else {
		for i := 0; i < newFlipbook.pageCount; i++ {
			// Validate every alternate before creating the job; an unsafe
			// secondary path must not become reachable after a 404.
			seen := make(map[string]bool)
			for choiceIndex, filename := range pageChoices[i] {
				imagePath, err := safePagePath(canonicalURL.Path, filename)
				if err != nil {
					return nil, fmt.Errorf("page %d choice %d: %w", i+1, choiceIndex+1, err)
				}
				downloadURL.Path = imagePath
				candidate := downloadURL.String()
				if seen[candidate] {
					continue
				}
				seen[candidate] = true
				if choiceIndex == 0 {
					newFlipbook.pageURLs = append(newFlipbook.pageURLs, candidate)
				} else {
					newFlipbook.pageFallbackURLs[i] = append(newFlipbook.pageFallbackURLs[i], candidate)
				}
			}
		}
	}

	return &newFlipbook, nil
}

// cleanDownloadURL removes problematic sequences from the download URL
// and resolves any "../" path traversal by traversing up the directory structure.
func cleanDownloadURL(rawURL string) string {
	decoded, err := url.PathUnescape(rawURL)
	if err != nil {
		decoded = rawURL
	}
	// Normalize backslashes to forward slashes before parsing
	decoded = strings.ReplaceAll(decoded, "\\", "/")

	u, err := url.Parse(decoded)
	if err != nil {
		return decoded
	}

	// Clean the path: resolves ".." and "." segments and removes double slashes
	u.Path = path.Clean(u.Path)

	// Deduplicate consecutive identical path segments produced by sites that
	// generate URLs like "files/large/../files/mobile/" where resolving ".."
	// lands back in "files/" and then appends another "files/" segment.
	segments := strings.Split(u.Path, "/")
	deduped := segments[:0]
	for i, seg := range segments {
		if i > 0 && seg == segments[i-1] && seg != "" {
			continue
		}
		deduped = append(deduped, seg)
	}
	u.Path = strings.Join(deduped, "/")

	return u.String()
}
