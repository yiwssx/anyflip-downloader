package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestTrustedNetworkBrowserSessionAndAPI(t *testing.T) {
	s, err := newWebServer(testWebToken, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s.browserAccess = true
	s.browserCookieSecure = true
	h := s.handler()

	makeRequest := func(method, path string, cookie *http.Cookie, origin, site, bearer string) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, "http://anyflip.test"+path, nil)
		if cookie != nil {
			r.AddCookie(cookie)
		}
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		if site != "" {
			r.Header.Set("Sec-Fetch-Site", site)
		}
		if bearer != "" {
			r.Header.Set("Authorization", "Bearer "+bearer)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}

	unauthenticated := makeRequest("GET", "/api/jobs", nil, "", "", "")
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("uncredentialed API returned %d, want 401", unauthenticated.Code)
	}

	page := makeRequest("GET", "/", nil, "", "none", "")
	if page.Code != http.StatusOK {
		t.Fatalf("page returned %d", page.Code)
	}
	body, err := io.ReadAll(page.Result().Body)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "id=\"token\"") || strings.Contains(string(body), testWebToken) {
		t.Fatal("Web UI exposed an administrator token field or secret")
	}
	cookies := page.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("wanted a single browser cookie, got %d", len(cookies))
	}
	cookie := cookies[0]
	if cookie.Name != browserSessionCookie || !cookie.HttpOnly ||
		!cookie.Secure || cookie.SameSite != http.SameSiteStrictMode ||
		cookie.Path != "/" {
		t.Fatalf("insecure browser cookie properties: %+v", cookie)
	}

	if got := makeRequest("GET", "/api/jobs", cookie, "", "same-origin", "").Code; got != http.StatusOK {
		t.Fatalf("same-origin cookie request returned %d", got)
	}
	if got := makeRequest("POST", "/api/jobs", cookie, "http://anyflip.test", "same-origin", "").Code; got != http.StatusBadRequest {
		t.Fatalf("authorized browser POST returned %d, want 400 for absent JSON", got)
	}
	if got := makeRequest("POST", "/api/jobs", cookie, "https://attacker.example", "cross-site", "").Code; got != http.StatusUnauthorized {
		t.Fatalf("cross-origin cookie POST returned %d", got)
	}
	if got := makeRequest("POST", "/api/jobs", cookie, "", "", "").Code; got != http.StatusUnauthorized {
		t.Fatalf("missing origin/fetch metadata cookie POST returned %d", got)
	}
	if got := makeRequest("GET", "/api/jobs", cookie, "", "cross-site", "").Code; got != http.StatusUnauthorized {
		t.Fatalf("cross-site cookie GET returned %d", got)
	}
	bad := *cookie
	bad.Value = cookie.Value[:len(cookie.Value)-1] + "0"
	if got := makeRequest("GET", "/api/jobs", &bad, "", "", "").Code; got != http.StatusUnauthorized {
		t.Fatalf("tampered browser cookie returned %d", got)
	}
	if got := makeRequest("GET", "/api/jobs", nil, "", "", testWebToken).Code; got != http.StatusOK {
		t.Fatalf("existing administrator Bearer token was rejected: %d", got)
	}

	disabled, err := newWebServer(testWebToken, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("GET", "/api/jobs", nil)
	r.AddCookie(cookie)
	w := httptest.NewRecorder()
	disabled.handler().ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("default-disabled browser access returned %d", w.Code)
	}
}

func TestExpiredBrowserSessionRejected(t *testing.T) {
	s, err := newWebServer(testWebToken, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s.browserAccess = true
	for _, expiry := range []int64{
		time.Now().Add(-time.Minute).Unix(),
		time.Now().Add(24 * time.Hour).Unix(),
	} {
		payload := strconv.FormatInt(expiry, 10) + "." + strings.Repeat("a", 32)
		mac := hmac.New(sha256.New, []byte(testWebToken))
		_, _ = mac.Write([]byte(payload))
		cookie := &http.Cookie{Name: browserSessionCookie, Value: payload + "." + hex.EncodeToString(mac.Sum(nil))}
		r := httptest.NewRequest("GET", "http://anyflip.test/api/jobs", nil)
		r.AddCookie(cookie)
		w := httptest.NewRecorder()
		s.handler().ServeHTTP(w, r)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("invalid session expiry %d returned %d", expiry, w.Code)
		}
	}
}
