import { expect, test } from './fixtures/authenticated'

test('TEST_ONLY domain workspace retains four dimensions and a mapping blocker', async ({ page }) => {
  await page.goto('/invoices/inv-accounting-v2-test-only?tab=classification')
  await expect(page.getByText('Configurație sintetică TEST_ONLY', { exact: false })).toBeVisible()
  await expect(page.getByText('Tratament TVA', { exact: true }).first()).toBeVisible()
  await expect(page.getByText('Drept de deducere TVA', { exact: true }).first()).toBeVisible()
  await expect(page.getByText('Tratament fiscal al cheltuielii — impozit pe profit', { exact: true }).first()).toBeVisible()
  await expect(page.getByText('Cont contabil', { exact: true }).first()).toBeVisible()
  await expect(page.getByText('Factura necesită verificarea ta')).toBeVisible()
  await expect(page.getByText('Maparea SAGA pentru tratamentul obișnuit nu este aprobată/validată.')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Descarcă XML' })).toHaveCount(0)
})

test('typed classifications remain final and read-only while the SAGA mapping blocker stays open', async ({ page }) => {
  await page.goto('/invoices/inv-accounting-v2-test-only?tab=classification')
  const row = page.locator('article[aria-label="Drept de deducere TVA"]')
  await expect(row.getByText('Finală')).toBeVisible()
  await expect(row.getByRole('button', { name: 'Modifică' })).toHaveCount(0)
  await expect(row).toContainText('TVA deductibilă integral')
  await expect(page.getByText('Maparea SAGA pentru tratamentul obișnuit nu este aprobată/validată.')).toBeVisible()
  await page.reload()
  await expect(page.locator('article[aria-label="Drept de deducere TVA"]')).toContainText('TVA deductibilă integral')
  await expect(page.getByText('Factura necesită verificarea ta')).toBeVisible()
})
