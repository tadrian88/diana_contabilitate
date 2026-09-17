import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import type { ClassificationReviewItem, Invoice, DomainValue } from '../../domain/invoice'
import { DomainCorrectionDialog } from './DomainCorrectionDialog'
import { ClassificationWorkspace } from './ClassificationWorkspace'
import { CLASSIFICATION_DIMENSION_LABELS } from '../../domain/invoice'

function fixture(): Invoice {
  return { id: 'TEST_ONLY-domain', scenario: 'PROCESSING', primaryDemo: false, clientId: 'TEST_ONLY-client', supplierName: 'TEST_ONLY supplier', documentNumber: 'TEST_ONLY-001', issueDate: '2026-09-15', total: { amount: 121, currency: 'RON' }, spvReference: 'TEST_ONLY-source', modelVersion: 'ACCOUNTING_DOMAIN_V2', pipelineStatus: 'READY_FOR_SAGA', pipelinePath: ['DOWNLOADED', 'ARCHIVED', 'MATCHING', 'DEDUPE_CHECKED', 'HEADER_READ', 'LINES_READ', 'CLASSIFIED', 'READY_FOR_SAGA', 'EXPORTING', 'EXPORTED'], sagaStatus: 'READY', autoRun: false, revision: 3, authority: 'MOCK', activity: [], lines: [{ id: 'line', position: 1, description: 'TEST_ONLY service', unit: 'H87', quantity: 1, unitPrice: { amount: 100, currency: 'RON' }, netValue: { amount: 100, currency: 'RON' }, vatValue: { amount: 21, currency: 'RON' }, grossValue: { amount: 121, currency: 'RON' }, vatLabel: '21%', sourceFacts: { sourceId: 'SOURCE-1', path: '/Invoice/InvoiceLine[1]', code: 'S', rate: '21', scheme: 'VAT', vatOrigin: 'CALCULATED' }, classifications: [] }] }
}
function item(dimension: ClassificationReviewItem['dimension']): ClassificationReviewItem { return { id: 'decision', lineId: 'line', lineLabel: 'TEST_ONLY service', dimension, proposedValue: 'Necesită decizie', confidence: 'Necesită revizuire', explanation: 'TEST_ONLY', legalBasis: 'TEST_ONLY', status: 'PENDING', revision: 1, modelVersion: 'ACCOUNTING_DOMAIN_V2' } }

describe('accounting domain v2', () => {
  it('shows four dimensions and source VAT origin without legacy instruction', () => {
    render(<MemoryRouter><ClassificationWorkspace invoice={fixture()} /></MemoryRouter>)
    expect(screen.getByText('Tratament TVA')).toBeInTheDocument()
    expect(screen.getByText('Drept de deducere TVA')).toBeInTheDocument()
    expect(screen.getByText(CLASSIFICATION_DIMENSION_LABELS.EXPENSE_TAX_TREATMENT)).toBeInTheDocument()
    expect(screen.getByText(/calculată de Diana/)).toBeInTheDocument()
    expect(screen.queryByText('Instrucțiune SAGA istorică')).not.toBeInTheDocument()
  })
  it('retains legacy rate and SAGA instruction labels', () => {
    const invoice = fixture(); invoice.modelVersion = 'LEGACY_V1'
    render(<MemoryRouter><ClassificationWorkspace invoice={invoice} /></MemoryRouter>)
    expect(screen.getByText(CLASSIFICATION_DIMENSION_LABELS.VAT)).toBeInTheDocument()
    expect(screen.getByText('Instrucțiune SAGA istorică')).toBeInTheDocument()
    expect(screen.queryByText('Drept de deducere TVA')).not.toBeInTheDocument()
  })
  it('requires a reason and sends exact limited percentage plus basis', async () => {
    const user = userEvent.setup(); const submit = vi.fn()
    render(<DomainCorrectionDialog item={item('VAT_DEDUCTIBILITY')} invoice={fixture()} onCancel={() => undefined} onSubmit={submit} />)
    await user.selectOptions(screen.getByLabelText('Decizie', { exact: true }), 'LIMITED')
    await user.click(screen.getByRole('button', { name: 'Salvează decizia' }))
    expect(screen.getByRole('alert')).toHaveTextContent('Completează motivul')
    await user.type(screen.getByLabelText('Motiv / documente justificative'), 'TEST_ONLY utilization evidence')
    await user.type(screen.getByLabelText('Procent deductibil (%) — separator punct'), '50.1250')
    await user.type(screen.getByLabelText('Baza legală / utilizarea justificată'), 'TEST_ONLY basis')
    await user.click(screen.getByRole('button', { name: 'Salvează decizia' }))
    expect(submit).toHaveBeenCalledWith({ kind: 'LIMITED', percentage: '50.1250', basis: 'TEST_ONLY basis' }, 'TEST_ONLY utilization evidence')
  })
  it('records period category without calculating a ceiling', async () => {
    const user = userEvent.setup(); const submit = vi.fn<(value: DomainValue, reason: string) => void>()
    render(<DomainCorrectionDialog item={item('EXPENSE_TAX_TREATMENT')} invoice={fixture()} onCancel={() => undefined} onSubmit={submit} />)
    await user.selectOptions(screen.getByLabelText('Decizie', { exact: true }), 'PERIOD_LIMIT_CATEGORY')
    await user.type(screen.getByLabelText('Categoria plafonului fiscal'), 'TEST_ONLY_PROTOCOL')
    await user.type(screen.getByLabelText('Baza plafonului'), 'TEST_ONLY article')
    await user.type(screen.getByLabelText('Motiv / documente justificative'), 'TEST_ONLY review')
    await user.click(screen.getByRole('button', { name: 'Salvează decizia' }))
    expect(submit).toHaveBeenCalledWith({ kind: 'PERIOD_LIMIT_CATEGORY', category: 'TEST_ONLY_PROTOCOL', basis: 'TEST_ONLY article' }, 'TEST_ONLY review')
    expect(screen.getByText('Diana nu calculează plafonul perioadei.')).toBeInTheDocument()
  })
  it('cannot invent missing source rate through fiscal correction', async () => {
    const user = userEvent.setup(); const submit = vi.fn(); const invoice = fixture(); delete invoice.lines[0].sourceFacts!.rate
    render(<DomainCorrectionDialog item={item('VAT_TREATMENT')} invoice={invoice} onCancel={() => undefined} onSubmit={submit} />)
    await user.type(screen.getByLabelText('Motiv / documente justificative'), 'TEST_ONLY review')
    await user.click(screen.getByRole('button', { name: 'Salvează decizia' }))
    expect(submit).not.toHaveBeenCalled()
    expect(screen.getByRole('alert')).toHaveTextContent('Sursa nu conține cota/categoria')
  })
})
