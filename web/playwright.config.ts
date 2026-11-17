import { defineConfig, devices } from '@playwright/test'

// Runs against a real bin/aim daemon (build it first with `task build`).
// globalSetup picks free ports, so baseURL is set in the spec, not here.
export default defineConfig({
  testDir: './e2e',
  testMatch: '**/*.spec.ts',
  globalSetup: './e2e/global-setup.ts',
  outputDir: './test-results',
  fullyParallel: false,
  workers: 1,
  retries: 0,
  timeout: 30_000,
  expect: { timeout: 10_000 },
  reporter: 'line',
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
})
