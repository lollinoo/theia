import { defineConfig, devices } from '@playwright/test';

// The deployment harness creates its own isolated instance and operator files.
// HTTPS is independently checked against the real CA by the administration CLI.
export default defineConfig({
  testDir: './deployment-e2e',
  fullyParallel: false,
  retries: 0,
  reporter: 'list',
  use: {
    baseURL: process.env.THEIA_MANAGED_URL,
    ignoreHTTPSErrors: true,
    trace: 'retain-on-failure',
  },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
});
