package main

import (
	"reflect"
	"strings"
	"testing"
)

func TestGetPageFileNames(t *testing.T) {
	tests := []struct {
		name    string
		config  string
		want    []string
		wantErr bool
	}{
		{"missing names uses mobile fallback", `{"totalPageCount":2}`, nil, false},
		{"single filename", `{"fliphtml5_pages":[{"n":["page1.webp"]}]}`, []string{"page1.webp"}, false},
		{"separate names retain order", `{"fliphtml5_pages":[{"n":["a.webp"]},{"n":["b.webp"]}]}`, []string{"a.webp", "b.webp"}, false},
		{"multiple alternatives are one page", `{"fliphtml5_pages":[{"n":["high.webp","thumb.jpg"]},{"n":["second.webp"]}]}`, []string{"high.webp", "second.webp"}, false},
		{"spaces around colon", `{"fliphtml5_pages":[{"n" : ["1.jpg"]}]}`, []string{"1.jpg"}, false},
		{"bracket inside filename", `{"fliphtml5_pages":[{"n":["page[1].webp"]}]}`, []string{"page[1].webp"}, false},
		{"invalid JSON array", `{"fliphtml5_pages":[{"n":["missing",}]}`, nil, true},
		{"empty primary name", `{"fliphtml5_pages":[{"n":[""]}]}`, nil, true},
		{"empty array", `{"fliphtml5_pages":[{"n":[]}]}`, nil, true},
		{"invalid array type", `{"fliphtml5_pages":[{"n":[1,2]}]}`, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getPageFileNames(tt.config)
			if (err != nil) != tt.wantErr {
				t.Fatalf("getPageFileNames error=%v, wantErr=%v", err, tt.wantErr)
			}
			if !tt.wantErr && !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetPageFileNamesPreservesUnicodeAndQuotedNames(t *testing.T) {
	input := `{"fliphtml5_pages":[{"n":["หนังสือ\\u00201.webp"]}]}`
	got, err := getPageFileNames(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || !strings.Contains(got[0], "หนังสือ") {
		t.Fatalf("unexpected decoded filename: %v", got)
	}
}
