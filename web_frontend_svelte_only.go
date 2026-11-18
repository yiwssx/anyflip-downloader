//go:build svelte_only

package main

// Compile-time guarantee: production Svelte image cannot fall back to legacy.
const requireSvelteUI = true
