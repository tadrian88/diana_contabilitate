import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ClientScope, Invoice } from '../../domain/invoice'
import { MockInvoiceRepository } from '../../mocks/MockInvoiceRepository'
import { mockClients, mockInvoices } from '../../mocks/scenarios'
import { renderApp } from '../../test/render-app'
import { selectDashboard } from './dashboard-selectors'

function kpi(label: string) {
  return document.querySelector(`[data-kpi="${label}"]`) as HTMLElement
}

describe('Dashboard selectors', () => {
  it('uses deterministic current-month definitions and excludes waiting from immediate attention', () => {
    const outsideMonth: Invoice = { ...structuredClone(mockInvoices[0]), id: 'outside-month', issueDate: '2026-08-31T23:00:00.000Z' }
    const dashboard = selectDashboard([...mockInvoices, outsideMonth], mockClients)

    expect(dashboard.invoices).toHaveLength(11)
    expect(dashboard.kpis).toEqual({ processing: 8, attention: 4, ready: 0, exported: 2 })
    expect(dashboard.waitingInvoices).toHaveLength(1)
    expect(dashboard.openInvoices).toHaveLength(4)
    expect(dashboard.failedInvoices).toHaveLength(1)
    expect(dashboard.pipelineCounts.DUPLICATE).toBe(1)
    expect(dashboard.openInvoices.some((invoice) => invoice.pipelineStatus === 'DUPLICATE')).toBe(false)
    expect(dashboard.failedInvoices[0].task).toBeUndefined()
  })

  it('derives READY_FOR_SAGA independently from exported invoices', () => {
    const readyInvoices = structuredClone(mockInvoices)
    const readyInvoice = readyInvoices.find((invoice) => invoice.id === 'inv-happy')!
    readyInvoice.pipelineStatus = 'READY_FOR_SAGA'
    readyInvoice.sagaStatus = 'READY'
    const dashboard = selectDashboard(readyInvoices, mockClients)

    expect(dashboard.kpis.ready).toBe(1)
    expect(dashboard.kpis.exported).toBe(2)
    expect(dashboard.kpis.processing).toBe(8)
  })

  it('does not count a generated or downloaded artifact as exported', () => {
    const invoices = structuredClone(mockInvoices)
    const generated = invoices.find((invoice) => invoice.id === 'inv-happy')!
    generated.pipelineStatus = 'EXPORTING'; generated.sagaStatus = 'EXPORTING'
    generated.sagaExport = { attemptId: 'attempt-generated', artifactStatus: 'GENERATED', filename: 'generated.xml', generatedAt: '2026-09-14T10:00:00Z', downloadedAt: '2026-09-14T10:05:00Z', invoiceRevision: 3 }

    expect(selectDashboard(invoices, mockClients).kpis.exported).toBe(2)
  })
})

