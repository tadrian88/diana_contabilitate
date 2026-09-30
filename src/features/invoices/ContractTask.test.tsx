import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MockInvoiceRepository } from '../../mocks/MockInvoiceRepository'
import { renderApp } from '../../test/render-app'

describe('Continuă fără contract', () => {
  it('requires a reason of at least 10 characters before continuing', async () => {
    const user = userEvent.setup()
    const repository = new MockInvoiceRepository()
    const spy = vi.spyOn(repository, 'continueWithoutContract')
    renderApp(repository, '/invoices/inv-missing?tab=contract')

    await user.click(await screen.findByRole('button', { name: 'Continuă fără contract' }))
    const dialog = await screen.findByRole('dialog', { name: 'Continui fără contract?' })
    const confirm = within(dialog).getByRole('button', { name: 'Continuă fără contract' })
    expect(confirm).toBeDisabled()

    await user.type(within(dialog).getByLabelText('Motivul deciziei'), '  scurt  ')
    expect(confirm).toBeDisabled()

    await user.click(within(dialog).getByRole('button', { name: 'Anulează' }))
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
    expect(spy).not.toHaveBeenCalled()
  })

  it('continues the invoice and shows the reasoned decision', async () => {
    const user = userEvent.setup()
    const repository = new MockInvoiceRepository()
    const spy = vi.spyOn(repository, 'continueWithoutContract')
    renderApp(repository, '/invoices/inv-missing?tab=contract')

    await user.click(await screen.findByRole('button', { name: 'Continuă fără contract' }))
    const dialog = await screen.findByRole('dialog', { name: 'Continui fără contract?' })
    await user.type(within(dialog).getByLabelText('Motivul deciziei'), 'Achiziție punctuală fără contract')
    await user.click(within(dialog).getByRole('button', { name: 'Continuă fără contract' }))

    await waitFor(() => expect(spy).toHaveBeenCalledWith('inv-missing', 'Achiziție punctuală fără contract'))
    expect((await screen.findAllByText('Continuat fără contract')).length).toBeGreaterThan(0)
    expect(screen.getByText('Achiziție punctuală fără contract')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Solicită contract' })).not.toBeInTheDocument()
  })

  it('is also offered while a requested contract is still awaited', async () => {
    renderApp(new MockInvoiceRepository(), '/invoices/inv-missing-waiting?tab=contract')

    expect((await screen.findAllByText('Contract solicitat')).length).toBeGreaterThan(0)
    expect(screen.getByRole('button', { name: 'Continuă fără contract' })).toBeInTheDocument()
  })
})
