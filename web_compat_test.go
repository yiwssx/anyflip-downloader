package main

import (
	"context"
	"net/http"
	"net/url"
	"testing"
)

func TestPrepareDownloadContextRealAnyFlipRelativeImageNames(t *testing.T) {
	const config = `{"title":"Test Book","pageCount":2,"pages":[{"n":["../files/large/page-one.webp"]},{"n":["../files/large/page-two.webp"]}]}`
	client := &http.Client{Transport: mockRoundTrip(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/qrfai/tfvn/mobile/javascript/config.js" {
			t.Errorf("unexpected request path %q", req.URL.Path)
		}
		return mockResponse(req, http.StatusOK, config), nil
	})}
	bookURL, _ := url.Parse("https://anyflip.com/qrfai/tfvn")
	book, err := prepareDownloadContext(context.Background(), bookURL, "", client)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"https://online.anyflip.com/qrfai/tfvn/files/large/page-one.webp",
		"https://online.anyflip.com/qrfai/tfvn/files/large/page-two.webp",
	}
	if book.pageCount != len(want) || len(book.pageURLs) != len(want) {
		t.Fatalf("unexpected page metadata: count=%d urls=%d", book.pageCount, len(book.pageURLs))
	}
	for i, url := range want {
		if book.pageURLs[i] != url {
			t.Errorf("page %d URL = %q; want %q", i+1, book.pageURLs[i], url)
		}
	}
}
