package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestP1CLIExplicitlyReportsImageBasedPDF(t *testing.T) {
	msg := completedPDFMessage("authorized.pdf")
	if !strings.Contains(msg, "authorized.pdf") ||
		!strings.Contains(msg, "Image-based PDF") ||
		!strings.Contains(msg, "not the publisher's original") {
		t.Fatalf("CLI output does not identify artifact limits: %q", msg)
	}
}

func TestP1WebPageExplainsImageBasedPDF(t *testing.T) {
	s, err := newWebServer(testWebToken, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(s.handler())
	defer ts.Close()
	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("web UI HTTP status=%d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	page := string(data)
	for _, text := range []string{
		"Image-based PDF",
		"ดาวน์โหลด Image-based PDF",
		"ไม่ใช่ PDF ต้นฉบับ",
	} {
		if !strings.Contains(page, text) {
			t.Fatalf("Web GUI does not explain output artifact with %q", text)
		}
	}
}
