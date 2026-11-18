//go:build !svelte_only

package main

// Plain Go/CLI test builds remain compatible with existing legacy fixtures.
// Production Svelte images MUST be compiled with -tags=svelte_only.
const requireSvelteUI = false
