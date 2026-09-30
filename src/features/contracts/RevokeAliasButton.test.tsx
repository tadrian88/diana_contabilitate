import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { vi } from 'vitest'
import { RevokeAliasButton } from './RevokeAliasButton'

const handlers = vi.hoisted(() => ({ revoke: vi.fn() }))
vi.mock('./contract-hooks', () => ({ useRevokeCommercialServiceAlias: () => ({ mutate: handlers.revoke, reset: vi.fn(), isPending: false, isError: false }) }))

const alias = { id: 'alias-1', dossierId: 'dossier-1', serviceId: 'service-cleaning', serviceLabel: 'Servicii curățenie birou', normalizedLabel: 'SERVICII CURĂȚENIE BIROU', confirmedBy: 'ana@firma.ro', confirmedAt: '2026-09-30T06:53:14Z' }

describe('Revoking a learned wording', () => {
  it('asks first and explains that only future invoices are affected', async () => {
    const user = userEvent.setup()
    render(<RevokeAliasButton clientId="client-1" alias={alias} appearance="link" />)
    await user.click(screen.getByRole('button', { name: 'Revocă' }))
    expect(handlers.revoke).not.toHaveBeenCalled()
    expect(screen.getByRole('dialog')).toHaveTextContent('Facturile viitoare din acest contract')
    expect(screen.getByRole('dialog')).toHaveTextContent('Asocierile confirmate direct pe o factură rămân valabile')
    await user.click(screen.getByRole('button', { name: 'Anulează' }))
    expect(handlers.revoke).not.toHaveBeenCalled()
    await user.click(screen.getByRole('button', { name: 'Revocă' }))
    await user.click(screen.getByRole('button', { name: 'Revocă formularea' }))
    expect(handlers.revoke).toHaveBeenCalledWith('alias-1', expect.anything())
  })
})
