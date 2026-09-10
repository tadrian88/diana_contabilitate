import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ClassificationRule, ClientScope } from '../../domain/invoice'
import { MockInvoiceRepository } from '../../mocks/MockInvoiceRepository'
import { renderApp } from '../../test/render-app'

describe('Rules List', () => {
  it('renders approved fields, three categories and explicit global/override scope', async () => {
    renderApp(new MockInvoiceRepository(), '/rules')
    for (const heading of ['Nume regulă', 'Categorie', 'Scope', 'Client', 'Versiune', 'Efectiv de la', 'Efectiv până la', 'Rezultat', 'Bază legală', 'Ultima actualizare']) expect(await screen.findByRole('columnheader', { name: heading })).toBeInTheDocument()
    expect(screen.getAllByText('Cont contabil').length).toBeGreaterThan(0)
    expect(screen.getAllByText('Cotă TVA').length).toBeGreaterThan(0)
    expect(screen.getAllByText('Deductibilitate').length).toBeGreaterThan(0)
    expect(screen.getAllByText('Regulă globală').length).toBeGreaterThan(0)
    expect(screen.getAllByText('Override client').length).toBeGreaterThan(0)
    expect(screen.getByText('Client Demo Beta SRL', { selector: 'td' })).toBeInTheDocument()
    expect(screen.getByTestId('active-scope')).toHaveTextContent('Toți clienții')
  })

  it('shows global baseline plus only the selected client overrides', async () => {
    const user = userEvent.setup()
    renderApp(new MockInvoiceRepository(), '/rules')
    await user.click(await screen.findByLabelText('Selectează clientul'))
    await user.click(screen.getByRole('option', { name: 'Client Demo Alfa SRL' }))
    await waitFor(() => expect(screen.queryByText('REG-DEMO-TVA-01-OVR-BETA')).not.toBeInTheDocument())
    expect(screen.getByText('REG-DEMO-CONT-01')).toBeInTheDocument()
    expect(screen.getByText('REG-DEMO-TVA-01')).toBeInTheDocument()

    await user.selectOptions(screen.getByLabelText('Domeniu regulă'), 'CLIENT_OVERRIDE')
    expect(await screen.findByText('Nu există override-uri pentru selecția curentă')).toBeInTheDocument()
  })

  it('supports category, scope, client, version and text filtering', async () => {
    const user = userEvent.setup()
    renderApp(new MockInvoiceRepository(), '/rules')
    await user.selectOptions(await screen.findByLabelText('Categorie'), 'VAT')
    expect(screen.getByText('REG-DEMO-TVA-01')).toBeInTheDocument()
    expect(screen.queryByText('REG-DEMO-CONT-01')).not.toBeInTheDocument()
    await user.selectOptions(screen.getByLabelText('Client pentru override'), 'client-beta')
    expect(screen.getByText('REG-DEMO-TVA-01-OVR-BETA')).toBeInTheDocument()
    await user.selectOptions(screen.getByLabelText('Versiuni'), 'HISTORICAL')
    expect(screen.getByText('Niciun rezultat')).toBeInTheDocument()
  })

  it('searches criteria and exposes historical versions', async () => {
    const user = userEvent.setup()
    renderApp(new MockInvoiceRepository(), '/rules?version=HISTORICAL')
    expect(await screen.findByText('Versiunea 1 · Istorică')).toBeInTheDocument()
    const search = screen.getByLabelText('Caută reguli')
    await user.type(search, 'servicii generale')
    expect(screen.getByText('REG-DEMO-CONT-01')).toBeInTheDocument()
  })
})

