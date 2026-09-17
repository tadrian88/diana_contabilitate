import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { AppProviders } from '../../app/providers'
import { MockInvoiceRepository } from '../../mocks/MockInvoiceRepository'
import type { SPVConnection } from '../../repositories/invoiceRepository'
import { SPVConnectionCard } from './SPVConnectionCard'

class SPVRepository extends MockInvoiceRepository {
  constructor(private value: SPVConnection) { super() }
  override async getSPVConnection() { return structuredClone(this.value) }
  override async startSPVOAuth() { return 'https://anaf.example/authorize?state=opaque' }
  override async requestSPVSync() { this.value.lastSyncStatus = 'RUNNING'; return structuredClone(this.value) }
  override async disconnectSPV() { this.value.status = 'DISABLED'; this.value.importAutomatic = false; return structuredClone(this.value) }
}

const base: SPVConnection = { status: 'NOT_CONNECTED', lastSyncStatus: 'NEVER', importAutomatic: false, configurationReady: true, identityValidation: 'NOT_AVAILABLE' }
function renderCard(value: SPVConnection, initial = '/clients/client-alfa', onAuthorize?: (url: string) => void) {
  return render(<AppProviders repository={new SPVRepository(value)}><MemoryRouter initialEntries={[initial]}><SPVConnectionCard clientId="client-alfa" clientName="Client Alfa" clientCUI="RO12345678" onAuthorize={onAuthorize} /></MemoryRouter></AppProviders>)
}

test('renders the client-scoped not-connected card and starts OAuth redirect', async () => {
  const user = userEvent.setup(); const redirects: string[] = []; renderCard(base, undefined, (url) => redirects.push(url))
  expect(await screen.findByText('Neconectat')).toBeInTheDocument(); expect(screen.getByText(/Conectează Client Alfa/)).toBeInTheDocument()
  expect(screen.getByText('RO12345678')).toBeInTheDocument()
  expect(screen.getByText(/Diana nu primește, nu încarcă și nu stochează certificatul/)).toBeInTheDocument()
  expect(screen.queryByLabelText(/parolă|PIN/i)).not.toBeInTheDocument()
  expect(document.querySelector('input[type="file"]')).not.toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Conectează ANAF' }))
  await waitFor(() => expect(redirects).toEqual(['https://anaf.example/authorize?state=opaque']))
})

test('renders connected metadata, manual sync feedback and disconnect confirmation', async () => {
  const user = userEvent.setup(); renderCard({ ...base, status: 'CONNECTED', environment: 'TEST', connectedAt: '2026-09-14T09:00:00Z', lastSyncAt: '2026-09-14T10:00:00Z', lastSuccessfulSyncAt: '2026-09-14T10:00:00Z', lastSyncStatus: 'SUCCEEDED', importAutomatic: true })
  expect(await screen.findByText('Conectat')).toBeInTheDocument(); expect(screen.getByText('Activ')).toBeInTheDocument(); expect(screen.getByText('Reușită')).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Sincronizează acum' })); expect(await screen.findByText('Sincronizarea a fost pornită.')).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Deconectează' })); expect(screen.getByRole('dialog')).toHaveTextContent('Facturile și documentele importate anterior rămân disponibile.')
  await user.click(screen.getByRole('button', { name: 'Confirmă deconectarea' })); expect(await screen.findByText('Dezactivat')).toBeInTheDocument()
})

test('separates reauthentication from transient last-sync failure', async () => {
  const first = renderCard({ ...base, status: 'CONNECTED', environment: 'PRODUCTION', importAutomatic: true, lastSyncStatus: 'FAILED', safeError: 'Ultima sincronizare nu a putut fi finalizată.' })
  expect(await screen.findByText('Conectat')).toBeInTheDocument(); expect(screen.getByText('Ultima sincronizare nu a putut fi finalizată.')).toBeInTheDocument(); first.unmount()
  renderCard({ ...base, status: 'NEEDS_REAUTHENTICATION', environment: 'PRODUCTION', lastSyncStatus: 'FAILED', safeError: 'Conexiunea ANAF necesită reconectare.' })
  expect(await screen.findByText('Necesită reconectare')).toBeInTheDocument(); expect(screen.getByRole('button', { name: 'Reconectează ANAF' })).toBeInTheDocument()
})

test('renders explicit integration error and incomplete server configuration safely', async () => {
  renderCard({ ...base, status: 'ERROR', configurationReady: false, lastSyncStatus: 'FAILED', safeError: 'Ultima sincronizare nu a putut fi finalizată.' })
  expect(await screen.findByText('Eroare')).toBeInTheDocument()
  expect(screen.getByText('Integrarea ANAF nu este configurată pentru acest mediu.')).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Reconectează ANAF' })).toBeDisabled()
})

test('renders safe callback, loading and API error states', async () => {
  const connected = renderCard(base, '/clients/client-alfa?integration=anaf&result=error&reason=invalid_state')
  expect(await screen.findByText(/Sesiunea de conectare a expirat/)).toBeInTheDocument(); connected.unmount()
  class ErrorRepository extends MockInvoiceRepository { override async getSPVConnection(): Promise<SPVConnection> { throw new Error('raw provider secret') } }
  render(<AppProviders repository={new ErrorRepository()}><MemoryRouter><SPVConnectionCard clientId="client-alfa" clientName="Client Alfa" clientCUI="RO12345678" /></MemoryRouter></AppProviders>)
  expect(screen.getByLabelText('Se încarcă integrarea ANAF')).toBeInTheDocument()
  expect(await screen.findByText('Starea conexiunii ANAF nu a putut fi încărcată.')).toBeInTheDocument()
  expect(screen.queryByText('raw provider secret')).not.toBeInTheDocument()
})
