//go:build svelte_only

package main

import "embed"

// Production Svelte-only binary physically excludes legacy HTML/JS/CSS.
// Keep institution-owned branding and the Vite-generated embedded Svelte UI.
//go:embed web/favicon.svg web/logo.svg web/svelte
var webUI embed.FS
