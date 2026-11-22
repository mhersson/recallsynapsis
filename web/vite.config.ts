import react from '@vitejs/plugin-react'
import { defineConfig } from 'vitest/config'

export default defineConfig({
  plugins: [react()],
  build: {
    // The placeholder beside it is the Go package's and a build never touches it.
    outDir: 'dist/app',
    // The page's policy has no data: source.
    assetsInlineLimit: 0,
  },
  server: {
    // For npm run dev beside a running recallsynapsis serve on its default address.
    proxy: { '/api': 'http://127.0.0.1:7119' },
  },
  test: {
    environment: 'jsdom',
    globals: true,
    // CSS files read as raw text are empty unless processed; the colour check reads them.
    css: { include: /\.css\?raw/ },
    setupFiles: './src/test-setup.ts',
  },
})
