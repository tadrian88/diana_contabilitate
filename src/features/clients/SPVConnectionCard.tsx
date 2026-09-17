import * as Dialog from '@radix-ui/react-dialog'
import { AlertCircle, KeyRound, Link2, RefreshCw, ShieldCheck, Unplug, X } from 'lucide-react'
import { useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { Badge } from '../../components/ui/badge'
import { Button } from '../../components/ui/button'
import type { SPVConnectionStatus } from '../../repositories/invoiceRepository'
import { useDisconnectSPV, useRequestSPVSync, useSPVConnection, useStartSPVOAuth } from './spv-hooks'

const statusPresentation: Record<SPVConnectionStatus, { label: string; tone: 'neutral' | 'success' | 'warning' | 'danger' }> = {
  NOT_CONNECTED: { label: 'Neconectat', tone: 'neutral' }, CONNECTED: { label: 'Conectat', tone: 'success' },
  NEEDS_REAUTHENTICATION: { label: 'Necesită reconectare', tone: 'warning' }, ERROR: { label: 'Eroare', tone: 'danger' }, DISABLED: { label: 'Dezactivat', tone: 'neutral' },
}

const callbackMessages: Record<string, string> = {
  connected: 'Conexiunea ANAF a fost activată.', authorization_cancelled: 'Autorizarea ANAF a fost anulată.',
  invalid_state: 'Sesiunea de conectare a expirat sau a fost deja utilizată. Pornește din nou conectarea.',
  environment_mismatch: 'Mediul ANAF nu corespunde conexiunii solicitate.', token_exchange: 'ANAF nu a putut finaliza autorizarea.',
  configuration: 'Configurația aplicației ANAF este incompletă.', missing_code: 'ANAF nu a furnizat codul de autorizare.',
  persistence: 'Conexiunea nu a putut fi salvată.', token_security: 'Credentialele nu au putut fi securizate.',
}

export function SPVConnectionCard({ clientId, clientName, clientCUI, onAuthorize = (url) => window.location.assign(url) }: { clientId: string; clientName: string; clientCUI: string; onAuthorize?: (url: string) => void }) {
  const connection = useSPVConnection(clientId); const start = useStartSPVOAuth(clientId); const sync = useRequestSPVSync(clientId); const disconnect = useDisconnectSPV(clientId)
  const [params] = useSearchParams(); const [notice, setNotice] = useState<string>()
  const callbackResult = params.get('integration') === 'anaf' ? params.get('result') : null
  const callbackReason = params.get('reason') ?? ''
  const callbackNotice = callbackResult === 'connected' ? callbackMessages.connected : callbackResult === 'error' ? (callbackMessages[callbackReason] ?? 'Conectarea ANAF nu a putut fi finalizată.') : undefined

  if (connection.isLoading) return <section id="anaf-spv" aria-label="Se încarcă integrarea ANAF" className="card h-48 animate-pulse bg-[var(--surface-subtle)]" />
  if (connection.isError || !connection.data) return <section id="anaf-spv" className="card p-5"><div className="flex items-center gap-2 text-[var(--danger)]"><AlertCircle className="size-5" /><h3 className="font-bold">ANAF / SPV</h3></div><p className="mt-3 text-sm text-[var(--text-secondary)]">Starea conexiunii ANAF nu a putut fi încărcată.</p></section>
  const item = connection.data; const presentation = statusPresentation[item.status]; const canSync = item.status === 'CONNECTED' && item.configurationReady && item.importAutomatic; const canConnect = item.configurationReady && item.safeErrorCode!=='CLIENT_INACTIVE'
  const actionError = start.error || sync.error || disconnect.error
  const connect = () => start.mutate(undefined, { onSuccess: onAuthorize })

  return <section id="anaf-spv" className="card overflow-hidden">
    <div className="flex items-start justify-between gap-5 border-b border-[var(--border)] px-5 py-4"><div><div className="flex items-center gap-2"><Link2 className="size-5 text-[var(--accent)]" /><h3 className="font-bold">ANAF / SPV</h3></div><p className="mt-1 text-xs text-[var(--text-muted)]">Integrare pentru {clientName}</p></div><Badge tone={presentation.tone}>{presentation.label}</Badge></div>
    <div className="space-y-4 p-5">
      {(notice || callbackNotice) && <div role="status" className="rounded-lg border border-[var(--info-border)] bg-[var(--info-soft)] px-4 py-3 text-sm text-[var(--info)]">{notice ?? callbackNotice}</div>}
      {actionError && <div role="alert" className="rounded-lg border border-[var(--danger-border)] bg-[var(--danger-soft)] px-4 py-3 text-sm text-[var(--danger)]">Operația ANAF nu a putut fi finalizată. Încearcă din nou.</div>}
      {item.status === 'NOT_CONNECTED' && <p className="text-sm text-[var(--text-secondary)]">Conectează {clientName} la ANAF/SPV pentru importul automat al facturilor electronice primite.</p>}
      {item.status === 'NEEDS_REAUTHENTICATION' && <p className="text-sm text-[var(--text-secondary)]">Conexiunea cu ANAF nu mai poate fi reînnoită automat.</p>}
      {item.status === 'DISABLED' && <p className="text-sm text-[var(--text-secondary)]">Importul automat este oprit. Facturile importate anterior sunt păstrate.</p>}
      {item.status === 'ERROR' && <p className="text-sm text-[var(--text-secondary)]">Conexiunea necesită verificare sau reconectare.</p>}
      {item.status !== 'CONNECTED' && <CertificateOnboarding clientName={clientName} clientCUI={clientCUI} />}
      {item.status === 'CONNECTED' && <div className="grid grid-cols-3 gap-3 text-sm"><Info label="Client" value={clientName} /><Info label="CUI" value={clientCUI} /><Info label="Import automat" value={item.importAutomatic ? 'Activ' : 'Inactiv'} /><Info label="Mediu" value={item.environment === 'PRODUCTION' ? 'Producție' : 'Test'} /><Info label="Conectat la" value={item.connectedAt ? formatDateTime(item.connectedAt) : '—'} /><Info label="Ultima sincronizare" value={item.lastSyncAt ? formatDateTime(item.lastSyncAt) : 'Nicio sincronizare'} /><Info label="Ultima reușită" value={item.lastSuccessfulSyncAt ? formatDateTime(item.lastSuccessfulSyncAt) : '—'} /><Info label="Rezultat" value={syncLabel(item.lastSyncStatus)} /></div>}
      {item.safeError && <div className="flex items-start gap-2 rounded-lg bg-[var(--warning-soft)] p-3 text-sm text-[var(--warning)]"><AlertCircle className="mt-0.5 size-4 shrink-0" />{item.safeError}</div>}
      {item.identityValidation === 'NOT_AVAILABLE' && item.status === 'CONNECTED' && <p className="flex items-center gap-2 text-xs text-[var(--text-muted)]"><ShieldCheck className="size-4" />Identitatea CUI nu este confirmată de răspunsul OAuth; fiecare factură este verificată ulterior față de CUI-ul clientului.</p>}
      {!item.configurationReady && <p className="text-sm text-[var(--danger)]">Integrarea ANAF nu este configurată pentru acest mediu.</p>}
      <div className="flex flex-wrap gap-3">
        {canSync && <Button disabled={sync.isPending} onClick={() => sync.mutate(undefined, { onSuccess: () => setNotice('Sincronizarea a fost pornită.') })}><RefreshCw className={`size-4 ${sync.isPending ? 'animate-spin' : ''}`} />Sincronizează acum</Button>}
        <Button variant={canSync ? 'secondary' : 'primary'} disabled={!canConnect || start.isPending} onClick={connect}>{item.status === 'NOT_CONNECTED' ? 'Conectează ANAF' : 'Reconectează ANAF'}</Button>
        {item.status !== 'NOT_CONNECTED' && item.status !== 'DISABLED' && <DisconnectDialog clientName={clientName} pending={disconnect.isPending} onConfirm={() => disconnect.mutate(undefined, { onSuccess: () => setNotice('Conexiunea ANAF a fost dezactivată.') })} />}
      </div>
    </div>
  </section>
}

function CertificateOnboarding({ clientName, clientCUI }: { clientName: string; clientCUI: string }) {
  return <div className="rounded-xl border border-[var(--border)] bg-[var(--surface-subtle)] p-4">
    <div className="flex items-start gap-3">
      <span className="grid size-9 shrink-0 place-items-center rounded-lg bg-[var(--info-soft)] text-[var(--info)]"><KeyRound className="size-4" /></span>
      <div className="space-y-3 text-sm">
        <div><h4 className="font-bold">Certificat digital calificat</h4><p className="mt-1 text-[var(--text-secondary)]">Autorizarea se face în pagina securizată ANAF cu certificatul instalat în browser sau disponibil pe tokenul USB.</p></div>
        <dl className="grid gap-2 sm:grid-cols-2"><div><dt className="text-xs text-[var(--text-muted)]">Client</dt><dd className="font-semibold">{clientName}</dd></div><div><dt className="text-xs text-[var(--text-muted)]">CUI</dt><dd className="font-semibold">{clientCUI}</dd></div></dl>
        <ol className="list-decimal space-y-1 pl-5 text-[var(--text-secondary)]">
          <li>Conectează tokenul USB sau confirmă că certificatul este instalat pe acest dispozitiv.</li>
          <li>Continuă la ANAF și selectează certificatul atunci când browserul îl solicită.</li>
          <li>Introdu PIN-ul numai în fereastra certificatului și confirmă autorizarea ANAF.</li>
        </ol>
        <p className="flex items-start gap-2 text-xs text-[var(--text-muted)]"><ShieldCheck className="mt-0.5 size-4 shrink-0" />Diana nu primește, nu încarcă și nu stochează certificatul, cheia privată sau PIN-ul.</p>
      </div>
    </div>
  </div>
}

function DisconnectDialog({ clientName, pending, onConfirm }: { clientName: string; pending: boolean; onConfirm: () => void }) { return <Dialog.Root><Dialog.Trigger asChild><Button variant="ghost"><Unplug className="size-4" />Deconectează</Button></Dialog.Trigger><Dialog.Portal><Dialog.Overlay className="fixed inset-0 z-40 bg-black/40" /><Dialog.Content className="dialog-content fixed left-1/2 top-1/2 z-50 w-[500px] -translate-x-1/2 -translate-y-1/2 rounded-2xl border border-[var(--border)] bg-[var(--surface)] p-6 shadow-2xl"><Dialog.Title className="text-lg font-bold">Deconectează ANAF</Dialog.Title><Dialog.Description className="mt-2 text-sm text-[var(--text-secondary)]">Importul automat pentru {clientName} va fi oprit. Facturile și documentele importate anterior rămân disponibile.</Dialog.Description><div className="mt-6 flex justify-end gap-3"><Dialog.Close asChild><Button variant="ghost">Anulează</Button></Dialog.Close><Dialog.Close asChild><Button variant="danger" disabled={pending} onClick={onConfirm}>Confirmă deconectarea</Button></Dialog.Close></div><Dialog.Close className="absolute right-4 top-4 rounded-md p-1 text-[var(--text-muted)]" aria-label="Închide"><X className="size-4" /></Dialog.Close></Dialog.Content></Dialog.Portal></Dialog.Root> }
function Info({ label, value }: { label: string; value: string }) { return <div className="rounded-lg bg-[var(--surface-subtle)] p-3"><div className="text-xs text-[var(--text-muted)]">{label}</div><div className="mt-1 font-semibold">{value}</div></div> }
function syncLabel(status: string) { return status === 'RUNNING' ? 'În curs' : status === 'SUCCEEDED' ? 'Reușită' : status === 'FAILED' ? 'Eșuată' : 'Nicio sincronizare' }
function formatDateTime(value: string) { return new Intl.DateTimeFormat('ro-RO', { dateStyle: 'medium', timeStyle: 'short', timeZone: 'Europe/Bucharest' }).format(new Date(value)) }
