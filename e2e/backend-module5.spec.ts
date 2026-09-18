import { expect, test } from './fixtures/authenticated'

test('automatically classified invoice exposes all three dimensions', async ({ page }) => {
  await page.goto('/invoices/inv-classification-auto-api?tab=classification')
  await expect(page.getByText('Cont contabil')).toBeVisible()
  await expect(page.getByText('Confirmare istorică a cotei TVA')).toBeVisible()
  await expect(page.getByText('Instrucțiune SAGA istorică')).toBeVisible()
  await expect(page.getByText('Acceptat automat')).toHaveCount(3)
  await expect(page.getByText('Pregătită pentru SAGA').first()).toBeVisible()
})

test('uncertain classifications enter AWAITING_REVIEW', async ({ page }) => {
  await page.goto('/invoices/inv-classification-review-api?tab=classification')
  await expect(page.getByText('Decizie necesară')).toBeVisible()
  await expect(page.getByText('Așteaptă revizuirea').first()).toBeVisible()
})

test('one task groups multiple uncertain line dimensions', async ({ page }) => {
  await page.goto('/invoices/inv-classification-review-api?tab=classification')
  await expect(page.getByText('Un singur task grupează toate clasificările incerte ale facturii.')).toBeVisible()
  await expect(page.getByText('6', { exact: true }).first()).toBeVisible()
})

test('accountant can accept one proposed classification', async ({ page }) => {
  await page.goto('/invoices/inv-classification-review-api?tab=classification')
  const response = page.waitForResponse((value) => value.url().includes('/classification-decisions'))
  await page.getByRole('button', { name: 'Acceptă propunerea' }).first().click()
  expect((await response).status()).toBe(200)
  await expect(page.getByText('5', { exact: true }).first()).toBeVisible()
  await expect(page.getByText('Așteaptă revizuirea').first()).toBeVisible()
})

test('partial review remains blocked without continuation', async ({ page }) => {
  await page.goto('/invoices/inv-classification-review-api?tab=classification')
  await expect(page.getByText('5', { exact: true }).first()).toBeVisible()
  await expect(page.getByText('Decizie necesară')).toBeVisible()
  await expect(page.getByText('Așteaptă revizuirea').first()).toBeVisible()
})

test('accountant correction persists without resolving remaining items', async ({ page }) => {
  await page.goto('/invoices/inv-classification-correct-api?tab=classification')
  await page.getByRole('button', { name: 'Corectează' }).first().click()
  await page.getByLabel('Valoare corectată').fill('Valoare demonstrativă corectată')
  const response = page.waitForResponse((value) => value.url().includes('/classification-decisions'))
  await page.getByRole('button', { name: 'Salvează corecția' }).click()
  expect((await response).status()).toBe(200)
  await expect(page.getByText('Valoare demonstrativă corectată').first()).toBeVisible()
  await expect(page.getByText('Corectat').first()).toBeVisible()
  await expect(page.getByText('Așteaptă revizuirea').first()).toBeVisible()
})

test('final pending decisions resolve the task and resume the invoice', async ({ page }) => {
  await page.goto('/invoices/inv-classification-review-api?tab=classification')
  const acceptButtons = page.getByRole('button', { name: 'Acceptă propunerea' })
  await expect(acceptButtons.first()).toBeVisible()
  for (;;) {
    const pendingCount = await acceptButtons.count()
    if (pendingCount === 0) break
    const response = page.waitForResponse((value) => value.url().includes('/classification-decisions'))
    await acceptButtons.first().click()
    expect((await response).status()).toBe(200)
    await expect(acceptButtons).toHaveCount(pendingCount - 1)
  }
  await expect(page.getByRole('heading', { name: 'Pregătită pentru SAGA' })).toBeVisible()
  await expect(page.getByRole('heading', { name: 'Clasificări pe linii' })).toBeVisible()
  await expect(acceptButtons).toHaveCount(0)
  await page.reload()
  await expect(page.getByRole('heading', { name: 'Pregătită pentru SAGA' })).toBeVisible()
})

test('rule list detail and immutable history load from backend', async ({ page }) => {
  await page.goto('/rules')
  await expect(page.getByText('REG-DEMO-CONT-01').first()).toBeVisible()
  await page.getByRole('link', { name: 'Încadrare cont pentru servicii demonstrative' }).first().click()
  await expect(page.getByText('Istoric versiuni')).toBeVisible()
  await expect(page.getByText('Versiunea 2 · Curentă').first()).toBeVisible()
  await expect(page.getByText('Versiunea 1 · Istorică').first()).toBeVisible()
  await expect(page.getByText('Exemplu demonstrativ — bază legală nevalidată').first()).toBeVisible()
})

test('editing a rule creates a new immutable version', async ({ page }) => {
  await page.goto('/rules/rule-account-global')
  await page.getByRole('button', { name: 'Creează versiune nouă' }).click()
  await page.getByLabel('Criteriu / pattern').fill('Criteriu demonstrativ nou pentru test backend')
  await page.getByLabel('Rezultat clasificare').fill('Rezultat demonstrativ nou')
  const response = page.waitForResponse((value) => value.url().includes('/versions'))
  await page.getByRole('button', { name: 'Salvează versiunea nouă' }).click()
  expect((await response).status()).toBe(200)
  await expect(page.getByText('Versiunea 3 · Curentă').first()).toBeVisible()
  await expect(page.getByText('Versiunea 2 · Istorică').first()).toBeVisible()
})

test('creating a client override preserves the global origin', async ({ page }) => {
  await page.goto('/rules/rule-deductibility-global')
  await page.getByRole('button', { name: 'Creează override pentru client' }).click()
  const dialog = page.getByRole('dialog', { name: 'Creează override pentru client' })
  await dialog.locator('select[name="clientId"]').selectOption('client-alfa')
  await page.getByLabel('Criteriu / pattern').fill('Criteriu demonstrativ pentru Alfa')
  await page.getByLabel('Rezultat clasificare').fill('Rezultat demonstrativ Alfa')
  const response = page.waitForResponse((value) => value.url().includes('/client-overrides'))
  await page.getByRole('button', { name: 'Salvează override-ul' }).click()
  expect((await response).status()).toBe(200)
  await expect(page.locator('tbody').getByText('Override client', { exact: true }).first()).toBeVisible()
  await page.goto('/rules/rule-deductibility-global')
  await expect(page.getByText('Regulă globală').first()).toBeVisible()
})

test('classification correction does not create or mutate rules', async ({ page }) => {
  await page.goto('/rules')
  const before = await page.getByRole('row').count()
  await page.goto('/invoices/inv-classification-final-api?tab=classification')
  await page.getByRole('button', { name: 'Corectează' }).first().click()
  await page.getByLabel('Valoare corectată').fill('Corecție izolată pe factură')
  await page.getByRole('button', { name: 'Salvează corecția' }).click()
  await page.goto('/rules')
  await expect(page.getByRole('row')).toHaveCount(before)
})
