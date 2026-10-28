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

// getPageImageChoices parses publisher-supplied page alternatives without
// creating or guessing additional remote image paths. Keep the metadata order
// and bound alternatives per page to prevent unbounded upstream requests.
func getPageImageChoices(configjs string) ([][]string, error) {
	pattern := regexp.MustCompile(`"n"\s*:`)
	matches := pattern.FindAllStringIndex(configjs, -1)
	if len(matches) == 0 {
		return nil, nil
	}
	choicesByPage := make([][]string, 0, len(matches))
	for i, match := range matches {
		var choices []string
		decoder := json.NewDecoder(strings.NewReader(configjs[match[1]:]))
		if err := decoder.Decode(&choices); err != nil {
			return nil, fmt.Errorf("invalid page image metadata at index %d: %w", i+1, err)
		}
		if len(choices) == 0 || len(choices) > 8 {
			return nil, fmt.Errorf("page %d must have 1-8 image choices", i+1)
		}
		for _, name := range choices {
			if strings.TrimSpace(name) == "" {
				return nil, fmt.Errorf("empty image choice at page %d", i+1)
			}
		}
		choicesByPage = append(choicesByPage, choices)
	}
	return choicesByPage, nil
}

// getPageFileNames preserves the existing primary-filename API for callers.
// The first publisher-provided choice remains preferred for compatibility.
func getPageFileNames(configjs string) ([]string, error) {
	choices, err := getPageImageChoices(configjs)
	if err != nil || len(choices) == 0 {
		return nil, err
	}
	names := make([]string, 0, len(choices))
	for _, page := range choices {
		names = append(names, page[0])
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
