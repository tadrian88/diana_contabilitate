import { expect, test } from '@playwright/test'

test('rezolvă asocierea cu mai multe contracte și continuă automat', async ({ page }) => {
  await page.goto('/invoices/inv-multiple?tab=contract')
  await expect(page.getByText('Contract recomandat')).toBeVisible()
  await page.getByRole('button', { name: 'Confirmă', exact: true }).click()
  await expect(page.getByText('Exportată în SAGA', { exact: true }).first()).toBeVisible({ timeout: 15_000 })
})

test('permite alegerea contractului alternativ', async ({ page }) => {
  await page.goto('/invoices/inv-multiple?tab=contract')
  await page.getByRole('button', { name: 'Alege alt contract' }).click()
  await page.getByRole('radio').check()
  await page.getByRole('button', { name: 'Confirmă selecția' }).click()
  await expect(page.getByText('CTR-DEMO-202')).toBeVisible()
  await expect(page.getByText('Exportată în SAGA', { exact: true }).first()).toBeVisible()
})

test('solicită contractul și păstrează factura în așteptare', async ({ page }) => {
  await page.goto('/invoices/inv-missing?tab=contract')
  await page.getByRole('button', { name: 'Solicită contract' }).click()
  await expect(page.getByText('Contract solicitat')).toBeVisible()
  await expect(page.getByRole('heading', { name: 'Așteaptă contract' })).toBeVisible()
  await expect(page.getByText(/Task-ul rămâne în așteptare/)).toBeVisible()
})

test('rezolvă toate clasificările incerte înainte de export', async ({ page }) => {
  await page.goto('/invoices/inv-classification?tab=classification')
  await page.getByRole('button', { name: 'Acceptă propunerea' }).first().click()
  await expect(page.getByRole('button', { name: 'Acceptă propunerea' })).toHaveCount(1)
  await page.getByRole('button', { name: 'Corectează' }).click()
  await page.getByLabel('Valoare corectată').fill('Valoare contabilă demonstrativă corectată')
  await page.getByRole('button', { name: 'Salvează corecția' }).click()
  await expect(page.getByText('Exportată în SAGA', { exact: true }).first()).toBeVisible()
})

test('happy path ajunge automat în EXPORTED', async ({ page }) => {
  await page.goto('/invoices/inv-happy')
  await expect(page.getByText('Exportată în SAGA', { exact: true }).first()).toBeVisible({ timeout: 15_000 })
  await expect(page.getByText('Procesare finalizată. Factura a ajuns în starea SAGA simulată.')).toBeVisible()
  await page.getByRole('tab', { name: 'Istoric' }).click()
  await expect(page.getByText('Stare simulată: COMMERCIALLY_VALIDATED.')).toBeVisible()
  await expect(page.getByText('Stare simulată: READY_FOR_SAGA.')).toBeVisible()
  await expect(page.getByText('Stare simulată: EXPORTING.')).toBeVisible()
})

test('păstrează contextul de client vizibil și comută tema', async ({ page }) => {
  await page.goto('/')
  await expect(page.getByTestId('active-scope')).toHaveText('Toți clienții')
  await page.getByLabel('Selectează clientul').click()
  await page.getByRole('option', { name: 'Client Demo Alfa SRL' }).click()
  await expect(page.getByTestId('active-scope')).toHaveText('Client Demo Alfa SRL')

  await page.getByRole('button', { name: 'Activează tema întunecată' }).click()
  await expect(page.locator('html')).toHaveClass(/dark/)
})
