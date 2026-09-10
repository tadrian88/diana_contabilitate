import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { Client } from '../../domain/invoice'
import { MockInvoiceRepository } from '../../mocks/MockInvoiceRepository'
import { renderApp } from '../../test/render-app'

describe('Clients workspace', () => {
  it('lists all clients with operational counts derived from shared state', async () => {
    renderApp(new MockInvoiceRepository(), '/clients')
    const alfa = (await screen.findByText('Client Demo Alfa SRL')).closest('tr')!
    const beta = screen.getByText('Client Demo Beta SRL').closest('tr')!
    expect(within(alfa).getByText('RO-DEMO-ALFA-001')).toBeInTheDocument()
    expect(within(alfa).getAllByText('4').length).toBeGreaterThan(0)
    expect(within(alfa).getAllByText('1').length).toBeGreaterThan(0)
    expect(within(beta).getByText('RO-DEMO-BETA-002')).toBeInTheDocument()
    expect(screen.getByTestId('active-scope')).toHaveTextContent('Toți clienții')
  })

  it('shows a concise client detail with every approved operational section', async () => {
    renderApp(new MockInvoiceRepository(), '/clients/client-beta')
    expect(await screen.findByRole('heading', { name: 'Client Demo Beta SRL' })).toBeInTheDocument()
    for (const heading of ['Facturi', 'Task-uri deschise', 'În așteptare', 'Contracte', 'Override-uri client']) expect(screen.getByRole('heading', { name: heading })).toBeInTheDocument()
    for (const saga of ['Pregătite pentru SAGA', 'Export în curs', 'Exportate în SAGA', 'Export eșuat']) expect(screen.getAllByText(saga).length).toBeGreaterThan(0)
    expect(screen.getByText('REG-DEMO-TVA-01-OVR-BETA')).toBeInTheDocument()
    await waitFor(() => expect(screen.getByTestId('active-scope')).toHaveTextContent('Client Demo Beta SRL'))
  })

  it('navigates from client context into existing scoped modules', async () => {
    const user = userEvent.setup()
    renderApp(new MockInvoiceRepository(), '/clients/client-alfa')
    await user.click(await screen.findByRole('link', { name: /^Facturi/ }))
    expect(await screen.findByRole('heading', { name: 'Facturi', level: 2 })).toBeInTheDocument()
    expect(screen.getByText('din 6 facturi în context')).toBeInTheDocument()
    expect(screen.queryByText('DEMO-NC-003')).not.toBeInTheDocument()
  })

  it('limits the Clients List to the selected client context', async () => {
    const user = userEvent.setup()
    renderApp(new MockInvoiceRepository(), '/clients')
    await user.click(await screen.findByLabelText('Selectează clientul'))
    await user.click(screen.getByRole('option', { name: 'Client Demo Alfa SRL' }))
    await waitFor(() => expect(screen.queryByText('Client Demo Beta SRL')).not.toBeInTheDocument())
    expect(screen.getAllByText('Client Demo Alfa SRL').length).toBeGreaterThan(0)
  })

  it('moves away from a stale invoice detail when switching to another client', async () => {
    const user = userEvent.setup()
    renderApp(new MockInvoiceRepository(), '/invoices/inv-happy')
    expect(await screen.findByRole('heading', { name: 'DEMO-HP-001' })).toBeInTheDocument()
    await user.click(screen.getByLabelText('Selectează clientul'))
    await user.click(screen.getByRole('option', { name: 'Client Demo Beta SRL' }))
    expect(await screen.findByRole('heading', { name: 'Facturi', level: 2 })).toBeInTheDocument()
    expect(screen.queryByText('DEMO-HP-001')).not.toBeInTheDocument()
    expect(screen.getByTestId('active-scope')).toHaveTextContent('Client Demo Beta SRL')
  })

  it('protects contract and client-override details from cross-client context', async () => {
    const user = userEvent.setup()
    const first = renderApp(new MockInvoiceRepository(), '/contracts/contract-demo-401')
    expect(await screen.findByRole('heading', { name: 'CTR-DEMO-401' })).toBeInTheDocument()
    await user.click(screen.getByLabelText('Selectează clientul'))
    await user.click(screen.getByRole('option', { name: 'Client Demo Alfa SRL' }))
    expect(await screen.findByRole('heading', { name: 'Contracte', level: 2 })).toBeInTheDocument()
    first.unmount()

    renderApp(new MockInvoiceRepository(), '/rules/rule-vat-beta-override')
    expect(await screen.findByRole('heading', { name: 'Variație TVA pentru Client Demo Beta' })).toBeInTheDocument()
    await user.click(screen.getByLabelText('Selectează clientul'))
    await user.click(screen.getByRole('option', { name: 'Client Demo Alfa SRL' }))
    expect(await screen.findByRole('heading', { name: 'Reguli de clasificare' })).toBeInTheDocument()
  })

  it('renders client not-found, empty and repository-error states', async () => {
    const first = renderApp(new MockInvoiceRepository(), '/clients/unknown-client')
    expect(await screen.findByText('Clientul nu a fost găsit')).toBeInTheDocument()
    first.unmount()
    class EmptyRepository extends MockInvoiceRepository { override async listClients(): Promise<Client[]> { return [] } }
    const second = renderApp(new EmptyRepository(), '/clients')
    expect(await screen.findByText('Nu există clienți în contextul activ')).toBeInTheDocument()
    second.unmount()
    class ErrorRepository extends MockInvoiceRepository { override async listClients(): Promise<Client[]> { throw new Error('demo') } }
    renderApp(new ErrorRepository(), '/clients')
    expect(await screen.findByText('Clienții nu au putut fi încărcați')).toBeInTheDocument()
  })
})

