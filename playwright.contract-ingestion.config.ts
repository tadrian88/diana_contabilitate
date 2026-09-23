import { defineConfig, devices } from '@playwright/test'

// Requires a migrated isolated DATABASE_URL, REDIS_URL, and the test-only seed.
// No live Gemini calls. No implicit database creation, migration, or reset.
export default defineConfig({
  testDir: './e2e', testMatch: 'contract-ingestion.spec.ts', workers: 1, fullyParallel: false, reporter: 'list', timeout: 90_000,
  use: { baseURL: 'http://127.0.0.1:4180', trace: 'on-first-retry' },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
  webServer: [
    { command: './scripts/start-contract-ingestion-e2e.sh', url: 'http://127.0.0.1:8190/readyz', reuseExistingServer: false, timeout: 120_000 },
    { command: 'VITE_BACKEND_READS_ENABLED=true VITE_API_PROXY_TARGET=http://127.0.0.1:8190 npm run dev -- --host 127.0.0.1 --port 4180', url: 'http://127.0.0.1:4180', reuseExistingServer: false, timeout: 120_000 },
  ],
})
