import { expect, test } from '@playwright/test'

test('găsește și inspectează o factură', async ({ page }) => {
  await page.goto('/invoices')
  await page.getByLabel('Caută după furnizor sau număr factură').fill('DEMO-RS-007')
  await page.getByRole('link', { name: 'DEMO-RS-007' }).click()
  await expect(page.getByRole('heading', { name: 'Exportată în SAGA' })).toBeVisible()
  await page.getByRole('tab', { name: 'Linii factură' }).click()
  await expect(page.getByText('Serviciu demonstrativ finalizat')).toBeVisible()
  await page.getByRole('tab', { name: 'Istoric' }).click()
  await expect(page.getByText('Export finalizat')).toBeVisible()
})

test('rezolvă excepția de contract și actualizează lista', async ({ page }) => {
  await page.goto('/invoices?attention=REQUIRED')
  const row = page.locator('tbody tr').filter({ hasText: 'DEMO-MC-002' })
  await expect(row.getByText('Acțiune necesară')).toBeVisible()
  await row.getByRole('link', { name: 'DEMO-MC-002' }).click()
  await page.getByRole('tab', { name: 'Contract' }).click()
  await page.getByRole('button', { name: 'Confirmă', exact: true }).click()
  await page.getByRole('link', { name: 'Înapoi la lista facturilor' }).click()
  await expect(page.getByText('DEMO-MC-002')).toHaveCount(0)
  await page.getByLabel('Atenție').selectOption('ALL')
  const updatedRow = page.locator('tbody tr').filter({ hasText: 'DEMO-MC-002' })
  await expect(updatedRow).toContainText('0')
  await expect(updatedRow).not.toContainText('Acțiune necesară')
})

test('rezolvă clasificările și ajunge automat în SAGA', async ({ page }) => {
  await page.goto('/invoices?q=DEMO-CL-004')
  await page.getByRole('link', { name: 'DEMO-CL-004' }).click()
  await page.getByRole('tab', { name: 'Clasificare' }).click()
  await page.getByRole('button', { name: 'Acceptă propunerea' }).first().click()
  await expect(page.getByRole('button', { name: 'Acceptă propunerea' })).toHaveCount(1)
  await page.getByRole('button', { name: 'Acceptă propunerea' }).click()
  await expect(page.getByText('Exportată în SAGA', { exact: true }).first()).toBeVisible()
})

test('duplicatul rămâne terminal și nu avansează spre SAGA', async ({ page }) => {
  await page.goto('/invoices?pipeline=DUPLICATE')
  await page.getByRole('link', { name: 'DEMO-DP-009' }).click()
  await expect(page.getByRole('heading', { name: 'Duplicat' })).toBeVisible()
  await expect(page.getByText(/identificată drept duplicat și nu continuă către SAGA/)).toBeVisible()
  await expect(page.getByText('Nu a ajuns la SAGA').first()).toBeVisible()
  await page.waitForTimeout(500)
  await expect(page.getByRole('heading', { name: 'Duplicat' })).toBeVisible()
})
