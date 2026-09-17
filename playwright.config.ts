import { defineConfig, devices } from '@playwright/test'

export default defineConfig({
  testDir: './e2e',
  testIgnore: [
    'contract-ingestion.spec.ts',
    'accounting-domain-v2.spec.ts',
    'backend-module1.spec.ts',
    'backend-module3.spec.ts',
    'backend-module4.spec.ts',
    'backend-module5.spec.ts',
    'backend-module6.spec.ts',
    'backend-module7.spec.ts',
    'spv-connection.spec.ts',
    'client-onboarding.spec.ts',
	'saga-export-ux.spec.ts',
  ],
  fullyParallel: false,
  reporter: 'list',
  use: {
    baseURL: 'http://127.0.0.1:4173',
    trace: 'on-first-retry',
  },
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
  ],
  webServer: {
    command: 'npm run dev -- --host 127.0.0.1 --port 4173',
    url: 'http://127.0.0.1:4173',
    reuseExistingServer: true,
  },
})
