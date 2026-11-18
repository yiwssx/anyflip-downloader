//go:build svelte_only

package main

import "testing"

// Compile the same tags as Dockerfile.svelte and prove that the production
// binary has no copy of the retired HTML, JS, or CSS in its embedded FS.
func TestSvelteOnlyProductionDoesNotEmbedLegacyAssets(t *testing.T) {
    for _, path := range []string{
        "web/index.html", "web/google.html", "web/google.js",
        "web/tokens.css", "web/style.css", "web/readability.css",
        "web/app.js", "web/workspace.js",
        "web/admin.css", "web/admin-api.js", "web/admin.js",
    } {
        if _, err := webUI.ReadFile(path); err == nil {
            t.Errorf("retired UI asset unexpectedly embedded: %s", path)
        }
    }

    // Public institutional assets must remain in the immutable binary.
    for _, path := range []string{"web/favicon.svg", "web/logo.svg"} {
        if _, err := webUI.ReadFile(path); err != nil {
            t.Errorf("institutional asset missing from production build %s: %v", path, err)
        }
    }
}
