//go:build !svelte_only

package main

import "embed"

// Compatibility assets for plain Go/CLI tests and pre-migration builds.
//go:embed web/index.html web/tokens.css web/style.css web/readability.css web/admin.css web/workspace.js web/app.js web/google.html web/google.js web/admin-api.js web/admin.js web/favicon.svg web/logo.svg web/svelte
var webUI embed.FS
