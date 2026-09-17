import { defineConfig, devices } from '@playwright/test'

export default defineConfig({
  testDir: './e2e', testMatch: 'authentication-v1.spec.ts', fullyParallel: false, workers: 1, reporter: 'list',
  use: { baseURL: 'http://127.0.0.1:4181', trace: 'on-first-retry' },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
  webServer: [
    { command: './scripts/start-authentication-e2e.sh', url: 'http://127.0.0.1:8180/readyz', reuseExistingServer: false, timeout: 120000 },
    { command: 'VITE_BACKEND_READS_ENABLED=true VITE_API_PROXY_TARGET=http://127.0.0.1:8180 npm run dev -- --host 127.0.0.1 --port 4181', url: 'http://127.0.0.1:4181', reuseExistingServer: false, timeout: 120000 },
  ],
})
