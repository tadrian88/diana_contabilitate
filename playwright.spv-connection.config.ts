import { defineConfig, devices } from '@playwright/test'

export default defineConfig({
  testDir: './e2e', testMatch: 'spv-connection.spec.ts', fullyParallel: false, workers: 1, reporter: 'list',
  use: { baseURL: 'http://127.0.0.1:4180', trace: 'on-first-retry' },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
  webServer: [
    { command: './scripts/start-spv-connection-e2e.sh', url: 'http://127.0.0.1:8080/readyz', reuseExistingServer: false, timeout: 120_000 },
    { command: 'VITE_BACKEND_READS_ENABLED=true VITE_API_PROXY_TARGET=http://127.0.0.1:8080 npm run dev -- --host 127.0.0.1 --port 4180', url: 'http://127.0.0.1:4180', reuseExistingServer: false, timeout: 120_000 },
  ],
})
