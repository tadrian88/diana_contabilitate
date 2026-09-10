import { expect, test } from '@playwright/test'

test('navighează din contracte către o factură asociată', async ({ page }) => {
  await page.goto('/contracts')
  await page.getByLabel('Caută după referință sau furnizor').fill('CTR-DEMO-100')
  await page.getByRole('link', { name: 'CTR-DEMO-100', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'CTR-DEMO-100' })).toBeVisible()
  await expect(page.getByText('SRC-DEMO-CTR-100')).toBeVisible()
  await expect(page.getByText('DEMO-HP-001')).toBeVisible()
  await expect(page.getByText('DEMO-SH-011')).toBeVisible()
  await page.getByRole('link', { name: 'DEMO-SH-011' }).click()
  await expect(page.getByRole('heading', { name: 'DEMO-SH-011' })).toBeVisible()
})

test('inspectează candidații înainte de rezolvarea task-ului de contract', async ({ page }) => {
  await page.goto('/tasks')
  const taskRow = page.locator('tbody tr').filter({ hasText: 'DEMO-MC-002' })
  await taskRow.getByRole('link', { name: 'Rezolvă' }).click()

  await page.getByRole('link', { name: 'Inspectează contractul' }).click()
  await expect(page.getByRole('heading', { name: 'CTR-DEMO-201' })).toBeVisible()
  await expect(page.getByText('Vizualizarea nu rezolvă task-ul.', { exact: false })).toBeVisible()
  await page.getByRole('link', { name: 'Înapoi la revizuirea facturii' }).click()
  await expect(page.getByText('Contract recomandat')).toBeVisible()

  await page.getByRole('button', { name: 'Alege alt contract' }).click()
  await page.getByRole('link', { name: 'Inspectează detaliile' }).click()
  await expect(page.getByRole('heading', { name: 'CTR-DEMO-202' })).toBeVisible()
  await page.getByRole('link', { name: 'Înapoi la revizuirea facturii' }).click()
  await page.getByRole('button', { name: 'Alege alt contract' }).click()
  await page.getByRole('radio').check()
  await page.getByRole('button', { name: 'Confirmă selecția' }).click()
  await expect(page.getByText('CTR-DEMO-202')).toBeVisible()
  await page.getByRole('link', { name: 'Înapoi la Task Inbox' }).click()
  await expect(page.getByText('DEMO-MC-002')).toHaveCount(0)
})

test('filtrează contractele prin contextul persistent de client', async ({ page }) => {
  await page.goto('/contracts')
  await expect(page.getByTestId('active-scope')).toHaveText('Toți clienții')
  await expect(page.getByText('CTR-DEMO-401')).toBeVisible()
  await page.getByLabel('Selectează clientul').click()
  await page.getByRole('option', { name: 'Client Demo Alfa SRL' }).click()
  await expect(page.getByTestId('active-scope')).toHaveText('Client Demo Alfa SRL')
  await expect(page.getByText('CTR-DEMO-100')).toBeVisible()
  await expect(page.getByText('CTR-DEMO-401')).toHaveCount(0)
})
