import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { describe, expect, it } from 'vitest'
import { AppProviders } from '../../app/providers'
import type { AIUsagePeriodInput, AIUsageRunKind } from '../../domain/ai-usage'
import { demoAIUsage, demoClientUsage, demoOverview, demoRunDetail, demoRunPage } from '../../mocks/ai-usage'
import { MockInvoiceRepository } from '../../mocks/MockInvoiceRepository'
import { renderApp } from '../../test/render-app'
import { ClientAIUsageCard } from './ClientAIUsageCard'
import { previousMonthPeriod } from './period'

// Fixed demo data; the requested period is recorded but the data window stays
// September 2026 so assertions do not depend on the current date.
const september = { from: '2026-09-01', to: '2026-09-30' }
class UsageRepository extends MockInvoiceRepository {
  readonly usage = demoAIUsage(new Date('2026-09-20T12:00:00Z'))
  readonly requested: AIUsagePeriodInput[] = []
  pageSize?: number
  override async getAIUsageOverview(period: AIUsagePeriodInput) { this.requested.push(period); return demoOverview(this.usage, await this.listClients(), september) }
  override async getClientAIUsage(clientId: string, period: AIUsagePeriodInput) { this.requested.push(period); return demoClientUsage(this.usage, clientId, september) }
  override async listClientAIUsageRuns(clientId: string, _period: AIUsagePeriodInput, page: { limit: number; offset: number }) { return demoRunPage(this.usage, clientId, september, { limit: this.pageSize ?? page.limit, offset: page.offset }) }
  override async getAIUsageRun(clientId: string, runKind: AIUsageRunKind, runId: string) { return demoRunDetail(this.usage, clientId, runKind, runId) }
}

const authUser = { id: 'test-user', email: 'contabil.test@example.com', persona: 'CONTABIL' as const }
function renderCard(repository: UsageRepository, clientId = 'client-alfa') {
  return render(<AppProviders repository={repository} authUser={authUser}><MemoryRouter><ClientAIUsageCard clientId={clientId} /></MemoryRouter></AppProviders>)
}

describe('Consum AI', () => {
  it('shows the account total and every accessible client, most expensive first', async () => {
    renderApp(new UsageRepository(), '/ai-usage')
    expect(await screen.findByRole('heading', { name: 'Consum AI', level: 2 })).toBeInTheDocument()
    const summary = await screen.findByRole('region', { name: /Consum AI Total cont/ })
    expect(within(summary).getByText('0,06 USD')).toBeInTheDocument()
    expect(within(summary).getByText('41.590')).toBeInTheDocument()
    const rows = screen.getAllByRole('row').filter((row) => row.getAttribute('data-client'))
    expect(rows.map((row) => row.getAttribute('data-client'))).toEqual(['client-alfa', 'client-beta'])
    expect(within(rows[0]).getByText('0,05 USD')).toBeInTheDocument()
    expect(within(rows[1]).getByText('0,006 USD')).toBeInTheDocument()
    expect(screen.getByText(/Istoric parțial/)).toBeInTheDocument()
    expect(screen.getByText(/Un apel nu are preț configurat/)).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Consum AI' })).toHaveAttribute('href', '/ai-usage')
  })

  it('reloads the account total for the selected period', async () => {
    const user = userEvent.setup()
    const repository = new UsageRepository()
    renderApp(repository, '/ai-usage')
    await screen.findByRole('region', { name: /Consum AI Total cont/ })
    await user.selectOptions(screen.getByLabelText('Perioada consumului AI'), 'PREVIOUS_MONTH')
    await waitFor(() => expect(repository.requested).toContainEqual(previousMonthPeriod()))
  })

  it('lists the client runs and opens the calls of one run, retries included', async () => {
    const user = userEvent.setup()
    renderCard(new UsageRepository())
    expect(await screen.findByText('0,05 USD')).toBeInTheDocument()
    expect(screen.getByText('37.590')).toBeInTheDocument()
    expect(await screen.findByRole('link', { name: 'Factura FA-2026-0142 · Furnizor Servicii SRL' })).toHaveAttribute('href', '/invoices/inv-alfa-analysis')
    expect(screen.getByRole('link', { name: 'contract-servicii.pdf · încercarea 1' })).toHaveAttribute('href', '/contracts/documents/client-alfa/doc-alfa-1')
    await user.click(screen.getByRole('button', { name: 'Arată apelurile rulării FA-2026-0142 · Furnizor Servicii SRL' }))
    const calls = await screen.findByRole('list', { name: /Apelurile rulării FA-2026-0142/ })
    const items = within(calls).getAllByRole('listitem')
    expect(items).toHaveLength(2)
    expect(items[0]).toHaveTextContent('Eroare HTTP 429')
    expect(items[0]).toHaveTextContent('Fără tokeni raportați')
    expect(items[1]).toHaveTextContent('raționament 1.100')
  })

  it('pages through runs and shows the empty state', async () => {
    const user = userEvent.setup()
    const repository = new UsageRepository()
    repository.pageSize = 2
    const first = renderCard(repository)
    await screen.findByRole('link', { name: /Factura FA-2026-0142/ })
    expect(screen.queryByRole('link', { name: /FA-2026-0101/ })).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Încarcă mai multe' }))
    expect(await screen.findByRole('link', { name: /FA-2026-0101/ })).toBeInTheDocument()
    await waitFor(() => expect(screen.queryByRole('button', { name: 'Încarcă mai multe' })).not.toBeInTheDocument())
    first.unmount()
    renderCard(new UsageRepository(), 'client-without-usage')
    expect(await screen.findByText('Nu există consum AI în perioada selectată.')).toBeInTheDocument()
  })
})
