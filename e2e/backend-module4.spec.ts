import { expect, test } from './fixtures/authenticated'

test('unique compatible contract is associated automatically without a task', async ({ page }) => {
  await page.goto('/invoices/inv-contract-auto?tab=contract')
  await expect(page.getByText('Contract asociat')).toBeVisible()
  await expect(page.getByText('CTR-DEMO-100')).toBeVisible()
  await expect(page.getByText('Duplicate verificate').first()).toBeVisible()
  await expect(page.getByText('Decizie necesară')).toHaveCount(0)
})

test('multiple plausible contracts expose persisted recommendation and alternative', async ({ page }) => {
  await page.goto('/invoices/inv-contract-multiple?tab=contract')
  await expect(page.getByText('Contract recomandat')).toBeVisible()
  await expect(page.getByText('CTR-DEMO-201')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Alege alt contract' })).toBeVisible()
})

test('recommended candidate confirmation persists after refresh', async ({ page }) => {
  await page.goto('/invoices/inv-contract-multiple?tab=contract')
  const responsePromise = page.waitForResponse((response) => response.url().includes('/contract-confirmations') && response.request().method() === 'POST')
  await page.getByRole('button', { name: 'Confirmă', exact: true }).click()
  expect((await responsePromise).status()).toBe(200)
  await expect(page.getByText('Contract asociat')).toBeVisible()
  await expect(page.getByText('CTR-DEMO-201')).toBeVisible()
  await page.reload()
  await expect(page.getByText('Contract asociat')).toBeVisible()
  await expect(page.getByText('CTR-DEMO-201')).toBeVisible()
})

test('alternative candidate can be selected from the persisted match result', async ({ page }) => {
  await page.goto('/invoices/inv-contract-multiple-alt?tab=contract')
  await page.getByRole('button', { name: 'Alege alt contract' }).click()
  await page.getByRole('radio').click()
  const responsePromise = page.waitForResponse((response) => response.url().includes('/contract-confirmations'))
  await page.getByRole('button', { name: 'Confirmă selecția' }).click()
  expect((await responsePromise).status()).toBe(200)
  await expect(page.getByText('Contract asociat')).toBeVisible()
  await expect(page.getByText('CTR-DEMO-202')).toBeVisible()
})

test('unique incompatible contract requires review instead of automatic association', async ({ page }) => {
  await page.goto('/invoices/inv-contract-incompatible?tab=contract')
  await expect(page.getByText('Contract recomandat')).toBeVisible()
  await expect(page.getByText('CTR-DEMO-501')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Confirmă', exact: true })).toBeVisible()
  await expect(page.getByText('Contract asociat')).toHaveCount(0)
})

test('no contract creates the real missing-contract blocker', async ({ page }) => {
  await page.goto('/invoices/inv-contract-missing?tab=contract')
  await expect(page.getByText('Contract lipsă')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Solicită contract' })).toBeVisible()
  await expect(page.getByText('Așteaptă contract').first()).toBeVisible()
})

test('contract detail reads persisted invoice associations', async ({ page }) => {
  await page.goto('/contracts/contract-demo-100')
  await expect(page.getByRole('heading', { name: 'CTR-DEMO-100' })).toBeVisible()
  await expect(page.getByRole('heading', { name: 'Facturi asociate' })).toBeVisible()
  await expect(page.getByRole('link', { name: 'BACKEND-AUTO-001' })).toBeVisible()
})
