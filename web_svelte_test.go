package main

import (
    "io"
    "net/http"
    "net/http/httptest"
    "regexp"
    "strings"
    "testing"
)

func TestWebSvelteFrontendModeFailClosed(t *testing.T) {
    t.Setenv("ANYFLIP_WEB_UI", "bogus")
    if _, err := newWebServer(testWebToken, t.TempDir()); err == nil {
        t.Fatal("unknown Svelte frontend mode must fail startup")
    }

    t.Setenv("ANYFLIP_WEB_UI", "svelte")
    buildErr := validateSvelteBundle()
    s, err := newWebServer(testWebToken, t.TempDir())
    if buildErr != nil {
        if err == nil {
            t.Fatal("Svelte mode accepted an incomplete bundle")
        }
        return
    }
    if err != nil {
        t.Fatalf("valid Svelte bundle refused: %v", err)
    }
    if s.frontendMode != "svelte" {
        t.Fatalf("expected Svelte mode, got %q", s.frontendMode)
    }
}

func TestWebSvelteFrontendAssetsAndCSP(t *testing.T) {
    // Default remains the legacy HTML; no Svelte bundle/build is needed.
    t.Setenv("ANYFLIP_WEB_UI", "")
    legacy, err := newWebServer(testWebToken, t.TempDir())
    if err != nil {t.Fatal(err)}
    ts := httptest.NewServer(legacy.handler())
    legacyHome := webRequest(t, ts, "GET", "/", nil, false)
    bytes, err := io.ReadAll(legacyHome.Body)
    legacyHome.Body.Close()
    if err != nil || !strings.Contains(string(bytes), `id="jobForm"`) {
        t.Fatal("existing legacy default UI unexpectedly changed")
    }
    unknownAsset := webRequest(t, ts, "GET", "/svelte/assets/no.js", nil, false)
    unknownAsset.Body.Close()
    if unknownAsset.StatusCode != http.StatusNotFound {
        t.Fatal("Svelte assets must not be exposed in legacy mode")
    }
    ts.Close()

    if err := validateSvelteBundle(); err != nil {
        // Ordinary Go builds intentionally do not require Node/Vite artifacts.
        return
    }

    t.Setenv("ANYFLIP_WEB_UI", "svelte")
    candidate, err := newWebServer(testWebToken, t.TempDir())
    if err != nil {t.Fatal(err)}
    server := httptest.NewServer(candidate.handler())
    defer server.Close()

    home := webRequest(t, server, "GET", "/", nil, false)
    if home.StatusCode != http.StatusOK {
        t.Fatalf("Svelte home HTTP %d", home.StatusCode)
    }
    html, err := io.ReadAll(home.Body)
    csp := home.Header.Get("Content-Security-Policy")
    cache := home.Header.Get("Cache-Control")
    home.Body.Close()
    if err != nil {t.Fatal(err)}
    if !strings.Contains(string(html), "/svelte/assets/") {
        t.Fatal("Svelte entrypoint missing same-origin assets")
    }
    if cache!="no-store" || !strings.Contains(csp,"script-src 'self'") ||
       !strings.Contains(csp,"style-src 'self'") ||
       strings.Contains(csp,"'unsafe-inline'") || strings.Contains(csp,"'unsafe-eval'"){
        t.Fatalf("Svelte candidate weakened CSP/cache: %s / %s",csp,cache)
    }

    re:=regexp.MustCompile(`/svelte/assets/[A-Za-z0-9._-]+\.(js|css)`)
    refs:=re.FindAllString(string(html),-1)
    if len(refs)<2 {t.Fatalf("missing candidate assets %v",refs)}
    for _,asset:=range refs {
        resp:=webRequest(t,server,"GET",asset,nil,false)
        if resp.StatusCode!=200 {
            resp.Body.Close()
            t.Fatalf("%s: HTTP %d",asset,resp.StatusCode)
        }
        if resp.Header.Get("Cache-Control")!="no-store" || resp.Header.Get("Content-Security-Policy")!=csp {
            t.Fatalf("candidate asset headers differ: %s",asset)
        }
        if strings.HasSuffix(asset,".js") && !strings.HasPrefix(resp.Header.Get("Content-Type"),"text/javascript") {
            t.Fatalf("script has wrong content type: %s",resp.Header.Get("Content-Type"))
        }
        if strings.HasSuffix(asset,".css") && !strings.HasPrefix(resp.Header.Get("Content-Type"),"text/css") {
            t.Fatalf("style has wrong content type: %s",resp.Header.Get("Content-Type"))
        }
        resp.Body.Close()
    }
    // Vite rewrites public icon URLs under its /svelte/ base. These exact
    // institutional SVGs must be available, but no other web/svelte files.
    for _,brandPath:=range []string{"/svelte/favicon.svg","/svelte/logo.svg"} {
        resp:=webRequest(t,server,"GET",brandPath,nil,false)
        if resp.StatusCode!=http.StatusOK {
            resp.Body.Close()
            t.Fatalf("%s missing after Vite base rewrite: HTTP %d",brandPath,resp.StatusCode)
        }
        if resp.Header.Get("Content-Type")!="image/svg+xml" ||
           resp.Header.Get("Cache-Control")!="no-store" {
            resp.Body.Close()
            t.Fatalf("%s has incorrect mime/cache headers",brandPath)
        }
        body,readErr:=io.ReadAll(resp.Body)
        resp.Body.Close()
        if readErr!=nil || !strings.Contains(string(body),"<svg") {
            t.Fatalf("%s is not a valid embedded SVG",brandPath)
        }
    }
    for _,path:=range []string{"/svelte/assets/../README.md","/svelte/assets/secret.env","/svelte/assets/notfound.js","/svelte/README.md"}{
        resp:=webRequest(t,server,"GET",path,nil,false)
        resp.Body.Close()
        if resp.StatusCode==200 {t.Fatalf("unexpected asset exposure at %s",path)}
    }
    unauthenticated:=webRequest(t,server,"GET","/api/jobs",nil,false)
    unauthenticated.Body.Close()
    if unauthenticated.StatusCode!=http.StatusUnauthorized {
        t.Fatalf("Svelte UI bypassed Go API authorization: HTTP %d",unauthenticated.StatusCode)
    }
}
