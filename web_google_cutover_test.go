package main

import (
    "net/http"
    "net/http/httptest"
    "strings"
    "testing"
)

func TestGoogleCutoverNoLegacyToken(t *testing.T) {
    s, err := newWebServerForAuthMode("", t.TempDir(), "google")
    if err != nil { t.Fatalf("Google mode unexpectedly required legacy token: %v", err) }
    g, err := newGoogleAuth("ci-client", "ci-secret", "https://anyflip.rcat.ac.th/auth/google/callback")
    if err != nil { t.Fatal(err) }
    s.google = g
    handler := s.handler()

    request := func(method, target, host, proto string, bearer bool) *httptest.ResponseRecorder {
        t.Helper()
        r := httptest.NewRequest(method, target, nil)
        r.Host = host
        if proto != "" { r.Header.Set("X-Forwarded-Proto", proto) }
        if bearer { r.Header.Set("Authorization", "Bearer "+testWebToken) }
        w := httptest.NewRecorder()
        handler.ServeHTTP(w, r)
        return w
    }

    if got := request("GET", "http://anyflip.rcat.ac.th/", "anyflip.rcat.ac.th", "https", false); got.Code != http.StatusOK ||
        !strings.Contains(got.Body.String(), "mode-google") ||
        strings.Contains(got.Body.String(), "id=\"token\"") {
        t.Fatalf("canonical login not Google-only: %d", got.Code)
    }
    if got := request("GET", "http://anyflip.rcat.dpdns.org/", "anyflip.rcat.dpdns.org", "https", false); got.Code != http.StatusSeeOther ||
        got.Header().Get("Location") != "https://anyflip.rcat.ac.th/" {
        t.Fatalf("fallback hostname not redirected to canonical Google login: %d %s", got.Code, got.Header().Get("Location"))
    }
    if got := request("GET", "http://anyflip.rcat.dpdns.org/healthz", "anyflip.rcat.dpdns.org", "https", false); got.Code != http.StatusOK {
        t.Fatalf("fallback tunnel health regressed: %d", got.Code)
    }
    for _, bearer := range []bool{false,true} {
        if got := request("GET", "http://anyflip.rcat.ac.th/api/jobs", "anyflip.rcat.ac.th", "https", bearer); got.Code != http.StatusUnauthorized {
            t.Fatalf("Google mode accepted API without Google session (Bearer=%v): %d", bearer, got.Code)
        }
    }
    if got := request("GET", "http://anyflip.rcat.ac.th/auth/google", "anyflip.rcat.ac.th", "https", false); got.Code != http.StatusSeeOther ||
        !strings.HasPrefix(got.Header().Get("Location"), "https://accounts.google.com/") {
        t.Fatalf("Google sign-in did not initiate over trusted HTTPS: %d", got.Code)
    }
    if got := request("GET", "http://anyflip.rcat.ac.th/auth/google", "anyflip.rcat.ac.th", "http", false); got.Code != http.StatusForbidden {
        t.Fatalf("insecure origin was allowed to initiate Google sign-in: %d", got.Code)
    }
    if got := request("GET", "http://evil.example/auth/google", "evil.example", "https", false); got.Code != http.StatusForbidden {
        t.Fatalf("unconfigured hostname was allowed to initiate Google sign-in: %d", got.Code)
    }
}

func TestLegacyTokenModeStillFailsClosed(t *testing.T) {
    for _, mode := range []string{"token", "", "invalid"} {
        if _, err := newWebServerForAuthMode("", t.TempDir(), mode); err == nil {
            t.Fatalf("insecure auth mode %q accepted empty shared secret", mode)
        }
    }
    if _, err := newWebServerForAuthMode(testWebToken, t.TempDir(), "token"); err != nil {
        t.Fatalf("token-mode backwards compatibility broken: %v", err)
    }
}
