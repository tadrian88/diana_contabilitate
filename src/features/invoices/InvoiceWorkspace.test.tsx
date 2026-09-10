import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MockInvoiceRepository } from '../../mocks/MockInvoiceRepository'
import { renderApp } from '../../test/render-app'

const requiredColumns = [
  'Furnizor', 'Număr factură', 'Client', 'Valoare', 'Dată', 'Contract',
  'Status pipeline', 'Număr probleme', 'Încredere', 'Status SAGA',
]

describe('Invoice List', () => {
  it('renders the required columns and both clients in global scope', async () => {
    renderApp(new MockInvoiceRepository(), '/invoices')

    for (const column of requiredColumns) expect(await screen.findByRole('columnheader', { name: new RegExp(column) })).toBeInTheDocument()
    expect(screen.getByTestId('active-scope')).toHaveTextContent('Toți clienții')
    expect(screen.getAllByText('Client Demo Alfa SRL').length).toBeGreaterThan(0)
    expect(screen.getAllByText('Client Demo Beta SRL').length).toBeGreaterThan(0)
  })

  it('applies the persistent selected-client scope', async () => {
    const user = userEvent.setup()
    renderApp(new MockInvoiceRepository(), '/invoices')

    await user.click(await screen.findByLabelText('Selectează clientul'))
    await user.click(screen.getByRole('option', { name: 'Client Demo Alfa SRL' }))

    await waitFor(() => expect(screen.getByText('din 6 facturi în context')).toBeInTheDocument())
    expect(screen.queryByText('DEMO-NC-003')).not.toBeInTheDocument()
    expect(screen.getByText('DEMO-DP-009')).toBeInTheDocument()
  })

  it('searches by invoice or supplier', async () => {
    const user = userEvent.setup()
    renderApp(new MockInvoiceRepository(), '/invoices')
    const search = await screen.findByLabelText('Caută după furnizor sau număr factură')

    await user.type(search, 'DEMO-SF-010')
    expect(screen.getByText('Furnizor Demo Aurora SRL')).toBeInTheDocument()
    expect(screen.queryByText('DEMO-RS-007')).not.toBeInTheDocument()
  })

  it('filters by pipeline, attention and SAGA status', async () => {
    const user = userEvent.setup()
    renderApp(new MockInvoiceRepository(), '/invoices')

    await user.selectOptions(await screen.findByLabelText('Status pipeline'), 'DUPLICATE')
    expect(screen.getByText('DEMO-DP-009')).toBeInTheDocument()
    expect(screen.queryByText('DEMO-MC-002')).not.toBeInTheDocument()
    expect(within(screen.getByText('DEMO-DP-009').closest('tr')!).getByText('Duplicat')).toBeInTheDocument()

    await user.selectOptions(screen.getByLabelText('Status pipeline'), 'ALL')
    await user.selectOptions(screen.getByLabelText('Atenție'), 'REQUIRED')
    expect(screen.getByText('DEMO-MC-002')).toBeInTheDocument()
    expect(screen.getByText('DEMO-WT-006')).toBeInTheDocument()
    expect(screen.queryByText('DEMO-RS-007')).not.toBeInTheDocument()

    await user.selectOptions(screen.getByLabelText('Atenție'), 'ALL')
    await user.selectOptions(screen.getByLabelText('Status SAGA'), 'FAILED')
    expect(screen.getByText('DEMO-SF-010')).toBeInTheDocument()
    expect(screen.getAllByText('Export eșuat').length).toBeGreaterThan(0)
  })

  it('derives issue counts and updates the list after detail resolution', async () => {
    const user = userEvent.setup()
    renderApp(new MockInvoiceRepository(), '/invoices?q=DEMO-MC-002')

    const initialRow = (await screen.findByText('DEMO-MC-002')).closest('tr')!
    expect(within(initialRow).getByText('1')).toBeInTheDocument()
    await user.click(within(initialRow).getByRole('link', { name: 'DEMO-MC-002' }))
    await user.click(await screen.findByRole('tab', { name: 'Contract' }))
    await user.click(screen.getByRole('button', { name: 'Confirmă' }))
    await user.click(await screen.findByRole('link', { name: 'Înapoi la lista facturilor' }))

    const updatedRow = (await screen.findByText('DEMO-MC-002')).closest('tr')!
    await waitFor(() => expect(within(updatedRow).getByText('0')).toBeInTheDocument())
    expect(within(updatedRow).queryByText('Acțiune necesară')).not.toBeInTheDocument()
  })
})

