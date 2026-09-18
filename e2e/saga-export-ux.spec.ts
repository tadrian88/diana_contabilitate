import { expect, test } from './fixtures/authenticated'

test('real SAGA artifact stays EXPORTING until explicit human confirmation', async ({ page }) => {
  await page.goto('/invoices/inv-saga-ux-real')
  await expect(page.getByText('Fișier pregătit')).toBeVisible({ timeout: 15_000 })
  await expect(page.getByText('Export în curs').first()).toBeVisible()

  const downloadPromise = page.waitForEvent('download')
  await page.getByRole('button', { name: 'Descarcă XML' }).click()
  const download = await downloadPromise
  expect(download.suggestedFilename()).toMatch(/\.xml$/)
  await expect(page.getByText('Export în curs').first()).toBeVisible()

  await page.getByRole('button', { name: 'Marchează ca importat în SAGA' }).click()
  await expect(page.getByRole('dialog')).toContainText('Diana nu poate verifica automat importul în SAGA.')
  await page.getByRole('button', { name: 'Confirmă importul' }).click()
  await expect(page.getByText('Import confirmat manual').first()).toBeVisible()
  await expect(page.getByText('Exportată în SAGA').first()).toBeVisible()

  await page.reload()
  await expect(page.getByText('Import confirmat manual').first()).toBeVisible()
  await page.getByRole('tab', { name: 'Istoric' }).click()
  await expect(page.getByText('Fișier SAGA generat')).toBeVisible()
  await expect(page.getByText('Fișier SAGA descărcat')).toBeVisible()
  await expect(page.getByText('Import SAGA confirmat manual')).toBeVisible()
  await page.goto('/')
  await expect(page.getByText('Exportate în SAGA').first()).toBeVisible()
})

test('another client path cannot disclose artifact bytes', async ({ request }) => {
  const response = await request.get('/api/v1/clients/client-beta/invoices/inv-saga-ux-real/saga-export/artifact')
  expect([403, 404, 409]).toContain(response.status())
  expect(response.headers()['content-type']).not.toContain('application/xml')
})
