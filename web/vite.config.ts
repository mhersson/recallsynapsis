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
    // For npm run dev beside a running recallsynapsis serve on its default address. The harness answers the API itself,
    // but for a file's contents, which an image or a link reads: those are the app's icon.
    proxy: {
      '/api': {
        target: 'http://127.0.0.1:7119',
        bypass: (req) => (req.headers.referer?.includes('/harness.html') && /^\/api\/notes\/\d+\/files\/\d+$/.test(req.url ?? '') ? '/icon-512.png' : undefined),
      },
    },
  },
  test: {
    environment: 'jsdom',
    globals: true,
    // CSS files read as raw text are empty unless processed; the colour check reads them.
    css: { include: /\.css\?raw/ },
    setupFiles: './src/test-setup.ts',
  },
})
