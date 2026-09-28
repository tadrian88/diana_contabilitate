import { expect, test } from './fixtures/authenticated'

const typed = [
  { id: 'c-account', dimension: 'ACCOUNT', proposedValue: '626', proposedTypedValue: { kind: 'ACCOUNT', account: '626' } },
  { id: 'c-vat', dimension: 'VAT_TREATMENT', proposedValue: 'TVA obișnuită', proposedTypedValue: { kind: 'ORDINARY', timing: 'IMMEDIATE', sourceCategory: 'S', sourceRate: '21' } },
  { id: 'c-vat-deduction', dimension: 'VAT_DEDUCTIBILITY', proposedValue: 'Integral', proposedTypedValue: { kind: 'FULL' } },
  { id: 'c-expense', dimension: 'EXPENSE_TAX_TREATMENT', proposedValue: 'Integral deductibilă', proposedTypedValue: { kind: 'FULLY_DEDUCTIBLE' } },
]

function invoice(state: 'RUNNING' | 'REVIEW' | 'COMPLETED'): any {
  const reviewing = state === 'REVIEW'
  const completed = state === 'COMPLETED'
  const classifications = typed.map((item) => ({
    ...item, modelVersion: 'ACCOUNTING_DOMAIN_V2', value: completed ? item.proposedValue : 'Necesită decizie', confidence: 'HIGH',
    explanation: 'TEST_ONLY propunere automată validată granular.', legalBasis: 'TEST_ONLY art. C', status: completed ? 'ACCEPTED' : 'PENDING',
    effectiveSource: completed ? 'MANUAL' : undefined, humanReviewed: completed, source: 'AI_PROPOSAL', revision: completed ? 2 : 1, validationResults: [],
    legalCitations: [{ fragmentId: 'fragment-c', versionId: 'version-c', citationKey: 'TEST_ONLY art. C', contentHash: 'hash', verified: true }],
  }))
  return {
    id: 'inv-workflow-c', clientId: 'TEST_ONLY-accounting-client', supplierName: 'Orange TEST_ONLY', supplierCui: 'RO999999', documentNumber: 'C-001', issueDate: '2026-09-25T00:00:00Z',
    total: { amount: 121, currency: 'RON' }, spvReference: 'TEST_ONLY-C', pipelineStatus: state === 'RUNNING' ? 'CLASSIFIED' : completed ? 'READY_FOR_SAGA' : 'AWAITING_REVIEW',
    accountingWorkflowStatus: state === 'RUNNING' ? 'AI_ANALYSIS_RUNNING' : completed ? 'COMPLETED' : 'REVIEW_REQUIRED', sagaStatus: completed ? 'READY' : 'NOT_READY', revision: completed ? 5 : reviewing ? 4 : 3,
    modelVersion: 'ACCOUNTING_DOMAIN_V2', currentClassificationRunId: 'run-c', activity: [], lines: [{ id: 'line-c', position: 1, description: 'Abonament Smart 15', unit: 'H87', quantity: 1, unitPrice: { amount: 100, currency: 'RON' }, netValue: { amount: 100, currency: 'RON' }, vatRate: 21, vatValue: { amount: 21, currency: 'RON' }, totalValue: { amount: 121, currency: 'RON' }, sourceFacts: { sourceId: '1', path: '/line/1', code: 'S', rate: '21', vatOrigin: 'DECLARED' }, classifications }],
    ...(reviewing ? { task: { id: 'task-c', classificationRunId: 'run-c', type: 'CLASSIFICATION', status: 'OPEN', createdAt: '2026-09-25T10:00:00Z', updatedAt: '2026-09-25T10:00:00Z', title: 'Revizuiește clasificarea contabilă', reason: '4 dimensiuni necesită decizia contabilului.', revision: 1, classificationItems: classifications.map((item) => ({ ...item, lineId: 'line-c', lineLabel: 'Linia 1 · Abonament Smart 15' })) } } : {}),
  }
}

