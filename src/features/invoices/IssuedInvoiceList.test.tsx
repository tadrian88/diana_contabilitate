import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { Invoice } from '../../domain/invoice'
import { MockInvoiceRepository } from '../../mocks/MockInvoiceRepository'
import { mockInvoices } from '../../mocks/scenarios'
import { renderApp } from '../../test/render-app'

// One issued invoice added only for this test, so the shared demo scenarios
// (and every count built on them) stay unchanged.
class RepositoryWithIssuedInvoice extends MockInvoiceRepository {
  constructor() {
    super()
    const source = mockInvoices.find((invoice) => invoice.pipelineStatus === 'EXPORTED') ?? mockInvoices[0]
    const issued: Invoice = { ...structuredClone(source), id: 'inv-issued-demo', documentNumber: 'VE-DEMO-001', task: undefined, direction: 'OUTGOING', supplierName: 'Client Demo Alfa SRL', customerName: 'Persoană Fizică Demo', customerIdentifier: '180***', customerIdentifierKind: 'CNP' }
    this['invoices'].set(issued.id, issued)
  }
}

describe('Invoice list tabs (D-131)', () => {
  it('keeps received invoices under „Primite” and lists issued ones under „Emise”', async () => {
    const user = userEvent.setup()
    renderApp(new RepositoryWithIssuedInvoice(), '/invoices')

    // The counts appear once the list has loaded.
    expect(await screen.findByRole('tab', { name: /Emise \(1\)/ })).toHaveAttribute('aria-selected', 'false')
    expect(screen.getByRole('tab', { name: /Primite \([1-9]/ })).toHaveAttribute('aria-selected', 'true')
    expect(screen.queryByText('VE-DEMO-001')).not.toBeInTheDocument()
    expect(screen.getByLabelText('Caută după furnizor sau număr factură')).toBeInTheDocument()

    await user.click(screen.getByRole('tab', { name: /Emise/ }))
    expect(await screen.findByText('VE-DEMO-001')).toBeInTheDocument()
    expect(screen.getByText('Persoană Fizică Demo')).toBeInTheDocument()
    expect(screen.getByText('180***')).toBeInTheDocument()
    expect(screen.getByRole('columnheader', { name: /Emitent/ })).toBeInTheDocument()
    expect(screen.getByLabelText('Caută după client sau număr factură')).toBeInTheDocument()
    expect(screen.getByText('din 1 facturi în context')).toBeInTheDocument()
  })
})
