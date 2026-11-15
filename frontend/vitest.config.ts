// Vitest gets its own config so the app build stays on Vite's own types: mixing
// vitest/config and vite/config in one file produces duplicate Vite typings.
import { defineConfig } from 'vitest/config'

export default defineConfig({
  esbuild: { jsx: 'automatic' },
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: ['./src/test/setup.ts'],
    css: false,
  },
})
