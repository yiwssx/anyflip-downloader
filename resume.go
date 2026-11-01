package main

import (
    "crypto/sha256"
    "encoding/hex"
    "encoding/json"
    "errors"
    "fmt"
    "image"
    "io"
    "net/url"
    "os"
    "path"
    "path/filepath"
    "runtime"
    "strings"
)

const resumeManifestName = ".anyflip-resume.json"

// A manifest binds a private interrupted workspace to the exact book page
// metadata and quality mode, without storing the source URL or book title.
type resumeManifest struct {
    Version int `json:"version"`
    Fingerprint string `json:"fingerprint"`
    Pages int `json:"pages"`
}

func expectedResumeManifest(book *flipbook, bestQuality bool) resumeManifest {
    h := sha256.New()
    fmt.Fprintf(h, "anyflip-resume-v1\x00%s\x00%d\x00%t\n", book.URL.String(), book.pageCount, bestQuality)
    for i, primary := range book.pageURLs {
        fmt.Fprintf(h, "%d\x00%s\x00", i, primary)
        if i < len(book.pageFallbackURLs) {
            for _, alternative := range book.pageFallbackURLs[i] {
                fmt.Fprintf(h, "%s\x00", alternative)
            }
        }
        fmt.Fprintln(h)
    }
    return resumeManifest{Version: 1, Fingerprint: hex.EncodeToString(h.Sum(nil)), Pages: book.pageCount}
}

func writeResumeManifest(workspace string, book *flipbook, bestQuality bool) error {
    data, err := json.Marshal(expectedResumeManifest(book, bestQuality))
    if err != nil { return err }
    file, err := os.OpenFile(filepath.Join(workspace, resumeManifestName), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
    if err != nil { return fmt.Errorf("create resume manifest: %w", err) }
    _, writeErr := file.Write(append(data, '\n'))
    if writeErr == nil { writeErr = file.Sync() }
    closeErr := file.Close()
    if writeErr != nil { return writeErr }
    return closeErr
}

func openResumeWorkspace(requested string) (string, error) {
    if requested == "" { return "", errors.New("resume requires -temp-download-folder") }
    folder, err := filepath.Abs(requested)
    if err != nil { return "", err }
    st, err := os.Lstat(folder)
    if err != nil { return "", fmt.Errorf("inspect resume workspace: %w", err) }
    if !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
        return "", errors.New("resume workspace must be a real directory, not a symlink")
    }
    if runtime.GOOS != "windows" && st.Mode().Perm()&0077 != 0 {
        return "", errors.New("resume workspace must not be accessible to group or other users")
    }
    return folder, nil
}

func verifyResumeManifest(workspace string, book *flipbook, bestQuality bool) error {
    manifestPath := filepath.Join(workspace, resumeManifestName)
    st, err := os.Lstat(manifestPath)
    if err != nil { return fmt.Errorf("missing trusted resume manifest (cannot resume legacy workspaces): %w", err) }
    if !st.Mode().IsRegular() || st.Size() < 2 || st.Size() > 2048 {
        return errors.New("untrusted resume manifest file")
    }
    if runtime.GOOS != "windows" && st.Mode().Perm()&0077 != 0 {
        return errors.New("resume manifest must not be accessible to other users")
    }
    data, err := os.ReadFile(manifestPath)
    if err != nil { return err }
    var got resumeManifest
    if err := json.Unmarshal(data, &got); err != nil { return fmt.Errorf("invalid resume manifest: %w", err) }
    expected := expectedResumeManifest(book, bestQuality)
    if got != expected {
        return errors.New("resume workspace belongs to a different book, updated page map, or quality setting")
    }
    return nil
}

// checkExistingResumePage skips only complete, validated page files in the
// matching private workspace. It never treats a hidden .anyflip-page-* partial
// file as complete and never overwrites an existing named page image.
func checkExistingResumePage(book *flipbook, folder string, index int) (bool, error) {
    if book == nil || book.URL == nil || index < 0 || index >= len(book.pageURLs) {
        return false, errors.New("invalid resume page")
    }
    candidates := []string{book.pageURLs[index]}
    if index < len(book.pageFallbackURLs) {
        candidates = append(candidates, book.pageFallbackURLs[index]...)
    }
    allowed := map[string]bool{}
    for _, raw := range candidates {
        parsed, err := url.Parse(raw)
        if err != nil { return false, err }
        allowed[strings.ToLower(path.Ext(parsed.Path))] = true
    }
    var existing string
    for _, ext := range []string{".jpg", ".jpeg", ".png", ".webp"} {
        name := filepath.Join(folder, fmt.Sprintf("%04d%s", index, ext))
        st, err := os.Lstat(name)
        if errors.Is(err, os.ErrNotExist) { continue }
        if err != nil { return false, err }
        if existing != "" { return false, fmt.Errorf("duplicate stored images for resume page %d", index+1) }
        if !allowed[ext] || !st.Mode().IsRegular() || st.Size() <= 0 || st.Size() > maxPageBytes {
            return false, fmt.Errorf("untrusted cached image for page %d", index+1)
        }
        existing = name
    }
    if existing == "" { return false, nil }
    file, err := os.Open(existing)
    if err != nil { return false, err }
    cfg, format, decodeErr := image.DecodeConfig(io.LimitReader(file, maxPageBytes))
    closeErr := file.Close()
    if decodeErr != nil { return false, fmt.Errorf("cached page %d is corrupt: %w", index+1, decodeErr) }
    if closeErr != nil { return false, closeErr }
    if err := validatePageDimensions(cfg.Width, cfg.Height); err != nil { return false, err }
    if format != "jpeg" && format != "png" && format != "webp" {
        return false, fmt.Errorf("unexpected cached page %d format %q", index+1, format)
    }
    return true, nil
}
