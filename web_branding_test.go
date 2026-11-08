package main

import (
 "bytes"
 "encoding/xml"
 "io"
 "net/http"
 "net/http/httptest"
 "strings"
 "testing"
)

func TestBrandingAssetsAreSelfContainedAndServedInBothModes(t *testing.T) {
 for _,google :=range []bool{false,true} {
  s,err:=newWebServer(testWebToken,t.TempDir())
  if err!=nil{t.Fatal(err)}
  if google {
   s.google,err=newGoogleAuth("test-client","test-secret","https://rcat.example/auth/google/callback")
   if err!=nil{t.Fatal(err)}
  }
  handler:=s.handler()
  for _,asset:=range []struct{path,embedded string}{
   {"/favicon.svg","data:image/png;base64,"},
   {"/logo.svg","data:image/webp;base64,"},
  } {
   r:=httptest.NewRequest(http.MethodGet,"http://example.test"+asset.path,nil)
   w:=httptest.NewRecorder()
   handler.ServeHTTP(w,r)
   if w.Code!=http.StatusOK{t.Fatalf("google=%v path=%s returned HTTP %d",google,asset.path,w.Code)}
   if ct:=w.Header().Get("Content-Type");ct!="image/svg+xml" {t.Fatalf("incorrect content type: %s",ct)}
   body:=w.Body.Bytes()
   if !bytes.Contains(body,[]byte(asset.embedded)){t.Fatalf("missing self-contained icon artwork: %s",asset.path)}
   if bytes.Contains(body,[]byte("<script"))||bytes.Contains(body,[]byte("http://"))&&bytes.Contains(body,[]byte("<image href=\"http://")) {
    t.Fatalf("unexpected executable or externally fetched brand image: %s",asset.path)
   }
   decoder:=xml.NewDecoder(bytes.NewReader(body))
   for {
    _,err:=decoder.Token()
    if err==io.EOF{break}
    if err!=nil{t.Fatalf("malformed brand SVG %s: %v",asset.path,err)}
   }
  }
  // In Google mode the login page is served only on its canonical hostname;
  // static brand assets remain accessible on either ingress.
  pageHost:="example.test"
  if google {pageHost="rcat.example"}
  w:=httptest.NewRecorder()
  handler.ServeHTTP(w,httptest.NewRequest(http.MethodGet,"http://"+pageHost+"/",nil))
  if w.Code!=http.StatusOK{t.Fatalf("unexpected page status: %d",w.Code)}
  body:=w.Body.String()
  for _,needle:=range []string{`rel="icon" type="image/svg+xml" href="/favicon.svg"`,`class="brand-logo" src="/logo.svg"`,`src="/workspace.js"`,`RCAT AnyFlip Downloader`} {
   if !strings.Contains(body,needle){t.Fatalf("google=%v missing branding %q",google,needle)}
  }
 }
}
