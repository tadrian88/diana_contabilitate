import type { ContractDocument, ExtractedContractField } from '../../repositories/invoiceRepository'
import { buildConfirmedItems, buildReviewItems, initialReviewValues, reviewProgress, valueStatedInSnippet, type PdfTextIndex } from './contract-review-model'

describe('valueStatedInSnippet', () => {
  it.each([
    ['supplierCui', 'RO38920171', '…cod de înregistrare fiscală RO38920171', true],
    ['supplierCui', '38920171', 'CUI RO 38920171, J2018004511402', true],
    ['supplierCui', 'RO38920171', 'cod fiscal RO38920172', false],
    ['supplierCui', 'RO38920171', 'J2018004511402', false],
    ['effectiveFrom', '2026-01-01', 'începând cu 01.01.2026', true],
    ['effectiveFrom', '2026-01-01', 'începând cu 1.1.2026', true],
    ['effectiveFrom', '2026-01-01', 'din 1 ianuarie 2026', true],
    ['effectiveFrom', '2026-01-01', 'din 01 / 01 / 2026', true],
    ['effectiveFrom', '2026-01-01', 'din 11.01.2026', false],
    ['totalValue', '1800.00', 'tarif lunar 1.800,00 lei', true],
    ['totalValue', '1800', 'tarif lunar 1 800,00 lei', true],
    ['totalValue', '1800.00', 'monthly fee 1,800.00 RON', true],
    ['totalValue', '180.00', 'tarif lunar 1.800,00 lei', false],
    ['currency', 'RON', '1.800,00 lei', true],
    ['currency', 'RON', 'un leu', true],
    ['currency', 'EUR', '500 euro', true],
    ['currency', 'RON', '500 euro', false],
    ['periodType', 'INDEFINITE_TERM', 'pe durată nedeterminată', true],
    ['periodType', 'FIXED_TERM', 'pe durată nedeterminată', false],
    ['periodType', 'FIXED_TERM', 'pe durată determinată de 12 luni', true],
    ['supplierName', 'BYTEGUARD IT SOLUTIONS S.R.L.', '1.1. Byteguard IT Solutions S.R.L., cu sediul', true],
    ['paymentTerms', '30 zile', 'în termen de 30 de zile', false],
  ] as const)('%s %s in „%s” → %s', (key, value, snippet, stated) => {
    expect(valueStatedInSnippet(key, value, snippet)).toBe(stated)
  })

  it('does not apply to the document role and fails without a snippet', () => {
    expect(valueStatedInSnippet('documentRole', 'BASE_CONTRACT', 'Contract de prestări servicii')).toBeNull()
    expect(valueStatedInSnippet('reference', 'BG-2025-117', undefined)).toBe(false)
  })
})

const read = (value: string, snippet = value, confidence: ExtractedContractField['confidence'] = 'HIGH'): ExtractedContractField => ({ value, status: 'PRESENT', confidence, evidence: { page: 1, snippet }, alternatives: [] })
const missing: ExtractedContractField = { value: null, status: 'MISSING', confidence: 'UNKNOWN', evidence: { page: null }, alternatives: [] }

function document(): ContractDocument {
  return {
    id: 'document-1', clientId: 'client-1', originalFilename: 'contract.pdf', mimeType: 'application/pdf', sizeBytes: 1, sha256: 'hash', status: 'READY_FOR_REVIEW', lifecycleState: 'ACTIVE', revision: 1, uploadedAt: '2026-09-30T10:00:00Z', buyerMismatch: false,
    extraction: { id: 'attempt-1', provider: 'TEST', model: 'm', schemaVersion: 's', promptVersion: 'p', status: 'SUCCEEDED', startedAt: '2026-09-30T10:00:00Z', proposal: {
      supplierName: read('BYTEGUARD IT SOLUTIONS S.R.L.', 'BYTEGUARD IT SOLUTIONS S.R.L., cu sediul'), supplierCui: read('RO38920171', 'cod de înregistrare fiscală RO38920171'), buyerCui: read('RO10000000', 'CUI RO10000000'),
      reference: read('BG-2025-117', 'Contract nr. BG-2025-117'), effectiveFrom: read('2026-01-01', 'începând cu 01.01.2026'), effectiveTo: missing, totalValue: missing, currency: read('RON', 'prețuri în lei'),
      unitType: read('lună', 'tarif / lună', 'MEDIUM'), paymentTerms: read('30 zile', 'în 30 zile de la emitere'), periodType: read('INDEFINITE_TERM', 'pe durată nedeterminată'), documentRole: read('BASE_CONTRACT', 'CONTRACT DE PRESTĂRI SERVICII'),
      serviceTerms: [], commercialClauses: [],
    } },
  }
}
const exactPdf: PdfTextIndex = { ready: true, scanned: false, locate: () => ({ location: 'EXACT', page: 1 }) }

