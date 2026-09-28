import react from '@vitejs/plugin-react'
import { defineConfig } from 'vitest/config'

declare const process: { env: Record<string, string | undefined> }

// `npm run dev` proxies the API to a running `qtldr serve` (set QTLDR_URL).
export default defineConfig({
  plugins: [react()],
  build: { outDir: 'dist', emptyOutDir: true, chunkSizeWarningLimit: 1500 },
  server: { proxy: { '/api': process.env.QTLDR_URL ?? 'http://127.0.0.1:7777' } },
  test: { environment: 'node', include: ['src/**/*.test.ts'] },
})
