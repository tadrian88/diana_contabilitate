import { expect, test } from '@playwright/test'

test('Invoice Detail reads the persisted invoice and lines through Go and PostgreSQL', async ({ page }) => {
  const responsePromise = page.waitForResponse((response) => response.url().includes('/api/v1/invoices/inv-resolved'))
  await page.goto('/invoices/inv-resolved')
  const response = await responsePromise
  expect(response.status()).toBe(200)
  const payload = await response.json()
  expect(payload).toMatchObject({ id: 'inv-resolved', clientId: 'client-beta', pipelineStatus: 'EXPORTED' })
  expect(typeof payload.total.amount).toBe('number')
  expect(payload.lines).toHaveLength(1)
  expect(payload.lines[0]).toMatchObject({ description: 'Servicii conform contract', quantity: 1 })

  await expect(page.getByRole('heading', { name: 'DEMO-RS-007' })).toBeVisible()
  await expect(page.getByText('Furnizor Demo Orizont SRL').first()).toBeVisible()
  await expect(page.getByText('Exportată în SAGA', { exact: true }).first()).toBeVisible()
  await page.getByRole('tab', { name: 'Linii factură' }).click()
  await expect(page.getByText('Servicii conform contract')).toBeVisible()
})