describe('buildReviewItems', () => {
  it('pre-ticks only high-confidence values found exactly in the PDF and stated by their words', () => {
    const doc = document()
    const items = buildReviewItems(doc, initialReviewValues(doc.extraction!.proposal), {}, exactPdf)
    const tone = (id: string) => items.find((item) => item.id === id)?.tone
    expect(tone('field:supplierCui')).toBe('ok')
    expect(tone('field:effectiveFrom')).toBe('ok')
    expect(tone('field:periodType')).toBe('ok')
    expect(tone('field:unitType')).toBe('attention')
    // The contract states no total value: the reviewer confirms it is absent.
    expect(tone('field:totalValue')).toBe('attention')
    expect(reviewProgress(items).remaining.map((item) => item.id)).toEqual(['field:totalValue', 'field:unitType'])
  })

  it('ticks nothing while the PDF text is read and nothing it could not locate', () => {
    const doc = document()
    const values = initialReviewValues(doc.extraction!.proposal)
    expect(buildReviewItems(doc, values, {}, { ...exactPdf, ready: false }).find((item) => item.id === 'field:supplierCui')?.tone).toBe('unknown')
    expect(buildReviewItems(doc, values, {}, { ...exactPdf, locate: () => ({ location: 'PARTIAL', page: 1 }) }).find((item) => item.id === 'field:supplierCui')?.tone).toBe('attention')
  })

  it('keeps the reviewer decision and reports a blocker on its own row', () => {
    const doc = document()
    const values = { ...initialReviewValues(doc.extraction!.proposal), paymentTerms: '' }
    const items = buildReviewItems(doc, values, { 'field:unitType': 'CHECKED', 'field:paymentTerms': 'CHECKED' }, exactPdf)
    expect(items.find((item) => item.id === 'field:unitType')).toMatchObject({ tone: 'ok', status: 'Verificat de tine' })
    expect(items.find((item) => item.id === 'field:paymentTerms')).toMatchObject({ tone: 'attention', blocker: 'Termenii de plată sunt obligatorii.' })
  })
})

describe('buildConfirmedItems', () => {
  it('reads each confirmed service with the evidence of its own proposal row', () => {
    const doc = document()
    doc.status = 'CONFIRMED'
    const hosting = { serviceDescription: read('Hosting cloud'), pricingModel: read('FIXED_FEE'), unitPrice: read('650.00', '650,00 lei / lună'), currency: read('RON', '650,00 lei / lună'), unit: missing, quantitySource: missing, quantityValue: missing, quantityDriver: missing, billingFrequency: read('MONTHLY') }
    const maintenance = { ...hosting, serviceDescription: read('Mentenanță IT'), unitPrice: read('1800.00', '1.800,00 lei / lună') }
    doc.extraction!.proposal!.serviceTerms = [maintenance, hosting]
    doc.confirmedValues = { ...initialReviewValues(doc.extraction!.proposal), serviceTerms: [{ ...initialReviewValues(doc.extraction!.proposal).serviceTerms[1], sourceIndex: 1 }] }
    const [service] = buildConfirmedItems(doc).filter((item) => item.group === 'services')
    expect(service.highlights).toEqual(['Hosting cloud', '650,00 lei / lună'])
  })
})
