package main

import (
	"fmt"
	"net/url"
	"path"
	"strconv"
	"strings"

	"github.com/asaskevich/govalidator"
)

func prepareDownload(anyflipURL *url.URL) (*flipbook, error) {
	if anyflipURL == nil {
		return nil, fmt.Errorf("AnyFlip URL is required")
	}
	var newFlipbook flipbook

	canonicalURL, err := validateBookURL(anyflipURL.String())
	if err != nil {
		return nil, err
	}
	newFlipbook.URL = canonicalURL

	configjs, err := downloadConfigJSFile(canonicalURL)
	if err != nil {
		return nil, err
	}

	if title == "" {
		title, err = getBookTitle(configjs)
		if err != nil {
			title = path.Base(canonicalURL.Path)
		}
	}

	safeTitle := govalidator.SafeFileName(title)
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
	pageFileNames, err := getPageFileNames(configjs)
	if err != nil {
		return nil, err
	}
	if err := validateBookPages(newFlipbook.pageCount, pageFileNames); err != nil {
		return nil, err
	}

	downloadURL, _ := url.Parse("https://online.anyflip.com/")
	println(newFlipbook.URL.String())
	if len(pageFileNames) == 0 {
		for i := 1; i <= newFlipbook.pageCount; i++ {
			downloadURL.Path = path.Join(canonicalURL.Path, "files", "mobile", strconv.Itoa(i)+".jpg")
			newFlipbook.pageURLs = append(newFlipbook.pageURLs, downloadURL.String())
		}
	} else {
		for i := 0; i < newFlipbook.pageCount; i++ {
			imagePath, err := safePagePath(canonicalURL.Path, pageFileNames[i])
			if err != nil {
				return nil, fmt.Errorf("page %d: %w", i+1, err)
			}
			downloadURL.Path = imagePath
			newFlipbook.pageURLs = append(newFlipbook.pageURLs, downloadURL.String())
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