describe('Mock data integrity', () => {
  it('keeps clients, contracts, tasks and rule references coherent', async () => {
    const repository = new MockInvoiceRepository()
    const [clients, invoices, contracts, rules] = await Promise.all([repository.listClients(), repository.listInvoices('all'), repository.listContracts('all'), repository.listRules('all')])
    const clientIds = new Set(clients.map((client) => client.id))
    const contractsById = new Map(contracts.map((contract) => [contract.id, contract]))
    const rulesById = new Map(rules.map((rule) => [rule.id, rule]))
    const taskIds = invoices.flatMap((invoice) => invoice.task ? [invoice.task.id] : [])

    expect(new Set(taskIds).size).toBe(taskIds.length)
    for (const contract of contracts) expect(clientIds.has(contract.clientId)).toBe(true)
    for (const invoice of invoices) {
      expect(clientIds.has(invoice.clientId)).toBe(true)
      if (invoice.selectedContractId) expect(contractsById.get(invoice.selectedContractId)?.clientId).toBe(invoice.clientId)
      for (const candidate of invoice.task?.contractCandidates ?? []) expect(contractsById.get(candidate.id)?.clientId).toBe(invoice.clientId)
      for (const line of invoice.lines) for (const classification of line.classifications) if (classification.rule) expect(rulesById.get(classification.rule.ruleId)?.versions.some((version) => version.version === classification.rule?.version)).toBe(true)
      for (const item of invoice.task?.classificationItems ?? []) if (item.rule) expect(rulesById.get(item.rule.ruleId)?.versions.some((version) => version.version === item.rule?.version)).toBe(true)
    }
    for (const rule of rules) {
      if (rule.clientId) expect(clientIds.has(rule.clientId)).toBe(true)
      if (rule.parentRuleId) expect(rulesById.get(rule.parentRuleId)?.scope).toBe('GLOBAL')
      for (const version of rule.versions) expect(version.legalBasis).toBe('Exemplu demonstrativ — bază legală nevalidată')
    }
  })
})
