import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { vi } from 'vitest'
import type { ContractDocument } from '../../repositories/invoiceRepository'
import { ContractProposalReview } from './ContractDocumentReviewPage'

const handlers = vi.hoisted(() => ({ confirm: vi.fn(), retry: vi.fn() }))
vi.mock('./ContractPDFPreview', () => ({ ContractPDFPreview: () => <div>Private PDF preview</div> }))
vi.mock('./contract-ingestion-hooks', () => ({
  useConfirmContractDocument: () => ({ mutate: handlers.confirm, isPending: false, isError: false }),
  useRetryContractExtraction: () => ({ mutate: handlers.retry, isPending: false, isError: false }),
  useContractDocument: vi.fn(),
}))

function fixture(): ContractDocument {
  const field = (value: string) => ({ value, status: 'PRESENT' as const, confidence: 'HIGH' as const, evidence: { page: 1, snippet: value }, alternatives: [] })
  return {
    id: 'document-1', clientId: 'client-1', originalFilename: 'contract.pdf', mimeType: 'application/pdf', sizeBytes: 500,
    sha256: 'synthetic-hash', status: 'READY_FOR_REVIEW', lifecycleState: 'ACTIVE', revision: 3,
    uploadedAt: '2026-09-15T12:00:00Z', uploadedBy: 'Uploader', buyerMismatch: false,
    extraction: { id: 'attempt-1', provider: 'DETERMINISTIC_TEST', model: 'fixture-v1', schemaVersion: 'CONTRACT_EXTRACTION_V1',
      promptVersion: 'CONTRACT_EXTRACTION_PROMPT_V1', status: 'SUCCEEDED', startedAt: '2026-09-15T12:00:01Z',
      proposal: { supplierName: field('Supplier SRL'), supplierCui: field('RO12345678'), reference: field('AI-REFERENCE'),
        effectiveFrom: field('2026-01-01'), effectiveTo: field('2027-12-31'), totalValue: field('125000.00'), currency: field('RON'),
        unitType: field('servicii'), paymentTerms: field('30 zile'), buyerCui: field('RO10000000') } },
  }
}
function review(document = fixture(), onEvidence = vi.fn()) {
  render(<MemoryRouter><ContractProposalReview document={document} onEvidence={onEvidence} onStale={vi.fn()} /></MemoryRouter>)
}
beforeEach(() => vi.clearAllMocks())

describe('Contract ingestion human review boundary', () => {
  it('shows the proposal without creating a contract automatically', () => {
    review()
    expect(screen.getByLabelText('Referință contract')).toHaveValue('AI-REFERENCE')
    expect(screen.getByText('Datele nu sunt încă autoritative.', { exact: false })).toBeInTheDocument()
    expect(handlers.confirm).not.toHaveBeenCalled()
  })
  it('submits edited values and the exact extraction revision only on explicit confirmation', async () => {
    const user = userEvent.setup(); const document = fixture(); review(document)
    await user.clear(screen.getByLabelText('Referință contract'))
    await user.type(screen.getByLabelText('Referință contract'), 'USER-CORRECTION')
    expect(handlers.confirm).not.toHaveBeenCalled()
    await user.click(screen.getByRole('button', { name: 'Confirmă contractul' }))
    expect(handlers.confirm).toHaveBeenCalledWith(expect.objectContaining({ document, key: expect.any(String), contract: expect.objectContaining({ reference: 'USER-CORRECTION' }) }))
    expect(document.extraction?.proposal?.reference.value).toBe('AI-REFERENCE')
  })
  it('retains evidence page navigation independently of confirmation', async () => {
    const evidence = vi.fn(); review(fixture(), evidence)
    await userEvent.setup().click(screen.getAllByRole('button', { name: 'Pagina 1' })[0])
    expect(evidence).toHaveBeenCalledWith(1)
    expect(handlers.confirm).not.toHaveBeenCalled()
  })
  it('does not invent a missing currency and native validation blocks confirmation', async () => {
    const document = fixture(); document.extraction!.proposal!.currency = { value: null, status: 'MISSING', confidence: 'UNKNOWN', evidence: { page: null, snippet: '' }, alternatives: [] }
    review(document)
    expect(screen.getByLabelText('Monedă ISO')).toHaveValue('')
    expect(screen.getByText('Câmp obligatoriu lipsă')).toBeInTheDocument()
    await userEvent.setup().click(screen.getByRole('button', { name: 'Confirmă contractul' }))
    expect(handlers.confirm).not.toHaveBeenCalled()
  })
  it('blocks buyer mismatch rather than silently moving the document to another client', () => {
    review({ ...fixture(), buyerMismatch: true })
    expect(screen.getByRole('button', { name: 'Confirmă contractul' })).toBeDisabled()
    expect(screen.getByRole('alert')).toHaveTextContent('CUI-ul cumpărătorului')
  })
  it('keeps failure recovery out of the manual-from-zero form', async () => {
    review({ ...fixture(), status: 'EXTRACTION_FAILED' })
    expect(screen.queryByLabelText('Referință contract')).not.toBeInTheDocument()
    await userEvent.setup().click(screen.getByRole('button', { name: 'Reîncearcă extragerea' }))
    expect(handlers.retry).toHaveBeenCalledWith(3)
    expect(handlers.confirm).not.toHaveBeenCalled()
  })
  it('shows processing as persisted state without a blank manual form', () => {
    review({ ...fixture(), status: 'EXTRACTING' })
    expect(screen.getByRole('status')).toHaveTextContent('extrage automat')
    expect(screen.queryByLabelText('Referință contract')).not.toBeInTheDocument()
  })
})
