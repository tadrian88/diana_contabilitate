import { expect, test } from '@playwright/test'

function kpi(page: import('@playwright/test').Page, label: string) {
  return page.locator(`[data-kpi="${label}"]`)
}

async function acceptAllClassificationProposals(page: import('@playwright/test').Page) {
  const buttons = page.getByRole('button', { name: 'Acceptă propunerea' })
  for (;;) {
    const count = await buttons.count()
    if (count === 0) return
    const response = page.waitForResponse((item) => item.request().method() === 'POST' && item.url().includes('/classification-decisions'))
    await buttons.first().click()
    expect((await response).status()).toBe(200)
    await expect(buttons).toHaveCount(count - 1)
  }
}

test('A. Dashboard derives the four approved KPIs from complete backend data', async ({ page }) => {
  const invoices = await (await page.request.get('/api/v1/invoices')).json() as Array<{ issueDate: string; pipelineStatus: string; task?: { status: string } }>
  const current = invoices.filter((item) => item.issueDate.startsWith('2026-09'))
  await page.goto('/')
  await expect(kpi(page, 'Facturi în procesare')).toContainText(String(current.filter((item) => !['EXPORTED', 'DUPLICATE'].includes(item.pipelineStatus)).length))
  await expect(kpi(page, 'Necesită atenție')).toContainText(String(current.filter((item) => item.task?.status === 'OPEN').length))
  await expect(kpi(page, 'Pregătite pentru SAGA')).toContainText(String(current.filter((item) => item.pipelineStatus === 'READY_FOR_SAGA').length))
  await expect(kpi(page, 'Exportate în SAGA')).toContainText(String(current.filter((item) => item.pipelineStatus === 'EXPORTED').length))
})

test('B. Client scope consistently filters backend invoices', async ({ page }) => {
  await page.goto('/invoices')
  await page.getByLabel('Selectează clientul').click()
  await page.getByRole('option', { name: 'Client Demo Alfa SRL' }).click()
  await expect(page.getByTestId('active-scope')).toHaveText('Client Demo Alfa SRL')
  await expect(page.getByText('M6-READY-002')).toBeVisible()
  await expect(page.getByText('M6-EXPORTED-003')).toHaveCount(0)
  await page.getByRole('link', { name: 'Contracte', exact: true }).click()
  await expect(page.getByText('CTR-DEMO-201')).toBeVisible()
  await expect(page.getByText('CTR-DEMO-501')).toHaveCount(0)
  await page.getByRole('link', { name: 'Task-uri', exact: true }).click()
  await page.getByRole('tab', { name: /În așteptare/ }).click()
  await expect(page.getByText('TASK-MC-WAIT')).toHaveCount(0)
  await page.getByRole('link', { name: 'Dashboard', exact: true }).click()
  await expect(page.getByTestId('active-scope')).toHaveText('Client Demo Alfa SRL')
  await expect(page.getByRole('heading', { name: 'Context pe clienți' })).toHaveCount(0)
})

test('C. Invoice list opens complete backend detail and lines', async ({ page }) => {
  await page.goto('/invoices')
  for (const column of ['Furnizor', 'Număr factură', 'Client', 'Valoare', 'Dată', 'Contract', 'Status pipeline', 'Număr probleme', 'Încredere', 'Status SAGA']) {
    await expect(page.getByRole('columnheader', { name: new RegExp(column) })).toBeVisible()
  }
  await page.goto('/invoices?pipeline=DUPLICATE')
  await expect(page.getByLabel('Status pipeline')).toHaveValue('DUPLICATE')
  await expect(page.getByText('M6-EXPORTED-003').first()).toBeVisible()
  await page.goto('/invoices?saga=FAILED')
  await expect(page.getByLabel('Status SAGA')).toHaveValue('FAILED')
  await expect(page.getByText('M6-SAGA-FAILED-005')).toBeVisible()
  await page.goto('/invoices')
  const numberCells = page.locator('tbody td:nth-child(2) a')
  await expect(numberCells.first()).toBeVisible()
  const expectedNumbers = [...await numberCells.allTextContents()].sort((left, right) => left.localeCompare(right, 'ro'))
  await page.getByRole('button', { name: 'Număr factură' }).click()
  await expect(page).toHaveURL(/sort=number&direction=asc/)
  await expect.poll(() => numberCells.allTextContents()).toEqual(expectedNumbers)
  await page.goto('/invoices')
  await page.getByLabel('Caută după furnizor sau număr factură').fill('M6-JOURNEY-001')
  const journeyRow = page.locator('tbody tr').filter({ hasText: 'M6-JOURNEY-001' })
  await expect(journeyRow).toContainText('2 candidați')
  await expect(journeyRow).toContainText('1')
  await page.getByRole('link', { name: 'M6-JOURNEY-001' }).click()
  await expect(page.getByRole('heading', { name: 'M6-JOURNEY-001' })).toBeVisible()
  await page.getByRole('tab', { name: 'Linii factură' }).click()
  await expect(page.getByRole('heading', { name: 'Liniile facturii nu sunt disponibile' })).toBeVisible()

  // The journey invoice is intentionally blocked before line reading. Exercise
  // the complete persisted line DTO on an invoice that has reached that stage.
  await page.goto('/invoices?q=CLASS-01')
  await page.getByRole('link', { name: 'CLASS-01' }).click()
  await page.getByRole('tab', { name: 'Linii factură' }).click()
  await expect(page.getByText('Serviciu demonstrativ standard')).toBeVisible()
})

