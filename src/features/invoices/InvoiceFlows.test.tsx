import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MockInvoiceRepository } from '../../mocks/MockInvoiceRepository'
import { renderApp } from '../../test/render-app'

describe('fluxurile principale ale facturii', () => {
  it('resolves a contract match and resumes automatic processing', async () => {
    const user = userEvent.setup()
    renderApp(new MockInvoiceRepository(), '/invoices/inv-multiple?tab=contract')

    expect(await screen.findByText('Contract recomandat')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Confirmă' }))

    await waitFor(() => expect(screen.getAllByText('Exportată în SAGA').length).toBeGreaterThan(0), { timeout: 4000 })
  })

  it('keeps the missing-contract task and invoice waiting after the request', async () => {
    const user = userEvent.setup()
    const repository = new MockInvoiceRepository()
    renderApp(repository, '/invoices/inv-missing?tab=contract')

    await user.click(await screen.findByRole('button', { name: 'Solicită contract' }))
    expect(await screen.findByText('Contract solicitat')).toBeInTheDocument()
    expect(screen.getByText(/Task-ul rămâne în așteptare/)).toBeInTheDocument()

    const invoice = await repository.getInvoice('inv-missing')
    expect(invoice?.pipelineStatus).toBe('AWAITING_CONTRACT')
    expect(invoice?.task?.status).toBe('WAITING')
  })

  it('resolves classification only after all uncertain items are reviewed', async () => {
    const user = userEvent.setup()
    const repository = new MockInvoiceRepository()
    renderApp(repository, '/invoices/inv-classification?tab=classification')

    const acceptButtons = await screen.findAllByRole('button', { name: 'Acceptă propunerea' })
    await user.click(acceptButtons[0])
    await waitFor(async () => expect((await repository.getInvoice('inv-classification'))?.pipelineStatus).toBe('AWAITING_REVIEW'))
    expect(screen.getByText('1', { selector: 'div.text-2xl' })).toBeInTheDocument()

    const remainingArticle = screen.getAllByText('Deductibilitate').find((element) => element.tagName === 'H4')?.closest('article')
    await user.click(within(remainingArticle!).getByRole('button', { name: 'Acceptă propunerea' }))
    await waitFor(() => expect(screen.getAllByText('Exportată în SAGA').length).toBeGreaterThan(0), { timeout: 4000 })
    expect((await repository.getInvoice('inv-classification'))?.task?.status).toBe('RESOLVED')
  })

  it('keeps client scope visible', async () => {
    renderApp(new MockInvoiceRepository(), '/')
    expect(await screen.findByTestId('active-scope')).toHaveTextContent('Toți clienții')
    expect(screen.getByLabelText('Selectează clientul')).toBeInTheDocument()
  })
})
