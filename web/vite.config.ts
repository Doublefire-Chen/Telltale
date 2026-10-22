import react from '@vitejs/plugin-react'
import { defineConfig } from 'vitest/config'

// The Dashboard embeds dist/app; dist/.gitkeep stays so the Go embed pattern always resolves.
export default defineConfig({
  plugins: [react()],
  build: {
    outDir: 'dist/app',
    emptyOutDir: true,
    // Emit every flag as its own file so browsers fetch only the flags shown,
    // instead of inlining small ones into the JS bundle.
    assetsInlineLimit: (file) => (file.includes('flag-icons') ? false : undefined),
  },
  server: {
    proxy: {
      '/api': { target: 'http://127.0.0.1:8080', ws: true },
    },
  },
  test: {
    environment: 'jsdom',
    setupFiles: ['src/test-setup.ts'],
  },
})