test('D. Multiple contract confirmation is persisted after reload', async ({ page }) => {
  await page.goto('/invoices/inv-contract-multiple?tab=contract')
  const response = page.waitForResponse((item) => item.url().includes('/contract-confirmations'))
  await page.getByRole('button', { name: 'Confirmă', exact: true }).click()
  expect((await response).status()).toBe(200)
  await page.reload()
  await expect(page.getByText('Contract asociat')).toBeVisible()
  await expect(page.getByText('CTR-DEMO-201')).toBeVisible()
})

test('E. Missing-contract request remains WAITING after reload', async ({ page }) => {
  await page.goto('/invoices/inv-task-missing-open?tab=contract')
  const response = page.waitForResponse((item) => item.url().includes('/contract-requests'))
  await page.getByRole('button', { name: 'Solicită contract' }).click()
  expect((await response).status()).toBe(200)
  await page.reload()
  await expect(page.getByText('Contract solicitat', { exact: true })).toBeVisible()
  await expect(page.getByText('În așteptare', { exact: true }).first()).toBeVisible()
})

test('F. Partial classification review remains blocked and persists', async ({ page }) => {
  await page.goto('/invoices/inv-classification-correct-api?tab=classification')
  const response = page.waitForResponse((item) => item.request().method() === 'POST' && item.url().includes('/classification-decisions'))
  await page.getByRole('button', { name: 'Acceptă propunerea' }).first().click()
  expect((await response).status()).toBe(200)
  await page.reload()
  await expect(page.getByText('Așteaptă revizuirea').first()).toBeVisible()
  await expect(page.getByRole('button', { name: 'Acceptă propunerea' })).not.toHaveCount(0)
})

test('G. Final classification review removes the blocker and persists', async ({ page }) => {
  await page.goto('/invoices/inv-classification-final-api?tab=classification')
  await acceptAllClassificationProposals(page)
  await expect(page.getByText(/Pregătită pentru SAGA|Exportată/).first()).toBeVisible()
  await page.reload()
  await expect(page.getByRole('button', { name: 'Acceptă propunerea' })).toHaveCount(0)
})

test('H. New rule version and immutable history survive reload', async ({ page }) => {
  await page.goto('/rules/rule-module6-version')
  await page.getByRole('button', { name: 'Creează versiune nouă' }).click()
  await page.getByLabel('Criteriu / pattern').fill('Criteriu API persistent pentru Module 6')
  await page.getByLabel('Rezultat clasificare').fill('Rezultat API persistent')
  const response = page.waitForResponse((item) => item.request().method() === 'POST' && item.url().includes('/versions'))
  await page.getByRole('button', { name: 'Salvează versiunea nouă' }).click()
  expect((await response).status()).toBe(200)
  await expect(page.getByText('Versiunea 2 · Curentă').first()).toBeVisible()
  await page.reload()
  await expect(page.getByText('Versiunea 2 · Curentă').first()).toBeVisible()
  await expect(page.getByText('Versiunea 1 · Istorică').first()).toBeVisible()
})

