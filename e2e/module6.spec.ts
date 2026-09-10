import { expect, test } from '@playwright/test'

test('filtrează regulile și inspectează versiunea curentă și istoricul', async ({ page }) => {
  await page.goto('/rules')
  await page.getByLabel('Categorie').selectOption('ACCOUNT')
  await expect(page.getByRole('table')).toContainText('Încadrare cont pentru servicii demonstrative')
  await page.getByRole('link', { name: 'Încadrare cont pentru servicii demonstrative' }).click()
  await expect(page.getByRole('heading', { name: 'Încadrare cont pentru servicii demonstrative' })).toBeVisible()
  await expect(page.getByText('Versiunea 2 · Curentă').first()).toBeVisible()
  await expect(page.getByRole('heading', { name: 'Istoric versiuni' })).toBeVisible()
  await page.getByRole('link', { name: 'Inspectează versiunea' }).click()
  await expect(page.getByText('Versiunea 1 · Istorică').first()).toBeVisible()
  await expect(page.getByText('Versiune istorică read-only.')).toBeVisible()
})

test('creează o versiune nouă fără a rescrie istoricul', async ({ page }) => {
  await page.goto('/rules/rule-account-global')
  await page.getByRole('button', { name: 'Creează versiune nouă' }).click()
  await page.getByLabel('Criteriu / pattern').fill('Criteriu demonstrativ revizuit pentru servicii în RON.')
  await page.getByLabel('Rezultat clasificare').fill('Cont demonstrativ 6YY')
  await page.getByRole('button', { name: 'Salvează versiunea nouă' }).click()
  await expect(page.getByText('Versiunea 3 · Curentă').first()).toBeVisible()
  await expect(page.getByText('Versiunea 2 · Istorică').first()).toBeVisible()
  await expect(page.getByText('Criteriu demonstrativ revizuit pentru servicii în RON.').first()).toBeVisible()
  await expect(page.getByText('Regulă globală').first()).toBeVisible()
})

test('creează un override Alfa și îl păstrează limitat la client', async ({ page }) => {
  await page.goto('/rules/rule-vat-global')
  await page.getByRole('button', { name: 'Creează override pentru client' }).click()
  await page.getByRole('dialog', { name: 'Creează override pentru client' }).locator('select[name="clientId"]').selectOption('client-alfa')
  await page.getByLabel('Criteriu / pattern').fill('Criteriu TVA demonstrativ exclusiv pentru Alfa.')
  await page.getByLabel('Rezultat clasificare').fill('Rezultat TVA demonstrativ Alfa')
  await page.getByRole('button', { name: 'Salvează override-ul' }).click()

  await page.getByLabel('Selectează clientul').click()
  await page.getByRole('option', { name: 'Client Demo Alfa SRL' }).click()
  await expect(page.getByTestId('active-scope')).toHaveText('Client Demo Alfa SRL')
  await expect(page.getByText('REG-DEMO-TVA-01-OVR-ALFA-1')).toBeVisible()
  await expect(page.getByText('REG-DEMO-TVA-01', { exact: true })).toBeVisible()

  await page.getByLabel('Selectează clientul').click()
  await page.getByRole('option', { name: 'Client Demo Beta SRL' }).click()
  await expect(page.getByText('REG-DEMO-TVA-01-OVR-ALFA-1')).toHaveCount(0)
  await expect(page.getByText('REG-DEMO-TVA-01', { exact: true })).toBeVisible()
})

test('navighează din clasificare la versiunea exactă a regulii și înapoi', async ({ page }) => {
  await page.goto('/invoices/inv-resolved?tab=classification')
  const origin = page.getByRole('link', { name: 'Origine regulă: Regulă globală · REG-DEMO-CONT-01 · v2' }).first()
  await expect(origin).toBeVisible()
  await origin.click()
  await expect(page).toHaveURL(/\/rules\/rule-account-global\?version=2/)
  await expect(page.getByText('Versiunea 2 · Curentă').first()).toBeVisible()
  await page.getByRole('link', { name: 'Înapoi la clasificarea facturii' }).click()
  await expect(page).toHaveURL(/\/invoices\/inv-resolved\?tab=classification/)
  await expect(page.getByRole('heading', { name: 'Clasificări pe linii' })).toBeVisible()
})
