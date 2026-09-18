import type { Page } from '@playwright/test'
import { expect, test } from './fixtures/authenticated'

async function expectEventuallyExported(page: Page) {
  await expect(page.getByText('Exportată în SAGA', { exact: true }).first()).toBeVisible({ timeout: 15_000 })
}

async function confirmRecommendedContract(page: Page) {
  const response = page.waitForResponse((item) => item.request().method() === 'POST' && item.url().includes('/contract-confirmations'))
  await page.getByRole('button', { name: 'Confirmă', exact: true }).click()
  expect((await response).status()).toBe(200)
}

async function acceptAllClassificationProposals(page: Page) {
  const buttons = page.getByRole('button', { name: 'Acceptă propunerea' })
  for (;;) {
    const count = await buttons.count()
    if (count === 0) return
    const response = page.waitForResponse((item) => item.request().method() === 'POST' && item.url().includes('/classification-decisions'))
    await buttons.first().click()
    expect((await response).status()).toBe(200)
    await expect(buttons).toHaveCount(count - 1)
  }
}

test('A. asynchronous worker happy path reaches EXPORTED', async ({ page }) => {
  await page.goto('/invoices/inv-module7-happy')
  await expectEventuallyExported(page)
  await page.reload()
  await expectEventuallyExported(page)
})

test('B. multiple contract match stops for a human decision', async ({ page }) => {
  await page.goto('/invoices/inv-module7-match-block?tab=contract')
  await expect(page.getByText('Așteaptă confirmare').first()).toBeVisible({ timeout: 15_000 })
  await expect(page.getByText('Contract recomandat')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Confirmă', exact: true })).toBeVisible()
})

test('C. human contract confirmation commits synchronously and worker resumes', async ({ page }) => {
  await page.goto('/invoices/inv-module7-match-block?tab=contract')
  await confirmRecommendedContract(page)
  await expectEventuallyExported(page)
  await page.reload()
  await expectEventuallyExported(page)
})

test('D. uncertain classification stops the worker at AWAITING_REVIEW', async ({ page }) => {
  await page.goto('/invoices/inv-module7-classification-block?tab=classification')
  await expect(page.getByText('Așteaptă revizuirea').first()).toBeVisible({ timeout: 15_000 })
  await expect(page.getByRole('button', { name: 'Acceptă propunerea' })).not.toHaveCount(0)
})

test('E. final classification decision emits continuation and worker exports', async ({ page }) => {
  await page.goto('/invoices/inv-module7-classification-block?tab=classification')
  await acceptAllClassificationProposals(page)
  await expectEventuallyExported(page)
  await page.reload()
  await expect(page.getByRole('button', { name: 'Acceptă propunerea' })).toHaveCount(0)
})

test('F. missing-contract request remains WAITING and cannot be bypassed', async ({ page }) => {
  await page.goto('/invoices/inv-module7-missing-contract?tab=contract')
  await expect(page.getByText('Așteaptă contract').first()).toBeVisible({ timeout: 15_000 })
  const response = page.waitForResponse((item) => item.request().method() === 'POST' && item.url().includes('/contract-requests'))
  await page.getByRole('button', { name: 'Solicită contract' }).click()
  expect((await response).status()).toBe(200)
  await page.reload()
  await expect(page.getByText('Contract solicitat', { exact: true })).toBeVisible()
  await expect(page.getByText('În așteptare', { exact: true }).first()).toBeVisible()
})

test('G. duplicate remains terminal while worker is running', async ({ page }) => {
  await page.goto('/invoices/inv-module6-duplicate')
  await expect(page.getByText('Duplicat', { exact: true }).first()).toBeVisible()
  await page.reload()
  await expect(page.getByText('Stare terminală. Factura nu continuă către SAGA.')).toBeVisible()
})

test('H. worker exposes distinct liveness, readiness and operational metrics', async ({ request }) => {
  await expect.poll(async () => (await request.get('http://127.0.0.1:8081/healthz')).status()).toBe(200)
  await expect.poll(async () => (await request.get('http://127.0.0.1:8081/readyz')).status()).toBe(200)
  const metrics = await request.get('http://127.0.0.1:8081/metrics')
  expect(metrics.status()).toBe(200)
  expect(await metrics.text()).toContain('diana_worker_jobs_processed_total')
})

test('I. frontend reload does not interrupt background progression', async ({ page }) => {
  await page.goto('/invoices/inv-module7-reload?tab=contract')
  await expect(page.getByText('Așteaptă confirmare').first()).toBeVisible({ timeout: 15_000 })
  await confirmRecommendedContract(page)
  await page.reload()
  await expectEventuallyExported(page)
})

test('J. fake SAGA operational failure creates no ValidationTask', async ({ page, request }) => {
  await page.goto('/invoices/inv-module7-saga-failed')
  await expect(page.getByText('Export eșuat', { exact: true }).first()).toBeVisible({ timeout: 15_000 })
  const response = await request.get('/api/v1/invoices/inv-module7-saga-failed')
  expect(response.status()).toBe(200)
  const invoice = await response.json() as { pipelineStatus: string; sagaStatus: string; task?: unknown }
  expect(invoice).toMatchObject({ pipelineStatus: 'EXPORTING', sagaStatus: 'FAILED' })
  expect(invoice.task).toBeUndefined()
})
