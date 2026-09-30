import { execFileSync } from 'node:child_process'
import type { Page } from '@playwright/test'
import { expect, test } from './fixtures/authenticated'

const clientId = 'client-contract-ingestion'
function pdf(name: string) {
  return execFileSync('go', ['run', './cmd/contractfixture', '-name', name], { cwd: 'backend', env: { ...process.env, GOCACHE: '/private/tmp/diana-go-cache' } })
}
// A row of the review list, by its label and value.
const reviewRow = (page: Page, name: RegExp) => page.getByRole('region', { name: 'Ce a extras Diana' }).getByRole('button', { name })
// Opens a row's value for correction.
async function correct(page: Page, name: RegExp) {
  await reviewRow(page, name).click()
  await page.getByRole('button', { name: 'Corectează' }).click()
}
// Ticks every item left to check as a reviewer would, leaving proposed AI
// rules for after confirmation, until the contract can be confirmed.
async function finishReview(page: Page) {
  const confirm = page.getByRole('button', { name: 'Confirmă contractul' })
  const remaining = page.getByRole('button', { name: /^Mai ai \d+ de verificat$/ })
  await expect(confirm.or(remaining)).toBeVisible({ timeout: 60_000 })
  for (let step = 0; step < 60 && !(await confirm.isVisible()); step += 1) {
    const before = (await remaining.textContent()) ?? ''
    const defer = page.getByRole('button', { name: 'Lasă pentru după confirmare' })
    if (await defer.isVisible()) await defer.click()
    else await page.getByRole('button', { name: /^Corect\s*↵$/ }).click()
    await expect(confirm.or(remaining.filter({ hasNotText: before }))).toBeVisible()
  }
  await expect(confirm).toBeEnabled()
}
test.describe.serial('Actual API → PostgreSQL → outbox → Asynq → review → ContractAvailable', () => {
  test.beforeAll(async ({ request }) => {
    const response = await request.get('/api/v1/clients')
    expect(response.ok(), 'The contract E2E API must return the client list').toBeTruthy()
    const clients = await response.json() as Array<{ id: string }>
    expect(clients.some(client => client.id === clientId), 'Run cmd/contractingestionseed in the isolated E2E DATABASE_URL before this suite (handoff section L)').toBeTruthy()
  })
  test('upload, human correction, explicit confirmation, and persisted provenance', async ({ page }) => {
    await page.goto(`/contracts/upload?clientId=${clientId}`)
    await page.getByLabel('Document contractual PDF').setInputFiles({ name: 'english.pdf', mimeType: 'application/pdf', buffer: pdf('english') })
    await page.getByRole('button', { name: 'Încarcă și extrage datele' }).click()
    await expect(reviewRow(page, /Referință contract.*CTR-2026-01/)).toBeVisible({ timeout: 60_000 })
    const reviewURL = page.url()
    await page.reload()
    await expect(reviewRow(page, /Referință contract.*CTR-2026-01/)).toBeVisible()
    await correct(page, /Referință contract/)
    await page.getByLabel('Referință contract').fill('CI-HUMAN-CORRECTION')
    // Keep the next missing-contract journey independent: explicitly reviewed supplier differs.
    await correct(page, /CUI furnizor/)
    await page.getByLabel('CUI furnizor').fill('RO87654321')
    await page.keyboard.press('Enter')
    await finishReview(page)
    await page.getByRole('button', { name: 'Confirmă contractul' }).click()
    await expect(page.getByRole('link', { name: 'Deschide contractul autoritativ' })).toBeVisible()
    await page.reload()
    await expect(page.getByRole('region', { name: 'Ce a extras Diana' }).getByRole('button', { name: /Referință contract.*CI-HUMAN-CORRECTION/ })).toBeVisible()
    await page.getByRole('link', { name: 'Deschide contractul autoritativ' }).click()
    await expect(page.getByRole('heading', { name: 'CI-HUMAN-CORRECTION' })).toBeVisible()
    await page.goto(reviewURL)
    await expect(page.getByRole('button', { name: 'Confirmă contractul' })).toHaveCount(0)
  })
  test('missing invoice uses the same upload flow and resumes only after confirmation', async ({ page }) => {
    await page.goto('/invoices/inv-contract-ingestion-waiting?tab=contract')
    await expect(page.getByText('Contract lipsă', { exact: true })).toBeVisible()
    await page.getByRole('button', { name: 'Solicită contract' }).click()
    await page.getByRole('link', { name: /Încarcă contract/ }).click()
    await expect(page).toHaveURL(/clientId=client-contract-ingestion/)
    await page.getByLabel('Document contractual PDF').setInputFiles({ name: 'romanian.pdf', mimeType: 'application/pdf', buffer: pdf('romanian') })
    await page.getByRole('button', { name: 'Încarcă și extrage datele' }).click()
    await expect(reviewRow(page, /Referință contract/)).toBeVisible({ timeout: 60_000 })
    const before = await page.request.get('http://127.0.0.1:8190/api/v1/invoices/inv-contract-ingestion-waiting')
    expect((await before.json()).pipelineStatus).toBe('AWAITING_CONTRACT')
    await finishReview(page)
    await page.getByRole('button', { name: 'Confirmă contractul' }).click()
    await expect(page.getByRole('link', { name: 'Deschide contractul autoritativ' })).toBeVisible()
    await page.goto('/invoices/inv-contract-ingestion-waiting?tab=contract')
    await expect(page.getByText('Contract asociat', { exact: true })).toBeVisible({ timeout: 60_000 })
    await page.reload()
    await expect(page.getByText('Contract asociat', { exact: true })).toBeVisible()
  })
  test('duplicate upload reuses the confirmed source instead of extracting again', async ({ page }) => {
    await page.goto(`/contracts/upload?clientId=${clientId}`)
    await page.getByLabel('Document contractual PDF').setInputFiles({ name: 'renamed.pdf', mimeType: 'application/pdf', buffer: pdf('english') })
    await page.getByRole('button', { name: 'Încarcă și extrage datele' }).click()
    await expect(page.getByText('Acest PDF există deja pentru client.', { exact: false })).toBeVisible()
    await expect(page.getByRole('link', { name: 'Deschide contractul autoritativ' })).toBeVisible()
    await expect(page.getByLabel('Referință contract')).toHaveCount(0)
  })
  test('image-only PDF uses the same persisted review boundary', async ({ page }) => {
    await page.goto(`/contracts/upload?clientId=${clientId}`)
    await page.getByLabel('Document contractual PDF').setInputFiles({ name: 'scan.pdf', mimeType: 'application/pdf', buffer: pdf('scanned') })
    await page.getByRole('button', { name: 'Încarcă și extrage datele' }).click()
    await expect(reviewRow(page, /Referință contract.*CTR-2026-01/)).toBeVisible({ timeout: 60_000 })
    await page.reload()
    await expect(reviewRow(page, /Monedă.*RON/)).toBeVisible()
    await expect(page.getByRole('link', { name: 'Deschide contractul autoritativ' })).toHaveCount(0)
  })
  test('discard wrong upload, review indefinite service terms, and confirm replacement', async ({ page }) => {
    await page.goto(`/contracts/upload?clientId=${clientId}`)
    await page.getByLabel('Document contractual PDF').setInputFiles({ name: 'wrong.pdf', mimeType: 'application/pdf', buffer: pdf('missing') })
    await page.getByRole('button', { name: 'Încarcă și extrage datele' }).click()
    await expect(reviewRow(page, /Referință contract/)).toBeVisible({ timeout: 60_000 })
    page.once('dialog', dialog => dialog.accept())
    await page.getByRole('button', { name: 'Renunță / Șterge documentul' }).click()
    await expect(page).toHaveURL(new RegExp(`/contracts/upload\\?clientId=${clientId}`))
    await page.getByLabel('Document contractual PDF').setInputFiles({ name: 'servicii.pdf', mimeType: 'application/pdf', buffer: pdf('service-indefinite') })
    await page.getByRole('button', { name: 'Încarcă și extrage datele' }).click()
    await expect(reviewRow(page, /Referință contract.*SERV-2026/)).toBeVisible({ timeout: 60_000 })
    await expect(page.getByLabel('Pagina PDF 1')).toBeVisible()
    await expect(reviewRow(page, /Durata.*Nedeterminată/)).toBeVisible()
    await expect(page.getByLabel('Data de sfârșit')).toHaveCount(0)
    await correct(page, /Servicii de contabilitate/)
    await expect(page.getByLabel('Serviciu 1', { exact: true })).toHaveValue('Servicii de contabilitate')
    await expect(page.getByLabel('Preț 1', { exact: true })).toHaveValue('500')
    await page.keyboard.press('Escape')
    await correct(page, /Salarizare și resurse umane/)
    await expect(page.getByLabel('Model tarifare 2', { exact: true })).toHaveValue('UNIT_RATE')
    await expect(page.getByLabel('Unitate 2', { exact: true })).toHaveValue('SALARIAT')
    await page.keyboard.press('Escape')
    await finishReview(page)
    await page.getByRole('button', { name: 'Confirmă contractul' }).click()
    await expect(page.getByRole('link', { name: 'Deschide contractul autoritativ' })).toBeVisible()
  })
  test('failed extraction never opens an empty manual-from-zero form', async ({ page }) => {
    await page.goto(`/contracts/upload?clientId=${clientId}`)
    await page.getByLabel('Document contractual PDF').setInputFiles({ name: 'unknown.pdf', mimeType: 'application/pdf', buffer: Buffer.from('%PDF-1.4\n%not-a-known-test-fixture\n%%EOF\n') })
    await page.getByRole('button', { name: 'Încarcă și extrage datele' }).click()
    await expect(page.getByRole('button', { name: 'Reîncearcă extragerea' })).toBeVisible({ timeout: 60_000 })
    await page.reload()
    await expect(page.getByLabel('Referință contract')).toHaveCount(0)
    await expect(page.getByRole('button', { name: 'Confirmă contractul' })).toHaveCount(0)
  })
})
