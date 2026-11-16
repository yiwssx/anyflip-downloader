import { fileURLToPath, URL } from "node:url";
import { defineConfig } from "vite";
import { svelte } from "@sveltejs/vite-plugin-svelte";
import tailwindcss from "@tailwindcss/vite";

// Default preview paths remain unchanged. An explicit build-only flag emits
// same-origin /svelte/assets URLs for opt-in, Go-embedded candidate images.
const embedBuild = process.env.SUI_EMBED_BUILD === "1";
export default defineConfig({
  base: embedBuild ? "/svelte/" : "/",
  plugins: [svelte(), tailwindcss()],
  resolve: {
    alias: {
      $lib: fileURLToPath(new URL("./src/lib", import.meta.url)),
    },
  },
  build: {
    rollupOptions: {
      input: {
        index: fileURLToPath(new URL("./index.html", import.meta.url)),
        google: fileURLToPath(new URL("./google.html", import.meta.url)),
        token: fileURLToPath(new URL("./token.html", import.meta.url)),
      },
    },
    outDir: "dist",
    emptyOutDir: true,
  },
});
