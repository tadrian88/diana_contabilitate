import { defineConfig, devices } from '@playwright/test'

// Requires a migrated isolated DATABASE_URL, REDIS_URL, and the test-only seed.
// No live Gemini calls. No implicit database creation, migration, or reset.
export default defineConfig({
  testDir: './e2e', testMatch: 'contract-ingestion.spec.ts', workers: 1, fullyParallel: false, reporter: 'list', timeout: 90_000,
  use: { baseURL: 'http://127.0.0.1:4180', trace: 'on-first-retry' },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
  webServer: [
    { command: 'cd backend && APP_ENV=test GOCACHE=/private/tmp/diana-go-cache HTTP_ADDRESS=127.0.0.1:8090 PIPELINE_DISPATCH_ENABLED=true CONTRACT_EXTRACTOR_MODE=fake-fixtures go run ./cmd/api', url: 'http://127.0.0.1:8090/readyz', reuseExistingServer: false, timeout: 120_000 },
    { command: 'cd backend && APP_ENV=test GOCACHE=/private/tmp/diana-go-cache WORKER_HTTP_ADDRESS=127.0.0.1:8091 CONTRACT_EXTRACTOR_MODE=fake-fixtures go run ./cmd/worker', url: 'http://127.0.0.1:8091/readyz', reuseExistingServer: false, timeout: 120_000 },
    { command: 'VITE_BACKEND_READS_ENABLED=true VITE_API_PROXY_TARGET=http://127.0.0.1:8090 npm run dev -- --host 127.0.0.1 --port 4180', url: 'http://127.0.0.1:4180', reuseExistingServer: false, timeout: 120_000 },
  ],
})
