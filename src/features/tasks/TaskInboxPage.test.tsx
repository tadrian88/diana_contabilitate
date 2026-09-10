import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MockInvoiceRepository } from '../../mocks/MockInvoiceRepository'
import { renderApp } from '../../test/render-app'

describe('Task Inbox', () => {
  it('shows OPEN tasks across all clients by default', async () => {
    renderApp(new MockInvoiceRepository(), '/tasks')

    expect((await screen.findAllByRole('heading', { name: 'Task Inbox' })).length).toBeGreaterThan(0)
    expect(screen.getByTestId('active-scope')).toHaveTextContent('Toți clienții')
    expect(await screen.findAllByText('Deschis')).toHaveLength(4)
    expect(screen.getAllByText('Client Demo Alfa SRL').length).toBeGreaterThan(0)
    expect(screen.getAllByText('Client Demo Beta SRL').length).toBeGreaterThan(0)
  })

  it('scopes tasks when a client is selected', async () => {
    const user = userEvent.setup()
    renderApp(new MockInvoiceRepository(), '/tasks')

    await user.click(await screen.findByLabelText('Selectează clientul'))
    await user.click(screen.getByRole('option', { name: 'Client Demo Alfa SRL' }))

    await waitFor(() => expect(screen.getByTestId('active-scope')).toHaveTextContent('Client Demo Alfa SRL'))
    expect(screen.getAllByText('Client Demo Alfa SRL').length).toBeGreaterThan(0)
    expect(screen.queryByText('Client Demo Beta SRL')).not.toBeInTheDocument()
  })

  it('filters tasks by type', async () => {
    const user = userEvent.setup()
    renderApp(new MockInvoiceRepository(), '/tasks')

    await user.selectOptions(await screen.findByLabelText('Filtrează după tipul task-ului'), 'CLASSIFICATION')
    expect(screen.getByText('DEMO-CL-004')).toBeInTheDocument()
    expect(screen.queryByText('DEMO-MC-002')).not.toBeInTheDocument()
  })

  it('shows WAITING tasks and moves a requested contract there', async () => {
    const user = userEvent.setup()
    renderApp(new MockInvoiceRepository(), '/tasks')

    const missingRow = (await screen.findByText('DEMO-NC-003')).closest('tr')
    await user.click(within(missingRow!).getByRole('button', { name: 'Solicită contract' }))
    await waitFor(() => expect(screen.queryByText('DEMO-NC-003')).not.toBeInTheDocument())

    await user.click(screen.getByRole('tab', { name: /În așteptare/ }))
    expect(await screen.findByText('DEMO-NC-003')).toBeInTheDocument()
    expect(screen.getByText('DEMO-WT-006')).toBeInTheDocument()
  })

  it('removes a resolved task from OPEN and updates Dashboard attention', async () => {
    const user = userEvent.setup()
    const repository = new MockInvoiceRepository()
    renderApp(repository, '/tasks')

    const contractRow = (await screen.findByText('DEMO-MC-002')).closest('tr')
    await user.click(within(contractRow!).getByRole('link', { name: 'Rezolvă' }))
    await user.click(await screen.findByRole('button', { name: 'Confirmă' }))
    await user.click(screen.getByRole('link', { name: 'Înapoi la Task Inbox' }))
    await waitFor(() => expect(screen.queryByText('DEMO-MC-002')).not.toBeInTheDocument())

    await user.click(screen.getByRole('link', { name: 'Dashboard' }))
    const attentionLabel = (await screen.findAllByText('Necesită atenție')).find((element) => element.closest('article'))
    const attentionCard = attentionLabel?.closest('article')
    expect(within(attentionCard!).getByText('3')).toBeInTheDocument()
    await waitFor(async () => expect((await repository.getInvoice('inv-multiple'))?.pipelineStatus).toBe('EXPORTED'), { timeout: 4000 })
  })
})
