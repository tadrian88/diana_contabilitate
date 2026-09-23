import { defineConfig, devices } from '@playwright/test'

export default defineConfig({
  testDir: './e2e',
  testMatch: 'backend-module4.spec.ts',
  fullyParallel: false,
  workers: 1,
  reporter: 'list',
  use: {
    baseURL: 'http://127.0.0.1:4176',
    trace: 'on-first-retry',
  },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
  webServer: [
    {
      command: 'E2E_FRONTEND_BASE_URL=http://127.0.0.1:4176 E2E_BACKEND_HTTP_ADDRESS=127.0.0.1:8184 ./scripts/start-backend-e2e.sh',
      url: 'http://127.0.0.1:8184/readyz',
      reuseExistingServer: false,
      timeout: 120_000,
    },
    {
      command: 'VITE_BACKEND_READS_ENABLED=true VITE_API_PROXY_TARGET=http://127.0.0.1:8184 npm run dev -- --host 127.0.0.1 --port 4176',
      url: 'http://127.0.0.1:4176',
      reuseExistingServer: false,
      timeout: 120_000,
    },
  ],
})