describe('Invoice Detail workspace', () => {
  it('keeps all approved tabs and shows matched contract details', async () => {
    const user = userEvent.setup()
    renderApp(new MockInvoiceRepository(), '/invoices/inv-resolved')

    for (const tab of ['Rezumat', 'Contract', 'Linii factură', 'Clasificare', 'Istoric']) expect(await screen.findByRole('tab', { name: tab })).toBeInTheDocument()
    await user.click(screen.getByRole('tab', { name: 'Contract' }))
    expect(screen.getByText('CTR-DEMO-701')).toBeInTheDocument()
    expect(screen.getByText('Tip unitate')).toBeInTheDocument()
    expect(screen.getByText('Termen plată')).toBeInTheDocument()
  })

  it('represents missing contracts and partial source data without invention', async () => {
    const user = userEvent.setup()
    renderApp(new MockInvoiceRepository(), '/invoices/inv-missing?tab=contract')
    expect(await screen.findByRole('button', { name: 'Solicită contract' })).toBeInTheDocument()

    await user.click(screen.getByRole('tab', { name: 'Linii factură' }))
    expect(screen.getByText('Liniile facturii nu sunt disponibile')).toBeInTheDocument()
  })

  it('shows complete invoice lines, all classification dimensions and legal disclaimer', async () => {
    const user = userEvent.setup()
    renderApp(new MockInvoiceRepository(), '/invoices/inv-classification?tab=lines')

    for (const heading of ['Denumire articol/serviciu', 'UM', 'TVA', 'Cantitate', 'Preț unitar', 'Valoare', 'TVA valoare', 'Total', 'Informații suplimentare']) expect(await screen.findByRole('columnheader', { name: heading })).toBeInTheDocument()
    await user.click(screen.getByRole('tab', { name: 'Clasificare' }))
    const classificationOverview = screen.getByRole('heading', { name: 'Clasificări pe linii' }).closest('section')!
    expect(within(classificationOverview).getAllByText('Cont contabil')).toHaveLength(2)
    expect(within(classificationOverview).getAllByText('Cotă TVA')).toHaveLength(2)
    expect(within(classificationOverview).getAllByText('Deductibilitate')).toHaveLength(2)
    expect(within(classificationOverview).getAllByText('Exemplu demonstrativ — bază legală nevalidată')).toHaveLength(6)
  })

  it('shows deterministic audit events with before/after and duplicate terminal explanation', async () => {
    const user = userEvent.setup()
    renderApp(new MockInvoiceRepository(), '/invoices/inv-duplicate?tab=history')

    expect(await screen.findByText('Duplicat identificat')).toBeInTheDocument()
    expect(screen.getAllByText('Sistem demo', { exact: false }).length).toBeGreaterThan(0)
    expect(screen.getByText('DEDUPE_CHECKED')).toBeInTheDocument()
    expect(screen.getByText('DUPLICATE')).toBeInTheDocument()
    expect(screen.getAllByText(/nu continuă către SAGA/).length).toBeGreaterThan(0)

    await user.click(screen.getByRole('tab', { name: 'Rezumat' }))
    expect(screen.getByText('Stare terminală. Factura nu continuă către SAGA.')).toBeInTheDocument()
  })

  it('shows invoice-not-found state', async () => {
    renderApp(new MockInvoiceRepository(), '/invoices/does-not-exist')
    expect(await screen.findByText('Factura nu a fost găsită')).toBeInTheDocument()
  })
})
