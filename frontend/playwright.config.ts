import { defineConfig, devices } from '@playwright/test';

export default defineConfig({
  testDir: './e2e',
  fullyParallel: false,
  retries: 0,
  reporter: 'list',
  globalSetup: './e2e/global.setup.ts',
  use: {
    baseURL: 'http://127.0.0.1:3300',
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
    video: 'retain-on-failure',
  },
  projects: [
    {
      name: 'chromium',
      use: {
        ...devices['Desktop Chrome'],
        browserName: 'chromium',
      },
    },
  ],
  webServer: [
    {
      command:
        'bash -lc "rm -rf /tmp/theia-playwright && mkdir -p /tmp/theia-playwright && THEIA_DB_DSN=\\"${THEIA_E2E_DB_DSN:-${THEIA_DB_DSN:-}}\\" THEIA_DATA_DIR=/tmp/theia-playwright THEIA_LISTEN_ADDR=:38080 THEIA_ENCRYPTION_KEY=0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef go run ./cmd/theia -config config.yaml"',
      cwd: '..',
      url: 'http://127.0.0.1:38080/api/v1/health',
      reuseExistingServer: false,
      timeout: 120000,
    },
    {
      command: 'npm run dev -- --host 127.0.0.1 --port 3300',
      cwd: '.',
      env: {
        VITE_API_URL: 'http://127.0.0.1:38080',
      },
      url: 'http://127.0.0.1:3300',
      reuseExistingServer: false,
      timeout: 120000,
    },
  ],
});