test('I. Client override receives backend identity and keeps global origin', async ({ page }) => {
  await page.goto('/rules/rule-module6-override-parent')
  await page.getByRole('button', { name: 'Creează override pentru client' }).click()
  const dialog = page.getByRole('dialog', { name: 'Creează override pentru client' })
  await dialog.locator('select[name="clientId"]').selectOption('client-alfa')
  await dialog.getByLabel('Criteriu / pattern').fill('Override API persistent pentru Alfa')
  await dialog.getByLabel('Rezultat clasificare').fill('Rezultat override API')
  const response = page.waitForResponse((item) => item.request().method() === 'POST' && item.url().includes('/client-overrides'))
  await dialog.getByRole('button', { name: 'Salvează override-ul' }).click()
  const createdResponse = await response
  expect(createdResponse.status()).toBe(200)
  const created = await createdResponse.json() as { id: string; reference: string; scope: string; clientId?: string; parentRuleId?: string }
  expect(created).toMatchObject({ scope: 'CLIENT_OVERRIDE', clientId: 'client-alfa', parentRuleId: 'rule-module6-override-parent' })
  expect(created.id).toBeTruthy()
  const overrideRow = page.locator('tbody tr').filter({ hasText: created.reference })
  await expect(overrideRow.getByText('Override client', { exact: true })).toBeVisible()
  await expect(overrideRow.getByText('Rezultat override API', { exact: true })).toBeVisible()
  await page.goto('/rules/rule-module6-override-parent')
  await expect(page.getByText('Regulă globală').first()).toBeVisible()
})

test('J. Backend duplicate remains terminal after reload', async ({ page }) => {
  await page.goto('/invoices/inv-module6-duplicate')
  await expect(page.getByText('Duplicat', { exact: true }).first()).toBeVisible()
  await expect(page.getByText('Stare terminală. Factura nu continuă către SAGA.')).toBeVisible()
  await page.reload()
  await expect(page.getByText('Duplicat', { exact: true }).first()).toBeVisible()
})

test('K. Backend exported invoice remains terminal after reload', async ({ page }) => {
  await page.goto('/invoices/inv-module6-exported')
  await expect(page.getByText('Exportată în SAGA', { exact: true }).first()).toBeVisible()
  await page.reload()
  await expect(page.getByText('Exportată în SAGA', { exact: true }).first()).toBeVisible()
})

test('L. User-relevant persisted history is rendered after reload', async ({ page }) => {
  await page.goto('/invoices/inv-module6-exported?tab=history')
  await expect(page.getByText('Factură pregătită pentru demonstrație')).toBeVisible()
  await page.reload()
  await expect(page.getByText('Eveniment de business persistent pentru scenariul Module 6.')).toBeVisible()
})

test('M. API 404 never falls back to a mock-only invoice', async ({ page }) => {
  await page.goto('/invoices/inv-happy')
  await expect(page.getByRole('heading', { name: 'Factura nu a fost găsită' })).toBeVisible()
  await expect(page.getByText('DEMO-HP-001')).toHaveCount(0)
})

test('N. Full accountant journey is backend-owned and durable', async ({ page }) => {
  await page.goto('/invoices/inv-module6-journey?tab=contract')
  await expect(page.getByText('Contract recomandat')).toBeVisible()
  const confirmation = page.waitForResponse((item) => item.request().method() === 'POST' && item.url().includes('/contract-confirmations'))
  await page.getByRole('button', { name: 'Confirmă', exact: true }).click()
  expect((await confirmation).status()).toBe(200)
  await page.getByRole('tab', { name: 'Clasificare' }).click()
  await expect(page.getByText('Așteaptă revizuirea').first()).toBeVisible({ timeout: 15_000 })
  await acceptAllClassificationProposals(page)
  await expect(page.getByText('Exportată în SAGA', { exact: true }).first()).toBeVisible({ timeout: 15_000 })
  await page.getByRole('tab', { name: 'Istoric' }).click()
  await expect(page.getByText('Contract confirmat')).toBeVisible()
  await page.reload()
  await expect(page.getByText('Exportată în SAGA', { exact: true }).first()).toBeVisible()
  await page.goto('/rules')
  await expect(page.getByText('REG-DEMO-CONT-01').first()).toBeVisible()
})