describe('Rule Detail and mutations', () => {
  it('shows current rule content, legal disclaimer and readable history', async () => {
    renderApp(new MockInvoiceRepository(), '/rules/rule-account-global')
    expect(await screen.findByRole('heading', { name: 'Încadrare cont pentru servicii demonstrative' })).toBeInTheDocument()
    expect(screen.getAllByText('Versiunea 2 · Curentă').length).toBeGreaterThan(0)
    expect(screen.getByText('Versiunea 1 · Istorică')).toBeInTheDocument()
    expect(screen.getAllByText('Exemplu demonstrativ — bază legală nevalidată').length).toBeGreaterThan(0)
    expect(screen.getByText(/Schimbare față de versiunea 1/)).toBeInTheDocument()
  })

  it('keeps a historical version read-only', async () => {
    renderApp(new MockInvoiceRepository(), '/rules/rule-account-global?version=1')
    expect(await screen.findByText('Versiune istorică read-only.', { exact: false })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Creează versiune nouă' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Creează override pentru client' })).not.toBeInTheDocument()
  })

  it('creates a new version while preserving previous versions and scope', async () => {
    const user = userEvent.setup()
    const repository = new MockInvoiceRepository()
    const before = await repository.getRule('rule-account-global')
    renderApp(repository, '/rules/rule-account-global')
    await user.click(await screen.findByRole('button', { name: 'Creează versiune nouă' }))
    await user.clear(screen.getByLabelText('Criteriu / pattern'))
    await user.type(screen.getByLabelText('Criteriu / pattern'), 'Criteriu demonstrativ pentru versiunea trei')
    await user.clear(screen.getByLabelText('Rezultat clasificare'))
    await user.type(screen.getByLabelText('Rezultat clasificare'), 'Cont demonstrativ 6YY')
    await user.click(screen.getByRole('button', { name: 'Salvează versiunea nouă' }))

    await waitFor(() => expect(screen.getAllByText('Versiunea 3 · Curentă').length).toBeGreaterThan(0))
    const after = await repository.getRule('rule-account-global')
    expect(after?.scope).toBe('GLOBAL')
    expect(after?.versions).toHaveLength(3)
    expect(after?.versions[1]).toEqual(before?.versions[1])
    expect(screen.getByText('Versiunea 2 · Istorică')).toBeInTheDocument()
  })

  it('creates an Alfa override and leaves its global parent unchanged', async () => {
    const user = userEvent.setup()
    const repository = new MockInvoiceRepository()
    const globalBefore = await repository.getRule('rule-vat-global')
    renderApp(repository, '/rules/rule-vat-global')
    await user.click(await screen.findByRole('button', { name: 'Creează override pentru client' }))
    await user.selectOptions(screen.getByLabelText('Client', { exact: true }), 'client-alfa')
    await user.clear(screen.getByLabelText('Criteriu / pattern'))
    await user.type(screen.getByLabelText('Criteriu / pattern'), 'Criteriu TVA fictiv pentru Alfa')
    await user.clear(screen.getByLabelText('Rezultat clasificare'))
    await user.type(screen.getByLabelText('Rezultat clasificare'), 'Rezultat TVA demonstrativ Alfa')
    await user.click(screen.getByRole('button', { name: 'Salvează override-ul' }))

    expect(await screen.findByText('REG-DEMO-TVA-01-OVR-ALFA-1')).toBeInTheDocument()
    expect(await repository.getRule('rule-vat-global')).toEqual(globalBefore)
    await user.click(screen.getByLabelText('Selectează clientul'))
    await user.click(screen.getByRole('option', { name: 'Client Demo Alfa SRL' }))
    expect(await screen.findByText('REG-DEMO-TVA-01-OVR-ALFA-1')).toBeInTheDocument()
    expect(screen.getByText('REG-DEMO-TVA-01')).toBeInTheDocument()
    await user.click(screen.getByLabelText('Selectează clientul'))
    await user.click(screen.getByRole('option', { name: 'Client Demo Beta SRL' }))
    await waitFor(() => expect(screen.queryByText('REG-DEMO-TVA-01-OVR-ALFA-1')).not.toBeInTheDocument())
    expect(screen.getByText('REG-DEMO-TVA-01')).toBeInTheDocument()
  })

  it('links classification origin to Rule Detail without changing invoice output', async () => {
    const user = userEvent.setup()
    const repository = new MockInvoiceRepository()
    const invoiceBefore = await repository.getInvoice('inv-resolved')
    renderApp(repository, '/invoices/inv-resolved?tab=classification')
    const ruleLink = (await screen.findAllByText(/Origine regulă: Regulă globală · REG-DEMO-CONT-01 · v2/))[0]
    await user.click(ruleLink)
    expect(await screen.findByRole('heading', { name: 'Încadrare cont pentru servicii demonstrative' })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Înapoi la clasificarea facturii' })).toBeInTheDocument()
    expect(await repository.getInvoice('inv-resolved')).toEqual(invoiceBefore)
  })

  it('shows rule, historical-version and repository completeness states', async () => {
    const { unmount } = renderApp(new MockInvoiceRepository(), '/rules/does-not-exist')
    expect(await screen.findByText('Regula nu a fost găsită')).toBeInTheDocument()
    unmount()
    renderApp(new MockInvoiceRepository(), '/rules/rule-account-global?version=99')
    expect(await screen.findByText('Versiunea istorică nu este disponibilă')).toBeInTheDocument()
  })

  it('renders no-rules and repository-error states', async () => {
    class EmptyRepository extends MockInvoiceRepository { override async listRules(_scope: ClientScope): Promise<ClassificationRule[]> { return [] } }
    const { unmount } = renderApp(new EmptyRepository(), '/rules')
    expect(await screen.findByText('Nu există reguli în acest context')).toBeInTheDocument()
    unmount()
    class ErrorRepository extends MockInvoiceRepository { override async listRules(_scope: ClientScope): Promise<ClassificationRule[]> { throw new Error('demo') } }
    renderApp(new ErrorRepository(), '/rules')
    expect(await screen.findByText('Regulile nu au putut fi încărcate')).toBeInTheDocument()
  })
})