describe('Complete Dashboard', () => {
  it('renders exactly the four approved KPI concepts with derived values and deep links', async () => {
    renderApp(new MockInvoiceRepository(), '/')
    expect(await screen.findByText('Situația operațională de azi')).toBeInTheDocument()

    expect(document.querySelectorAll('[data-kpi]')).toHaveLength(4)
    expect(within(kpi('Facturi în procesare')).getByText('8')).toBeInTheDocument()
    expect(within(kpi('Necesită atenție')).getByText('4')).toBeInTheDocument()
    expect(within(kpi('Pregătite pentru SAGA')).getByText('0')).toBeInTheDocument()
    expect(within(kpi('Exportate în SAGA')).getByText('2')).toBeInTheDocument()
    expect(kpi('Necesită atenție')).toHaveAttribute('href', '/tasks?status=OPEN&type=ALL')
    expect(kpi('Pregătite pentru SAGA')).toHaveAttribute('href', '/invoices?pipeline=READY_FOR_SAGA')
  })

  it('shows the approved pipeline, SAGA failure, SPV context and duplicate without creating a task', async () => {
    renderApp(new MockInvoiceRepository(), '/')
    expect(await screen.findByRole('heading', { name: 'Ce se întâmplă în pipeline' })).toBeInTheDocument()
    for (const label of ['Descărcată din SPV', 'Așteaptă contract', 'Linii citite', 'Pregătită pentru SAGA', 'Exportată în SAGA', 'Duplicat']) expect(screen.getAllByText(label).length).toBeGreaterThan(0)
    expect(screen.getByText('DEMO-SF-010')).toBeInTheDocument()
    expect(screen.getByText('1 eșuate')).toBeInTheDocument()
    expect(screen.getByText(/facturi cu referință SPV demonstrativă/)).toBeInTheDocument()
    expect(screen.queryByText('DEMO-DP-009')).not.toBeInTheDocument()
  })

  it('separates OPEN actions from WAITING external conditions and deep-links to the right tabs', async () => {
    renderApp(new MockInvoiceRepository(), '/')
    const classification = await screen.findByText('DEMO-CL-004')
    expect(classification.closest('a')).toHaveAttribute('href', '/invoices/inv-classification?tab=classification&returnTo=%2F')
    const contract = screen.getByText('DEMO-MC-002')
    expect(contract.closest('a')).toHaveAttribute('href', '/invoices/inv-multiple?tab=contract&returnTo=%2F')
    expect(screen.getByText('DEMO-WT-006')).toBeInTheDocument()
    expect(within(kpi('Necesită atenție')).getByText('4')).toBeInTheDocument()
  })

  it('reacts to client scope and allows selecting a client from the overview', async () => {
    const user = userEvent.setup()
    renderApp(new MockInvoiceRepository(), '/')
    await user.click(await screen.findByRole('button', { name: 'Client Demo Alfa SRL' }))

    await waitFor(() => expect(screen.getByTestId('active-scope')).toHaveTextContent('Client Demo Alfa SRL'))
    await waitFor(() => expect(within(kpi('Necesită atenție')).getByText('1')).toBeInTheDocument())
    expect(within(kpi('Facturi în procesare')).getByText('4')).toBeInTheDocument()
    expect(screen.queryByRole('heading', { name: 'Context pe clienți' })).not.toBeInTheDocument()
    expect(screen.queryByText('DEMO-CL-004')).not.toBeInTheDocument()
  })

  it('updates attention immediately after resolving a contract task and returning', async () => {
    const user = userEvent.setup()
    renderApp(new MockInvoiceRepository(), '/')
    await user.click(await screen.findByText('DEMO-MC-002'))
    await user.click(screen.getByRole('button', { name: 'Confirmă' }))
    await user.click(await screen.findByRole('link', { name: 'Înapoi la Dashboard' }))

    await waitFor(() => expect(within(kpi('Necesită atenție')).getByText('3')).toBeInTheDocument())
    expect(screen.queryByText('DEMO-MC-002')).not.toBeInTheDocument()
  })

  it('continues automatic export globally and refreshes exported KPI', async () => {
    const user = userEvent.setup()
    renderApp(new MockInvoiceRepository(), '/')
    await user.click(await screen.findByText('DEMO-CL-004'))
    await user.click(screen.getAllByRole('button', { name: 'Acceptă propunerea' })[0])
    await waitFor(() => expect(screen.getAllByRole('button', { name: 'Acceptă propunerea' })).toHaveLength(1))
    await user.click(screen.getByRole('button', { name: 'Acceptă propunerea' }))
    await user.click(await screen.findByRole('link', { name: 'Înapoi la Dashboard' }))

    await waitFor(() => expect(within(kpi('Exportate în SAGA')).getByText('3')).toBeInTheDocument(), { timeout: 4000 })
    expect(screen.queryByText('DEMO-CL-004')).not.toBeInTheDocument()
  })

  it('renders repository error and zero-invoice states', async () => {
    class ErrorRepository extends MockInvoiceRepository {
      override async listInvoices(_scope: ClientScope): Promise<Invoice[]> { throw new Error('demo') }
    }
    const { unmount } = renderApp(new ErrorRepository(), '/')
    expect(await screen.findByText('Dashboard-ul nu a putut fi încărcat')).toBeInTheDocument()
    unmount()

    class EmptyRepository extends MockInvoiceRepository {
      override async listInvoices(_scope: ClientScope): Promise<Invoice[]> { return [] }
    }
    renderApp(new EmptyRepository(), '/')
    expect(await screen.findByText('Nu există facturi în luna curentă')).toBeInTheDocument()
  })
})
