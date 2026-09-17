import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MockInvoiceRepository } from '../../mocks/MockInvoiceRepository'
import { renderApp } from '../../test/render-app'
import type { Invoice } from '../../domain/invoice'

class SagaUXRepository extends MockInvoiceRepository {
  confirmationCalls = 0
  downloadCalls = 0
  protected invoice: Invoice

  constructor() {
    super()
    this.invoice = {
      id: 'invoice-saga', scenario: 'PROCESSING', primaryDemo: false, clientId: 'client-alfa',
      supplierName: 'Furnizor SAGA', documentNumber: 'SAGA-1', issueDate: '2026-09-14T09:00:00Z',
      total: { amount: 100, currency: 'RON' }, spvReference: 'SPV-SAGA-1', pipelineStatus: 'EXPORTING',
      pipelinePath: ['DOWNLOADED', 'ARCHIVED', 'MATCHING', 'DEDUPE_CHECKED', 'HEADER_READ', 'LINES_READ', 'CLASSIFIED', 'READY_FOR_SAGA', 'EXPORTING', 'EXPORTED'],
      sagaStatus: 'EXPORTING', autoRun: false, lines: [], activity: [], revision: 3, authority: 'MOCK',
      sagaExport: { attemptId: 'attempt-saga', artifactStatus: 'GENERATED', filename: 'factura-saga.xml', generatedAt: '2026-09-14T10:00:00Z', invoiceRevision: 3 },
    }
  }

  override async getInvoice(id: string) { return id === this.invoice.id ? structuredClone(this.invoice) : super.getInvoice(id) }
  override async downloadSagaArtifact() { this.downloadCalls++; return { blob: new Blob(['<Facturi/>'], { type: 'application/xml' }), filename: 'factura-saga.xml' } }
  override async confirmSagaImport() {
    this.confirmationCalls++
    this.invoice.pipelineStatus = 'EXPORTED'; this.invoice.sagaStatus = 'EXPORTED'; this.invoice.revision = 4
    this.invoice.sagaExport!.confirmedAt = '2026-09-14T11:00:00Z'; this.invoice.sagaExport!.confirmedBy = 'Contabil demo'; this.invoice.sagaExport!.confirmationType = 'HUMAN'
    return structuredClone(this.invoice)
  }

  markExported() { this.invoice.pipelineStatus = 'EXPORTED'; this.invoice.sagaStatus = 'EXPORTED'; this.invoice.sagaExport!.confirmedAt = '2026-09-14T11:00:00Z'; this.invoice.sagaExport!.confirmedBy = 'Contabil demo'; this.invoice.sagaExport!.confirmationType = 'HUMAN' }
  markFailed() { this.invoice.sagaStatus = 'FAILED'; this.invoice.sagaExport!.artifactStatus = 'FAILED' }
}

class SagaErrorRepository extends SagaUXRepository { override async confirmSagaImport(): Promise<Invoice> { throw new Error('raw backend detail') } }
class LoadingRepository extends MockInvoiceRepository { override async getInvoice(): Promise<Invoice | undefined> { return new Promise(() => undefined) } }

describe('SAGA export manual handoff', () => {
  it('shows the three-step journey and requires explicit human confirmation', async () => {
    const user = userEvent.setup(); const repository = new SagaUXRepository()
    renderApp(repository, '/invoices/invoice-saga')

    expect(await screen.findByText(/Descărcarea singură nu marchează factura ca importată\./)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Descarcă XML' })).toBeEnabled()
    await user.click(screen.getByRole('button', { name: 'Marchează ca importat în SAGA' }))
    expect(screen.getByRole('dialog')).toHaveTextContent('Diana nu poate verifica automat importul în SAGA.')
    await user.click(screen.getByRole('button', { name: 'Confirmă importul' }))

    await waitFor(() => expect(repository.confirmationCalls).toBe(1))
    expect(await screen.findByText(/Diana nu a primit o confirmare tehnică de la SAGA\./)).toBeInTheDocument()
  })

  it('downloads through the repository without equating download with export', async () => {
    const user = userEvent.setup(); const repository = new SagaUXRepository()
    const createObjectURL = vi.fn(() => 'blob:test'); const revokeObjectURL = vi.fn()
	const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => undefined)
    Object.defineProperty(URL, 'createObjectURL', { configurable: true, value: createObjectURL })
    Object.defineProperty(URL, 'revokeObjectURL', { configurable: true, value: revokeObjectURL })
    renderApp(repository, '/invoices/invoice-saga')
    await user.click(await screen.findByRole('button', { name: 'Descarcă XML' }))
    await waitFor(() => expect(repository.downloadCalls).toBe(1))
    expect((await repository.getInvoice('invoice-saga'))?.pipelineStatus).toBe('EXPORTING')
    expect(createObjectURL).toHaveBeenCalled(); expect(revokeObjectURL).toHaveBeenCalledWith('blob:test')
	click.mockRestore()
  })

  it('shows a safe error and preserves the confirmation action', async () => {
    const user = userEvent.setup(); renderApp(new SagaErrorRepository(), '/invoices/invoice-saga')
    await user.click(await screen.findByRole('button', { name: 'Marchează ca importat în SAGA' }))
    await user.click(screen.getByRole('button', { name: 'Confirmă importul' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('Operația SAGA nu a putut fi finalizată.')
  })

  it('renders immutable exported and failed states without false technical claims', async () => {
    const exported = new SagaUXRepository(); exported.markExported(); renderApp(exported, '/invoices/invoice-saga')
    expect(await screen.findByText(/Diana nu a primit o confirmare tehnică de la SAGA\./)).toBeInTheDocument()
  })

  it('renders generation failure without enabling confirmation', async () => {
    const failed = new SagaUXRepository(); failed.markFailed(); renderApp(failed, '/invoices/invoice-saga')
    expect(await screen.findByText('Generare eșuată')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Marchează ca importat în SAGA' })).not.toBeInTheDocument()
  })

  it('keeps the invoice loading state explicit', () => {
    renderApp(new LoadingRepository(), '/invoices/invoice-saga')
    expect(screen.getByLabelText('Se încarcă factura')).toBeInTheDocument()
  })
})
