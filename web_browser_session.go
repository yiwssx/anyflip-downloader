package main

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	browserSessionCookie = "anyflip_browser_session"
	browserSessionTTL = 12 * time.Hour
)

// A browser session is intentionally not a personal login. It is available
// only when an administrator explicitly enables trusted-network browser
// access and restricts the deployment using a firewall, VPN or reverse proxy.
// The administrator's API Bearer token is never sent to the browser.
func (s *webServer) issueBrowserSession(w http.ResponseWriter, r *http.Request) error {
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return fmt.Errorf("create browser session: %w", err)
	}
	expires := time.Now().Add(browserSessionTTL)
	unsigned := strconv.FormatInt(expires.Unix(), 10) + "." + hex.EncodeToString(nonce[:])
	mac := hmac.New(sha256.New, []byte(s.token))
	_, _ = mac.Write([]byte(unsigned))
	http.SetCookie(w, &http.Cookie{
		Name: browserSessionCookie,
		Value: unsigned + "." + hex.EncodeToString(mac.Sum(nil)),
		Path: "/",
		MaxAge: int(browserSessionTTL / time.Second),
		Expires: expires,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure: r.TLS != nil || s.browserCookieSecure,
	})
	return nil
}

// The browser cookie is signed with the server-only credential. It is not
// transmitted in a JS variable, localStorage or an Authorization header.
func (s *webServer) validBrowserSession(r *http.Request) bool {
	if !s.browserAccess || !sameOriginBrowserRequest(r) {
		return false
	}
	cookie, err := r.Cookie(browserSessionCookie)
	if err != nil || len(cookie.Value) > 160 {
		return false
	}
	parts := strings.Split(cookie.Value, ".")
	if len(parts) != 3 || len(parts[1]) != 32 {
		return false
	}
	expires, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || expires <= time.Now().Unix() ||
		expires > time.Now().Add(browserSessionTTL+time.Second).Unix() {
		return false
	}
	if _, err := hex.DecodeString(parts[1]); err != nil {
		return false
	}
	signature, err := hex.DecodeString(parts[2])
	if err != nil || len(signature) != sha256.Size {
		return false
	}
	mac := hmac.New(sha256.New, []byte(s.token))
	_, _ = mac.Write([]byte(parts[0] + "." + parts[1]))
	return hmac.Equal(signature, mac.Sum(nil))
}

// Prevent browser cross-origin requests from using an automatically issued
// cookie. Do not trust X-Forwarded-* headers supplied by arbitrary clients.
// Host matching works behind an HTTP reverse proxy with a preserved Host;
// Sec-Fetch-Site is supplied by modern browsers, not page scripts.
func sameOriginBrowserRequest(r *http.Request) bool {
	site := r.Header.Get("Sec-Fetch-Site")
	if site != "" && site != "same-origin" && site != "none" {
		return false
	}
	origin := r.Header.Get("Origin")
	if origin != "" {
		u, err := url.Parse(origin)
		if err != nil || u.Host != r.Host || (u.Scheme != "http" && u.Scheme != "https") ||
			u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
			return false
		}
		if r.TLS != nil && u.Scheme != "https" {
			return false
		}
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead &&
		origin == "" && site != "same-origin" {
		return false
	}
	return true
}