test('automatic AI stage becomes one review task and Approve All completes it', async ({ page }) => {
  let state: 'RUNNING' | 'REVIEW' | 'COMPLETED' = 'RUNNING'
  let promotionCalls = 0
  await page.route('**/promote', async route => { promotionCalls++; await route.fallback() })
  await page.route('**/api/v1/invoices/inv-workflow-c', async (route) => {
    if (route.request().method() === 'GET') return route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(invoice(state)) })
    return route.fallback()
  })
  await page.route('**/api/v1/invoices/inv-workflow-c/classification-decisions/approve-all', async (route) => {
    const body = route.request().postDataJSON()
    expect(body.taskId).toBe('task-c')
    expect(body.expected).toHaveLength(4)
    state = 'COMPLETED'
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(invoice(state)) })
  })

  await page.goto('/invoices/inv-workflow-c?tab=classification')
  await expect(page.getByText('Analiza contabilă este în curs')).toBeVisible()
  state = 'REVIEW'
  await page.reload()
  await expect(page.getByText('Factura necesită verificarea ta')).toBeVisible()
  await expect(page.getByText(/Sursă: Propunere automată/)).toHaveCount(4)
  await page.getByRole('button', { name: 'Aprobă toate propunerile valide' }).click()
  await expect(page.getByRole('button', { name: 'Aprobă toate propunerile valide' })).toHaveCount(0)
  await expect(page.getByText('Clasificare contabilă finalizată')).toBeVisible()
  await expect(page.getByText('Finală')).toHaveCount(4)
  expect(promotionCalls).toBe(0)
})

test('a final decision can be promoted only after the user inspects and confirms its scope', async ({ page }) => {
  const preview = {
    classificationId: 'c-account', classificationRevision: 2, classificationRunId: 'run-c', dimension: 'ACCOUNT',
    value: { kind: 'ACCOUNT', account: '626' },
    scope: { clientId: 'TEST_ONLY-accounting-client', clientDisplay: 'Client Orange TEST_ONLY', supplierDisplay: 'Orange TEST_ONLY', normalizedSupplierId: 'orange-test-only', serviceIdentityKind: 'NORMALIZED_DESCRIPTION', serviceIdentityValue: 'abonament smart 15', normalizerVersion: 'NORMALIZED_DESCRIPTION_V1', currency: 'RON', documentType: 'INVOICE', vatRate: '21', profileId: 'profile-test-only', profileVersion: 1 },
  }
  await page.route('**/api/v1/invoices/inv-workflow-c', route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(invoice('COMPLETED')) }))
  await page.route('**/api/v1/clients/TEST_ONLY-accounting-client/invoices/inv-workflow-c/classifications/c-account/reuse-preview', route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(preview) }))
  await page.route('**/api/v1/clients/TEST_ONLY-accounting-client/invoices/inv-workflow-c/classifications/c-account/promote', async route => {
    expect(route.request().postDataJSON()).toMatchObject({ expectedClassificationRevision: 2, expectedInvoiceRevision: 5, expectedClassificationRunId: 'run-c' })
    await route.fulfill({ status: 201, contentType: 'application/json', body: JSON.stringify({ id: 'knowledge-test-only', version: 1, dimension: 'ACCOUNT', value: preview.value, scope: preview.scope, status: 'ACTIVE' }) })
  })

  await page.goto('/invoices/inv-workflow-c?tab=classification')
  const account = page.locator('article[aria-label="Cont contabil"]')
  await account.getByRole('button', { name: 'Folosește pentru situații similare' }).click()
  const dialog = page.getByRole('dialog', { name: 'Confirmă reutilizarea deciziei' })
  await expect(dialog).toContainText('Client Orange TEST_ONLY')
  await expect(dialog).toContainText('Descriere normalizată = abonament smart 15')
  await dialog.getByRole('button', { name: 'Confirmă reutilizarea' }).click()
  await expect(page.getByText('Decizia este acum reutilizabilă pentru scope-ul confirmat.')).toBeVisible()
})

