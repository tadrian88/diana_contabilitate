import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MockInvoiceRepository } from '../../mocks/MockInvoiceRepository'
import { renderApp } from '../../test/render-app'

describe('Contracts workspace', () => {
  it('renders approved fields and cross-client contracts', async () => {
    renderApp(new MockInvoiceRepository(), '/contracts')

    for (const heading of ['Referință contract', 'Client', 'Furnizor', 'Perioadă', 'Monedă', 'Valoare', 'Tip unitate / bază comercială', 'Termeni de plată', 'Sursă']) {
      expect(await screen.findByRole('columnheader', { name: heading })).toBeInTheDocument()
    }
    expect(screen.getByTestId('active-scope')).toHaveTextContent('Toți clienții')
    expect(screen.getAllByText('Client Demo Alfa SRL').length).toBeGreaterThan(0)
    expect(screen.getAllByText('Client Demo Beta SRL').length).toBeGreaterThan(0)
  })

  it('scopes contracts through the persistent client selector', async () => {
    const user = userEvent.setup()
    renderApp(new MockInvoiceRepository(), '/contracts')

    await user.click(await screen.findByLabelText('Selectează clientul'))
    await user.click(screen.getByRole('option', { name: 'Client Demo Alfa SRL' }))

    await waitFor(() => expect(screen.getByText('din 4 contracte în context')).toBeInTheDocument())
    expect(screen.getByText('CTR-DEMO-100')).toBeInTheDocument()
    expect(screen.queryByText('CTR-DEMO-401')).not.toBeInTheDocument()
  })

  it('searches by supplier or reference and supports the supplier filter', async () => {
    const user = userEvent.setup()
    renderApp(new MockInvoiceRepository(), '/contracts')
    const search = await screen.findByLabelText('Caută după referință sau furnizor')

    await user.type(search, 'CTR-DEMO-501')
    expect(within(screen.getByText('CTR-DEMO-501').closest('tr')!).getByText('Furnizor Demo Central SRL')).toBeInTheDocument()
    expect(screen.queryByText('CTR-DEMO-100')).not.toBeInTheDocument()
    await user.clear(search)
    await user.selectOptions(screen.getByLabelText('Filtrează după furnizor'), 'Furnizor Demo Vest SRL')
    expect(screen.getByText('CTR-DEMO-201')).toBeInTheDocument()
    expect(screen.getByText('CTR-DEMO-202')).toBeInTheDocument()
    expect(screen.queryByText('CTR-DEMO-501')).not.toBeInTheDocument()
  })

  it('renders approved contract detail fields and associated invoices navigate to Invoice Detail', async () => {
    const user = userEvent.setup()
    renderApp(new MockInvoiceRepository(), '/contracts/contract-demo-100')

    expect(await screen.findByRole('heading', { name: 'CTR-DEMO-100' })).toBeInTheDocument()
    for (const label of ['Perioadă efectivă', 'Valoare contractuală totală (legacy)', 'Tip unitate / bază comercială', 'Termeni de plată', 'Referință sursă', 'Metadate sursă']) expect(screen.getByText(label)).toBeInTheDocument()
    expect(screen.getByText('DEMO-HP-001')).toBeInTheDocument()
    expect(screen.getByText('DEMO-SH-011')).toBeInTheDocument()
    await user.click(screen.getByRole('link', { name: 'DEMO-SH-011' }))
    expect(await screen.findByRole('heading', { name: 'DEMO-SH-011' })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Înapoi la lista contractelor' })).toBeInTheDocument()
  })

  it('inspecting a multiple-match candidate preserves the unresolved task', async () => {
    const user = userEvent.setup()
    const repository = new MockInvoiceRepository()
    renderApp(repository, '/invoices/inv-multiple?tab=contract&returnTo=%2Ftasks')

    await user.click(await screen.findByRole('link', { name: 'Inspectează contractul' }))
    expect(await screen.findByText('Semnale demonstrative disponibile')).toBeInTheDocument()
    expect((await repository.getInvoice('inv-multiple'))?.task?.status).toBe('OPEN')
    await user.click(screen.getByRole('link', { name: 'Înapoi la revizuirea facturii' }))
    expect(await screen.findByText('Contract recomandat')).toBeInTheDocument()
    expect((await repository.getInvoice('inv-multiple'))?.task?.status).toBe('OPEN')
  })

  it('keeps missing-contract handling limited to the approved request action', async () => {
    renderApp(new MockInvoiceRepository(), '/invoices/inv-missing?tab=contract')
    expect(await screen.findByRole('button', { name: 'Solicită contract' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /creează|încarcă|atașează/i })).not.toBeInTheDocument()
  })

  it('reassignment updates invoice state and the contract-associated invoice view', async () => {
    const user = userEvent.setup()
    const repository = new MockInvoiceRepository()
    renderApp(repository, '/invoices/inv-multiple?tab=contract')

    await user.click(await screen.findByRole('button', { name: 'Alege alt contract' }))
    const alternative = screen.getByRole('radio')
    await user.click(alternative)
    await user.click(screen.getByRole('button', { name: 'Confirmă selecția' }))
    await waitFor(() => expect(screen.getByText('CTR-DEMO-202')).toBeInTheDocument())
    expect((await repository.getInvoice('inv-multiple'))?.selectedContractId).toBe('contract-demo-202')
    expect((await repository.getInvoice('inv-multiple'))?.task?.status).toBe('RESOLVED')

    await user.click(screen.getByRole('link', { name: 'Inspectează contractul CTR-DEMO-202' }))
    const associatedTable = (await screen.findByRole('heading', { name: 'Facturi asociate' })).closest('section')!
    expect(within(associatedTable).getByText('DEMO-MC-002')).toBeInTheDocument()
  })

  it('shows contract-not-found state', async () => {
    renderApp(new MockInvoiceRepository(), '/contracts/does-not-exist')
    expect(await screen.findByText('Contractul nu a fost găsit')).toBeInTheDocument()
  })

  it('requires confirmation and removes a contract from the active list while retaining its history', async () => {
    const user = userEvent.setup()
    const repository = new MockInvoiceRepository()
    renderApp(repository, '/contracts/contract-demo-100')
    await user.click(await screen.findByRole('button', { name: 'Scoate din utilizare' }))
    expect(await repository.listContracts('all')).toEqual(expect.arrayContaining([expect.objectContaining({ id: 'contract-demo-100' })]))
    await user.click(screen.getByRole('button', { name: 'Confirmă arhivarea' }))
    await waitFor(async () => expect((await repository.listContracts('all')).some(contract => contract.id === 'contract-demo-100')).toBe(false))
    expect((await repository.getContract('contract-demo-100'))?.lifecycleState).toBe('ARCHIVED')
    expect((await repository.listContractInvoices('contract-demo-100')).length).toBeGreaterThan(0)
  })

  it('separates mistaken upload removal from archiving a used contract', async () => {
    const user = userEvent.setup()
    const repository = new MockInvoiceRepository()
    renderApp(repository, '/contracts/contract-demo-501')
    await user.click(await screen.findByRole('button', { name: 'Șterge încărcarea greșită' }))
    await user.click(screen.getByRole('button', { name: 'Confirmă ștergerea' }))
    await waitFor(async () => expect((await repository.listContracts('all')).some(contract=>contract.id==='contract-demo-501')).toBe(false))
  })
})
