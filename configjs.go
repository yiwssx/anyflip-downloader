package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// getPageFileNames selects the primary image filename for each page's "n"
// array. Multiple strings within one page are alternate representations,
// not additional pages. Missing arrays allow the legacy numbered-image mode.
func getPageFileNames(configjs string) ([]string, error) {
	pattern := regexp.MustCompile(`"n"\s*:`)
	matches := pattern.FindAllStringIndex(configjs, -1)
	if len(matches) == 0 {
		return nil, nil
	}
	names := make([]string, 0, len(matches))
	for i, match := range matches {
		var choices []string
		decoder := json.NewDecoder(strings.NewReader(configjs[match[1]:]))
		if err := decoder.Decode(&choices); err != nil {
			return nil, fmt.Errorf("invalid page image metadata at index %d: %w", i+1, err)
		}
		if len(choices) == 0 || strings.TrimSpace(choices[0]) == "" {
			return nil, fmt.Errorf("missing primary page image name at index %d", i+1)
		}
		names = append(names, choices[0])
	}
	return names, nil
}

func getBookTitle(configjs string) (string, error) {
	r := regexp.MustCompile(`("?(bookConfig.)?bookTitle"?[=]"(.*?)")|"title":"(.*?)"`)
	match := r.FindString(configjs)

	if strings.Contains(match, "=") {
		match = strings.Split(match, "=")[1]
	} else if strings.Contains(match, ":") {
		match = strings.Split(match, ":")[1]
	} else {
		return "", errors.New("could not find book title")
	}
	match = strings.ReplaceAll(match, "\"", "")

	return match, nil
}

func getPageCount(configjs string) (int, error) {
	r := regexp.MustCompile("\"?(bookConfig\\.)?(total)?[Pp]ageCount\"?[=:]\"?\\d+\"?")
	match := r.FindString(configjs)

	if strings.Contains(match, "=") {
		match = strings.Split(match, "=")[1]
	} else if strings.Contains(match, ":") {
		match = strings.Split(match, ":")[1]
	} else {
		return 0, errors.New("could not find page count")
	}

	match = strings.ReplaceAll(match, "\"", "")
	return strconv.Atoi(match)
}

func downloadConfigJSFile(bookURL *url.URL) (string, error) {
	return downloadConfigJSFileContext(context.Background(), newAnyFlipHTTPClient(), bookURL)
}
