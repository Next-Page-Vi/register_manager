import { writeFileSync } from 'node:fs'
import path from 'node:path'
import tailwindcss from '@tailwindcss/vite'
import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'

// During development the Go server runs on :8080; API calls and the
// customer access-link entry point are proxied to it.
const backend = process.env.RM_BACKEND ?? 'http://localhost:8080'

export default defineConfig({
  plugins: [
    react(),
    tailwindcss(),
    {
      // Keep web/dist in git (as an empty placeholder) so `go build` works
      // on a fresh clone; the Go server then reports that the frontend is
      // not built yet.
      name: 'keep-dist-placeholder',
      closeBundle() {
        writeFileSync(path.resolve(import.meta.dirname, 'dist/.gitkeep'), '')
      },
    },
  ],
  resolve: {
    alias: { '@': path.resolve(import.meta.dirname, './src') },
  },
  server: {
    proxy: {
      '/api': backend,
      '^/i/[^/]+/access': backend,
      '/healthz': backend,
    },
  },
  build: {
    license: { fileName: 'THIRD_PARTY_LICENSES.md' },
    // Vite 8's default modern target; matches the documented browser
    // baseline (Chrome/Edge 111, Firefox 128, Safari 16.4).
    outDir: 'dist',
    emptyOutDir: true,
  },
})
