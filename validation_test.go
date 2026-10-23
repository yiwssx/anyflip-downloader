package main

import (
	"testing"
)

func TestValidateBookURLAccepted(t *testing.T) {
	cases := []string{
		"https://anyflip.com/qrfai/tfvn",
		"https://online.anyflip.com/qrfai/tfvn/",
		"https://www.anyflip.com/qrfai/tfvn/mobile/index.html?unused=yes#page=2",
		"http://anyflip.com/qrfai/tfvn",
		" HTTPS://ANYFLIP.COM/qrfai/tfvn ",
	}
	const want = "https://online.anyflip.com/qrfai/tfvn"
	for _, input := range cases {
		t.Run(input, func(t *testing.T) {
			got, err := validateBookURL(input)
			if err != nil {
				t.Fatalf("expected valid input; got %v", err)
			}
			if got.String() != want {
				t.Fatalf("got %s, want %s", got, want)
			}
		})
	}
}

func TestValidateBookURLRejected(t *testing.T) {
	cases := []string{
		"",
		"/qrfai/tfvn",
		"ftp://anyflip.com/qrfai/tfvn",
		"https://anyflip.com",
		"https://anyflip.com/qrfai",
		"https://evil.example/qrfai/tfvn",
		"https://anyflip.com.evil.example/qrfai/tfvn",
		"https://evil.anyflip.com/qrfai/tfvn",
		"https://user:password@anyflip.com/qrfai/tfvn",
		"https://anyflip.com:8443/qrfai/tfvn",
		"https://anyflip.com/a/../b",
		"https://anyflip.com/a/b/%2e%2e",
		"https://anyflip.com/a%2fb/c",
		"https://anyflip.com/a/b\\c",
	}
	for _, input := range cases {
		t.Run(input, func(t *testing.T) {
			if u, err := validateBookURL(input); err == nil {
				t.Fatalf("expected %q to be rejected; got URL %s", input, u)
			}
		})
	}
}

func TestValidateBookPages(t *testing.T) {
	cases := []struct {
		name      string
		count     int
		pageNames []string
		wantError bool
	}{
		{"numbered image fallback", 120, nil, false},
		{"exact filename map", 2, []string{"1.jpg", "2.jpg"}, false},
		{"extra image names", 2, []string{"1.jpg", "2.jpg", "3.jpg"}, false},
		{"zero pages", 0, nil, true},
		{"negative page count", -2, nil, true},
		{"huge book", 10001, nil, true},
		{"insufficient filenames", 3, []string{"1.jpg", "2.jpg"}, true},
		{"empty filename", 2, []string{"1.jpg", ""}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateBookPages(tc.count, tc.pageNames)
			if (err != nil) != tc.wantError {
				t.Fatalf("got error %v, wantError=%v", err, tc.wantError)
			}
		})
	}
}
