import type { Invoice, InvoiceLine } from '../../domain/invoice'
import type { CommercialFinding, CommercialValidationRun } from '../../repositories/invoiceRepository'
import { buildCommercialView, formatAmount, formatRate, invoiceFocus } from './commercial-view'

const line = (id: string, position: number, description: string, price: number): InvoiceLine => ({ id, position, description, unit: 'MON', quantity: 1, unitPrice: { amount: price, currency: 'RON' }, netValue: { amount: price, currency: 'RON' }, vatLabel: '21%', vatValue: { amount: price * 0.21, currency: 'RON' }, grossValue: { amount: price * 1.21, currency: 'RON' }, classifications: [], sourceFacts: { sourceId: String(position), path: `/Invoice/InvoiceLine[${position}]`, code: 'S', rate: '21', vatOrigin: 'CALCULATED' } })
const invoice = { id: 'invoice', clientId: 'client', supplierName: 'BYTEGUARD IT SOLUTIONS S.R.L.', documentNumber: 'BG26000108', issueDate: '2026-08-31', total: { amount: 2964.5, currency: 'RON' }, lines: [line('line-1', 1, 'Mentenanță IT — abonament', 1800), line('line-2', 2, 'Hosting cloud Business 2 VM', 650)] } as unknown as Invoice
const vatEvidence = [{ documentId: 'contract', page: 2, snippet: '4.3. Prețurile nu includ TVA. TVA se aplică în cota legală în vigoare (21% la data semnării).' }]
const run = (findings: CommercialFinding[], engineVersion = 'COMMERCIAL_VALIDATION_V2'): CommercialValidationRun => ({ id: 'run', invoiceId: 'invoice', dossierId: 'dossier', invoiceRevision: 7, snapshotVersion: 4, engineVersion, outcome: 'NEVERIFICABIL', findings, createdAt: '', completedAt: '' })

it('shows a VAT rate that matches on every line once, for the whole invoice', () => {
  const view = buildCommercialView(run([
    { id: 'vat-1', ruleId: 'vat', code: 'VAT_RATE_MATCH', outcome: 'CONFORM', lineId: 'line-1', actual: '21', expected: '21', calculation: 'Cota legală la data facturii: 21% · Codul fiscal', reason: 'Cota TVA a liniei corespunde cotei legale la care trimite contractul.', evidence: vatEvidence },
    { id: 'vat-2', ruleId: 'vat', code: 'VAT_RATE_MATCH', outcome: 'CONFORM', lineId: 'line-2', actual: '21', expected: '21', calculation: 'Cota legală la data facturii: 21% · Codul fiscal', reason: 'Cota TVA a liniei corespunde cotei legale la care trimite contractul.', evidence: vatEvidence },
    { id: 'price-2', ruleId: 'service-hosting', code: 'PRICE_MATCH', outcome: 'CONFORM', lineId: 'line-2', actual: '650.0000', expected: '650.0000', calculation: 'Hosting cloud — pachet Business 2 VM · 650.00 RON', reason: 'Prețul liniei corespunde calculului contractual.' },
    { id: 'uncovered-1', ruleId: 'line-coverage', code: 'SERVICE_LINE_UNCOVERED', outcome: 'NEVERIFICABIL', lineId: 'line-1', actual: 'Mentenanță IT — abonament', reason: 'Linia facturii nu are o regulă comercială confirmată aplicabilă.', serviceCandidates: [{ ruleId: 'service-maintenance', label: 'Mentenanță IT — abonament lunar', suggested: true, score: 1 }] },
  ]), invoice)
  expect(view.invoiceChecks).toHaveLength(1)
  expect(view.invoiceChecks[0]).toMatchObject({ title: 'Cota TVA', scope: 'liniile 1–2', invoiceValue: '21%', contractValue: '21%', contractDetail: 'cota legală la data facturii' })
  expect(view.lineGroups.map((group) => group.checks.map((item) => item.title))).toEqual([['Serviciu neasociat'], ['Preț unitar']])
  expect(view.lineGroups[1]).toMatchObject({ serviceLabel: 'Hosting cloud — pachet Business 2 VM' })
  expect(view.lineGroups[1].checks[0]).toMatchObject({ invoiceValue: '650,00 RON', contractValue: '650,00 RON' })
  expect(view.lineGroups[0].checks[0].explanation).toBe('Sugestie: Mentenanță IT — abonament lunar')
  expect(view.mappings.map((mapping) => mapping.line.id)).toEqual(['line-1'])
  expect(view.pendingCount).toBe(1)
  expect(view.verifiable.map((item) => item.id)).toEqual(['vat:vat', 'price-2'])
  expect(invoiceFocus(view.invoiceChecks[0], invoice)).toMatchObject({ lineIds: ['line-1', 'line-2'], field: 'vat' })
})

