package main

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

var bookIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// validateBookURL accepts only AnyFlip reader URLs and reduces them to the
// canonical HTTPS origin and the two identifiers of the book. Query strings,
// fragments and viewer-specific trailing paths are not sent to the server.
func validateBookURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, fmt.Errorf("invalid AnyFlip URL: %w", err)
	}
	if u.Opaque != "" || (u.Scheme != "https" && u.Scheme != "http") {
		return nil, fmt.Errorf("AnyFlip URL must start with http:// or https://")
	}
	if u.User != nil || u.Port() != "" {
		return nil, fmt.Errorf("AnyFlip URL cannot contain credentials or a custom port")
	}
	switch strings.ToLower(u.Hostname()) {
	case "anyflip.com", "www.anyflip.com", "online.anyflip.com":
	default:
		return nil, fmt.Errorf("URL must belong to anyflip.com")
	}

	// IDs are ASCII slugs. Reject percent-encoded separators, traversal and
	// ambiguity instead of trying to clean untrusted path components.
	if strings.Contains(u.EscapedPath(), "%") || strings.Contains(u.Path, "\\") {
		return nil, fmt.Errorf("AnyFlip URL contains unsupported path escaping")
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 || !bookIDPattern.MatchString(parts[0]) || !bookIDPattern.MatchString(parts[1]) {
		return nil, fmt.Errorf("AnyFlip URL must contain publisher and book IDs")
	}
	for _, segment := range parts[2:] {
		if segment == "." || segment == ".." {
			return nil, fmt.Errorf("AnyFlip URL contains path traversal")
		}
	}
	return &url.URL{
		Scheme: "https",
		Host:   "online.anyflip.com",
		Path:   "/" + parts[0] + "/" + parts[1],
	}, nil
}

// validateBookPages rejects incomplete or malformed book metadata before any
// index access or network requests. A book may use numbered mobile images
// instead of a filename map (len(pageNames)==0).
func validateBookPages(pageCount int, pageNames []string) error {
	if pageCount < 1 || pageCount > 10000 {
		return fmt.Errorf("invalid book page count %d (expected 1-10000)", pageCount)
	}
	if len(pageNames) > 0 && len(pageNames) < pageCount {
		return fmt.Errorf("book metadata lists %d images for %d pages", len(pageNames), pageCount)
	}
	for i := 0; i < pageCount && len(pageNames) > 0; i++ {
		if strings.TrimSpace(pageNames[i]) == "" {
			return fmt.Errorf("book metadata has an empty image name at page %d", i+1)
		}
	}
	return nil
}