test('invalid 628 explains the synthetic account and accepts an analytic suggestion', async ({ page }) => {
  const review = invoice('REVIEW')
  const account = review.task.classificationItems[0]
  account.proposedValue = '628'
  account.proposedTypedValue = { kind: 'ACCOUNT', account: '628' }
  account.validationResults = [{ code: 'ACCOUNT_NOT_POSTABLE', message: 'technical', suggestedAccounts: ['6281'] }]
  await page.route('**/api/v1/accounts?*', route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify([{code:'6281',name:'Alte cheltuieli cu serviciile executate de terți',accountType:'EXPENSE',synthetic:false,postable:true,active:true}]) }))
  await page.route('**/api/v1/invoices/inv-workflow-c', route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(review) }))
  await page.route('**/api/v1/invoices/inv-workflow-c/classification-decisions', async route => {
    expect(route.request().postDataJSON()).toMatchObject({ action:'EDIT', typedValue:{kind:'ACCOUNT',account:'6281'} })
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(invoice('COMPLETED')) })
  })
  await page.goto('/invoices/inv-workflow-c?tab=classification')
  await expect(page.getByText(/Contul este sintetic și nu poate fi utilizat direct/)).toBeVisible()
  await page.getByRole('button',{name:/Selectează contul 6281/}).click()
  await expect(page.getByText('Clasificare contabilă finalizată')).toBeVisible()
})

test('stale analysis starts a new current run', async ({ page }) => {
  let current = invoice('REVIEW')
  const stale = current
  stale.classificationContext = {runId:'run-old',profileVersion:1,contextStale:true,staleReasons:['FISCAL_PROFILE_CHANGED'],createdAt:'2026-09-24T10:00:00Z'}
  await page.route('**/api/v1/invoices/inv-workflow-c', route => route.fulfill({ status:200,contentType:'application/json',body:JSON.stringify(current) }))
  await page.route('**/api/v1/clients/TEST_ONLY-accounting-client/invoices/inv-workflow-c/classification/reanalyze', route => { current=invoice('RUNNING');return route.fulfill({status:200,contentType:'application/json',body:JSON.stringify(current)}) })
  await page.goto('/invoices/inv-workflow-c?tab=classification')
  await expect(page.getByText(/Configurația clientului s-a modificat/)).toBeVisible()
  await page.getByRole('button',{name:'Reanalizează factura'}).click()
  await expect(page.getByText('Analiza contabilă este în curs')).toBeVisible()
})

test('provider failure remains completable through structured manual review', async ({ page }) => {
  const failed = invoice('COMPLETED')
  failed.pipelineStatus = 'AWAITING_REVIEW'; failed.accountingWorkflowStatus = 'REVIEW_REQUIRED'; failed.sagaStatus = 'NOT_READY'; failed.revision = 6
  failed.task = {id:'task-manual',classificationRunId:'run-c',type:'CLASSIFICATION',status:'OPEN',createdAt:'2026-09-25T10:00:00Z',title:'Completează manual',reason:'Analiza asistată nu este disponibilă; completați manual clasificările.',revision:1,classificationItems:[{...typed[2],lineId:'line-c',lineLabel:'Abonament Smart 15',proposedValue:'Necesită decizie',proposedTypedValue:undefined,confidence:'UNKNOWN',explanation:'Completează manual.',legalBasis:'Verificare contabilă',status:'PENDING',source:'NO_MATCH',revision:1,validationResults:[]}]}
  await page.route('**/api/v1/invoices/inv-workflow-c', route => route.fulfill({status:200,contentType:'application/json',body:JSON.stringify(failed)}))
  await page.route('**/api/v1/invoices/inv-workflow-c/classification-decisions', async route => { expect(route.request().postDataJSON()).toMatchObject({action:'EDIT',typedValue:{kind:'FULL'}});await route.fulfill({status:200,contentType:'application/json',body:JSON.stringify(invoice('COMPLETED'))}) })
  await page.goto('/invoices/inv-workflow-c?tab=classification')
  await expect(page.getByText('Analiza automată nu a putut fi finalizată.')).toBeVisible()
  await page.getByLabel('Drept de deducere TVA').getByRole('button',{name:'Modifică'}).click()
  await page.getByLabel('Motiv / documente justificative').fill('Verificare manuală pe documente')
  await page.getByRole('button',{name:'Salvează decizia'}).click()
  await expect(page.getByText('Clasificare contabilă finalizată')).toBeVisible()
})
