package main

import (
  "bytes"
  "fmt"
  "net/http"
  "os"
  "regexp"
  "strings"
)

// These files are embedded in the Go binary, never served from a writable
// host directory. The default legacy UI requires no Node build or bundle.
var (
  svelteHTMLAsset = regexp.MustCompile(`/(svelte/assets/[A-Za-z0-9._-]+\.(js|css))`)
  svelteAssetName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}\.(js|css|svg|png|webp|woff|woff2)$`)
)

func selectWebFrontend() (string, error) {
  mode := strings.TrimSpace(os.Getenv("ANYFLIP_WEB_UI"))
  switch mode {
  case "", "legacy":
    return "legacy", nil
  case "svelte":
    if err := validateSvelteBundle(); err != nil {
      return "", fmt.Errorf("Svelte frontend not embedded or invalid: %w", err)
    }
    return "svelte", nil
  default:
    return "", fmt.Errorf("unsupported ANYFLIP_WEB_UI mode %q", mode)
  }
}

func validateSvelteBundle() error {
  for _, entry := range []string{"web/svelte/token.html", "web/svelte/google.html"} {
    html, err := webUI.ReadFile(entry)
    if err != nil { return fmt.Errorf("%s missing: %w", entry, err) }
    // Built Vite entry files must reference external, same-origin hashed
    // CSS/JS. They may not require unsafe-inline or runtime CDN access.
    if bytes.Contains(html, []byte("<script>")) || bytes.Contains(html, []byte("<style>")) {
      return fmt.Errorf("%s contains inline script/style", entry)
    }
    matches := svelteHTMLAsset.FindAllSubmatch(html, -1)
    seenJS,seenCSS := false,false
    if len(matches)==0 { return fmt.Errorf("%s has no /svelte/assets references", entry) }
    for _, match := range matches {
      assetPath := string(match[1])
      body, err := webUI.ReadFile("web/"+assetPath)
      if err != nil || len(body)==0 {
        return fmt.Errorf("referenced Svelte asset %q unavailable", assetPath)
      }
      if string(match[2])=="js" {seenJS=true} else {seenCSS=true}
    }
    if !seenJS || !seenCSS {return fmt.Errorf("%s must use external JS and CSS",entry)}
  }
  return nil
}

func serveSvelteAsset(w http.ResponseWriter, r *http.Request) {
  name := strings.TrimPrefix(r.URL.Path, "/svelte/assets/")
  if !svelteAssetName.MatchString(name) {
    http.NotFound(w,r)
    return
  }
  var contentType string
  switch {
  case strings.HasSuffix(name,".js"):
    contentType="text/javascript; charset=utf-8"
  case strings.HasSuffix(name,".css"):
    contentType="text/css; charset=utf-8"
  case strings.HasSuffix(name,".svg"):
    contentType="image/svg+xml"
  case strings.HasSuffix(name,".png"):
    contentType="image/png"
  case strings.HasSuffix(name,".webp"):
    contentType="image/webp"
  case strings.HasSuffix(name,".woff"):
    contentType="font/woff"
  case strings.HasSuffix(name,".woff2"):
    contentType="font/woff2"
  default:
    http.NotFound(w,r)
    return
  }
  data,err:=webUI.ReadFile("web/svelte/assets/"+name)
  if err!=nil {
    http.NotFound(w,r)
    return
  }
  w.Header().Set("Content-Type",contentType)
  _,_ = w.Write(data)
}