it('keeps a VAT mismatch on its own line', () => {
  const view = buildCommercialView(run([
    { id: 'vat-1', ruleId: 'vat', code: 'VAT_RATE_MATCH', outcome: 'CONFORM', lineId: 'line-1', actual: '21', expected: '21', reason: '' },
    { id: 'vat-2', ruleId: 'vat', code: 'VAT_RATE_MISMATCH', outcome: 'NECONFORM', lineId: 'line-2', actual: '19', expected: '21', reason: 'Cota TVA a liniei diferă de cota din contract.' },
  ]), invoice)
  expect(view.invoiceChecks).toHaveLength(0)
  expect(view.lineGroups[1].checks[0]).toMatchObject({ title: 'Cota TVA', outcome: 'NECONFORM', invoiceValue: '19%' })
})

it('reads a legacy VAT clause reported as a price check for what it is', () => {
  const legacy = (id: string, lineId: string): CommercialFinding => ({ id, ruleId: 'extracted-vat', code: 'PRICE_MATCH', outcome: 'CONFORM', lineId, actual: '21.0000', expected: '21.0000', calculation: 'Prețurile nu includ TVA. TVA se aplică în cota legală în vigoare (21% la data semnării).', reason: 'Prețul liniei corespunde calculului contractual.', evidence: vatEvidence })
  const view = buildCommercialView(run([legacy('a', 'line-1'), legacy('b', 'line-2')], 'COMMERCIAL_VALIDATION_V1'), invoice)
  expect(view.invoiceChecks.map((item) => [item.title, item.scope, item.invoiceValue])).toEqual([['Cota TVA', 'liniile 1–2', '21%']])
  expect(view.lineGroups).toHaveLength(0)
})

it('points at the XML element each value was read from', () => {
  const view = buildCommercialView(run([
    { id: 'ref', ruleId: 'contract-reference', code: 'CONTRACT_REFERENCE_MATCH', outcome: 'CONFORM', actual: 'BG-2025-117', actualSource: 'Descrierea liniei 1', expected: 'BG-2025-117', reason: '' },
    { id: 'due', ruleId: 'payment', code: 'PAYMENT_DUE_MATCH', outcome: 'CONFORM', actual: '15', expected: '15.00', reason: '' },
  ]), invoice)
  const reference = view.invoiceChecks.find((item) => item.id === 'ref')!
  const due = view.invoiceChecks.find((item) => item.id === 'due')!
  expect(invoiceFocus(reference, invoice)).toMatchObject({ lineIds: ['line-1'], field: 'description', text: 'BG-2025-117', sourcePath: '/Invoice/InvoiceLine[1]/cac:Item/cbc:Description' })
  expect(invoiceFocus(due, invoice)).toMatchObject({ header: 'dueDate' })
  expect(due).toMatchObject({ invoiceValue: '15 zile', contractValue: '15 zile' })
})

it('formats amounts and rates the Romanian way', () => {
  expect(formatAmount('1800.0000', 'RON')).toBe('1.800,00 RON')
  expect(formatRate('21.0000')).toBe('21%')
  expect(formatRate('5.5')).toBe('5,5%')
})
