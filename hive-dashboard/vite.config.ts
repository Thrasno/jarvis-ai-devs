import { defineConfig } from 'vitest/config'

export default defineConfig({
  base: '/dashboard/',
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: './src/test/setup.ts'
  }
})
