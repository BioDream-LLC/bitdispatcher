// The end-to-end suite: a real bitdispatcher (demo data) and the BitVoice simulator, driven through the
// browser. Run with `make e2e` (it builds both binaries first).
import { defineConfig } from '@playwright/test'

export default defineConfig({
  testDir: './e2e/suite',
  globalSetup: './e2e/suite/setup.ts',
  globalTeardown: './e2e/suite/teardown.ts',
  timeout: 60_000,
  workers: 1,
  retries: 0,
  reporter: [['list']],
  use: { baseURL: 'https://127.0.0.1:9643/', ignoreHTTPSErrors: true, viewport: { width: 1920, height: 1080 }, trace: 'retain-on-failure' },
})
