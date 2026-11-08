package main

import (
  "net/http"
  "net/http/httptest"
  "strings"
  "testing"
)

// Public JS assets must not accidentally expose routes in legacy-token mode.
// Real privileged data is gated by the Go authorize/RBAC middleware.
func TestAdminClientEmbeddedOnlyInGoogleMode(t *testing.T) {
  for _, google := range []bool{false,true} {
    s,err:=newWebServer(testWebToken,t.TempDir())
    if err!=nil {t.Fatal(err)}
    if google {
      s.google,err=newGoogleAuth("client","secret","https://rcat.example/auth/google/callback")
      if err!=nil {t.Fatal(err)}
    }
    w:=httptest.NewRecorder()
    s.handler().ServeHTTP(w,httptest.NewRequest(http.MethodGet,"http://rcat.example/admin-api.js",nil))
    expected:=http.StatusNotFound
    if google {expected=http.StatusOK}
    if w.Code!=expected {t.Fatalf("google=%v admin-api.js HTTP %d; expected %d",google,w.Code,expected)}
    if google {
      if got:=w.Header().Get("Content-Type");got!="text/javascript; charset=utf-8" {t.Fatalf("invalid Content-Type %q",got)}
      if got:=w.Header().Get("Cache-Control");got!="no-store" {t.Fatalf("invalid Cache-Control %q",got)}
      if !strings.Contains(w.Body.String(),"window.AnyflipAdminAPI") {t.Fatal("missing admin API adapter")}
      // Fetching a JS asset never constitutes API access.
      api:=httptest.NewRecorder()
      s.handler().ServeHTTP(api,httptest.NewRequest(http.MethodGet,"http://rcat.example/api/admin/users",nil))
      if api.Code==http.StatusOK {t.Fatal("unauthenticated admin data exposed")}
    }
  }
}

func TestAdminStylesheetIsGoogleModeOnly(t *testing.T) {
  for _, google :=range []bool{false,true} {
    s,err:=newWebServer(testWebToken,t.TempDir())
    if err!=nil {t.Fatal(err)}
    if google {
      s.google,err=newGoogleAuth("client","secret","https://rcat.example/auth/google/callback")
      if err!=nil {t.Fatal(err)}
    }
    w:=httptest.NewRecorder()
    s.handler().ServeHTTP(w,httptest.NewRequest(http.MethodGet,"http://rcat.example/admin.css",nil))
    expected:=http.StatusNotFound
    if google {expected=http.StatusOK}
    if w.Code!=expected {t.Fatalf("google=%v /admin.css HTTP %d expected %d",google,w.Code,expected)}
    if google {
      if got:=w.Header().Get("Content-Type");got!="text/css; charset=utf-8" {t.Fatalf("admin CSS content type %q",got)}
      if !strings.Contains(w.Body.String(),".admin-") {t.Fatal("missing Admin component styles")}
    }
  }
}
