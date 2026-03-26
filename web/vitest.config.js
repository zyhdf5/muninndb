import { defineConfig } from 'vitest/config'

export default defineConfig({
  test: {
    include: ['**/*.{test,spec}.{js,ts}'],
    exclude: ['e2e/**', 'node_modules/**', 'dist/**'],
  },
})
