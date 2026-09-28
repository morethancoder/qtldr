import { defineConfig } from '@playwright/test'

declare const process: { env: Record<string, string | undefined> }

// Smoke test against the real server on the fixture (docs/decisions.md).
// Artifacts go to <repo>/.playwright/.
const port = Number(process.env.QTLDR_E2E_PORT ?? 7790)

export default defineConfig({
  testDir: './e2e',
  outputDir: '../.playwright/test-results',
  reporter: [['list']],
  use: { baseURL: `http://127.0.0.1:${port}`, viewport: { width: 1440, height: 900 }, trace: 'retain-on-failure' },
  webServer: {
    command: `go run ../cmd/qtldr -C ../testdata/ledger serve --port ${port}`,
    url: `http://127.0.0.1:${port}/api/config`,
    reuseExistingServer: false,
    timeout: 120_000,
  },
})
