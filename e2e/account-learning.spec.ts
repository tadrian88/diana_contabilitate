import { execFileSync } from 'node:child_process'
import { expect, test } from './fixtures/authenticated'

const databaseURL = 'postgresql://diana:diana@127.0.0.1:5442/diana_backend_e2e?sslmode=disable'

type LearningState = {
  mappingCount: number
  mappingVersionCount: number
  currentVersion: number
  i2Source: string
  i2ReviewStatus: string
}

function fixture(action: 'state' | 'process-i2'): LearningState {
  const output = execFileSync('go', ['run', './cmd/accountlearninge2e', '-action', action], {
    cwd: 'backend',
    encoding: 'utf8',
    env: { ...process.env, APP_ENV: 'test', DATABASE_URL: databaseURL, GOCACHE: '/private/tmp/diana-go-cache' },
  })
  return JSON.parse(output) as LearningState
}

test('I1 CREATE mapping → independent I2 LEARNED_MAPPING → VALIDATE', async ({ page }) => {
  test.setTimeout(120_000)

  await page.goto('/invoices/inv-account-learning-i1?tab=classification')
  const i1Account = page.locator('article').filter({ has: page.getByRole('heading', { name: 'Cont contabil', exact: true }) })
  await expect(i1Account.getByText('Sursă: mapare confirmată anterior')).toHaveCount(0)
  await expect(i1Account.getByRole('button', { name: 'Selectează cont' })).toBeVisible()
  await i1Account.getByRole('button', { name: 'Selectează cont' }).click()

  const dialog = page.getByRole('dialog', { name: 'Selectează contul Diana' })
  await dialog.getByRole('combobox').click()
  await dialog.getByLabel('Caută în conturi').fill('6281')
  await dialog.getByRole('option', { name: '6281 — Cheltuieli cu serviciile IT', exact: true }).click()
  await expect(dialog.getByRole('combobox')).toContainText('6281 — Cheltuieli cu serviciile IT')
  await expect(dialog.getByText('Reutilizează pentru linii viitoare')).toBeVisible()
  await dialog.getByText('Reutilizează pentru linii viitoare').click()
  await expect(dialog.getByLabel('Scope mapare reutilizabilă')).toContainText('Supplier S')
  const createResponse = page.waitForResponse(response => response.request().method() === 'POST' && response.url().includes('/classification-decisions'))
  await dialog.getByRole('button', { name: 'Salvează decizia' }).click()
  const created = await createResponse
  expect(created.status(), await created.text()).toBe(200)
  await expect(i1Account.getByText('6281 — Cheltuieli cu serviciile IT')).toBeVisible()
  await expect(i1Account.getByText('Corectat', { exact: true })).toBeVisible()

  expect(fixture('state')).toMatchObject({ mappingCount: 1, mappingVersionCount: 1, currentVersion: 1, i2Source: '' })

  expect(fixture('process-i2')).toMatchObject({
    mappingCount: 1,
    mappingVersionCount: 1,
    currentVersion: 1,
    i2Source: 'LEARNED_MAPPING',
    i2ReviewStatus: 'PENDING',
  })

  await page.goto('/invoices/inv-account-learning-i2?tab=classification')
  const i2Account = page.locator('article').filter({ has: page.getByRole('heading', { name: 'Cont contabil', exact: true }) })
  await expect(i2Account.getByText('6281 — Cheltuieli cu serviciile IT')).toBeVisible()
  await expect(i2Account.getByText('Sursă: mapare confirmată anterior')).toBeVisible()
  await expect(i2Account.getByText('Necesită validare', { exact: true })).toBeVisible()
  await expect(i2Account.getByText('De revizuit', { exact: true })).toBeVisible()
  await expect(i2Account.getByRole('button', { name: 'Validează' })).toBeVisible()

  const validateResponse = page.waitForResponse(response => response.request().method() === 'POST' && response.url().includes('/classification-decisions'))
  await i2Account.getByRole('button', { name: 'Validează' }).click()
  const validated = await validateResponse
  expect(validated.status(), await validated.text()).toBe(200)
  await expect(i2Account.getByText('Acceptat', { exact: true })).toBeVisible()

  expect(fixture('state')).toMatchObject({
    mappingCount: 1,
    mappingVersionCount: 1,
    currentVersion: 1,
    i2Source: 'LEARNED_MAPPING',
    i2ReviewStatus: 'ACCEPTED',
  })
})
