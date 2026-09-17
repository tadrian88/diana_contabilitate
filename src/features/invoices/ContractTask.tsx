import * as Dialog from '@radix-ui/react-dialog'
import { CheckCircle2, ChevronRight, Info, X } from 'lucide-react'
import { useState } from 'react'
import { Link, useLocation } from 'react-router-dom'
import { Badge } from '../../components/ui/badge'
import { Button } from '../../components/ui/button'
import type { Invoice } from '../../domain/invoice'
import { useResolveContract, useRequestContract } from './invoice-mutations'

export function ContractTask({ invoice }: { invoice: Invoice }) {
  const task = invoice.task
  const resolveContract = useResolveContract(invoice.id)
  const requestContract = useRequestContract(invoice.id)
  const [alternativeId, setAlternativeId] = useState('')
  const location = useLocation()
  const invoiceReturnTo = `${location.pathname}${location.search}`

  if (!task || task.type === 'CLASSIFICATION') return <ContractResolved invoice={invoice} returnTo={invoiceReturnTo} />

  if (task.type === 'MISSING_CONTRACT') {
    return (
      <div className="card p-6">
        <div className="flex items-start justify-between gap-4">
          <div>
            <Badge tone={task.status === 'WAITING' ? 'info' : 'warning'}>{task.status === 'WAITING' ? 'În așteptare' : 'Decizie necesară'}</Badge>
            <h3 className="mt-3 text-lg font-bold">{task.title}</h3>
            <p className="mt-1 text-sm text-[var(--text-secondary)]">{task.reason}</p>
          </div>
          <Info className="size-5 text-[var(--warning)]" />
        </div>
        {task.contractRequested ? (
          <div className="mt-6 rounded-xl border border-[var(--info-border)] bg-[var(--info-soft)] p-4" role="status">
            <div className="font-semibold text-[var(--info)]">Contract solicitat</div>
            <p className="mt-1 text-sm text-[var(--text-secondary)]">Task-ul rămâne în așteptare. Factura nu avansează până când contractul este disponibil printr-o condiție externă.</p>
          </div>
        ) : (
          <div className="mt-6">
            <Button onClick={() => requestContract.mutate()} disabled={requestContract.isPending}>Solicită contract</Button>
          </div>
        )}
        <Link to={`/contracts/upload?clientId=${encodeURIComponent(invoice.clientId)}&invoiceId=${encodeURIComponent(invoice.id)}`} className="mt-5 inline-block rounded-lg border border-[var(--border-strong)] px-4 py-2 text-sm font-semibold text-[var(--accent)]">Încarcă contract</Link>
      </div>
    )
  }

  const candidates = task.contractCandidates ?? []
  const recommended = candidates.find((candidate) => candidate.recommended)
  const alternatives = candidates.filter((candidate) => !candidate.recommended)

  if (task.status === 'RESOLVED') return <ContractResolved invoice={invoice} returnTo={invoiceReturnTo} />

  return (
    <div className="space-y-4">
      <div className="card p-6">
        <Badge tone="warning">Decizie necesară</Badge>
        <div className="mt-3 flex items-start justify-between">
          <div><h3 className="text-lg font-bold">{task.title}</h3><p className="mt-1 text-sm text-[var(--text-secondary)]">{task.reason}</p></div>
          <span className="text-xs font-semibold text-[var(--text-muted)]">Date demonstrative</span>
        </div>
      </div>
      {recommended && (
        <div className="card overflow-hidden border-[var(--info-border)]">
          <div className="flex items-center justify-between border-b border-[var(--info-border)] bg-[var(--info-soft)] px-5 py-3">
            <div className="flex items-center gap-2 font-semibold text-[var(--info)]"><CheckCircle2 className="size-4" /> Contract recomandat</div>
            <Badge tone="info">Încredere: {recommended.confidence}</Badge>
          </div>
          <div className="grid grid-cols-[1fr_1fr] gap-6 p-5">
            <div>
              <div className="text-lg font-bold">{recommended.reference}</div>
              <div className="mt-1 text-sm text-[var(--text-secondary)]">{recommended.supplierName}</div>
              <dl className="mt-4 grid grid-cols-2 gap-3 text-sm">
                <div><dt className="text-xs text-[var(--text-muted)]">Perioadă</dt><dd className="mt-1 font-medium">{recommended.period}</dd></div>
                <div><dt className="text-xs text-[var(--text-muted)]">Valoare</dt><dd className="mt-1 font-medium">{formatMoney(recommended.value.amount, recommended.value.currency)}</dd></div>
              </dl>
              <Link to={`/contracts/${recommended.id}?invoiceId=${invoice.id}&returnTo=${encodeURIComponent(invoiceReturnTo)}`} className="mt-4 inline-flex rounded text-xs font-semibold text-[var(--accent)] outline-none hover:underline focus-visible:ring-2 focus-visible:ring-[var(--focus)]">Inspectează contractul</Link>
            </div>
            <div>
              <div className="eyebrow">De ce a fost propus</div>
              <ul className="mt-3 space-y-2 text-sm text-[var(--text-secondary)]">{recommended.reasons.map((reason) => <li key={reason} className="flex gap-2"><CheckCircle2 className="mt-0.5 size-4 shrink-0 text-[var(--success)]" />{reason}</li>)}</ul>
            </div>
          </div>
          <div className="flex gap-3 border-t border-[var(--border)] bg-[var(--surface-subtle)] px-5 py-4">
            <Button onClick={() => resolveContract.mutate(recommended.id)} disabled={resolveContract.isPending}>Confirmă</Button>
            {alternatives.length > 0 && <Dialog.Root>
              <Dialog.Trigger asChild><Button variant="secondary">Alege alt contract</Button></Dialog.Trigger>
              <Dialog.Portal>
                <Dialog.Overlay className="fixed inset-0 z-40 bg-black/40" />
                <Dialog.Content className="dialog-content fixed left-1/2 top-1/2 z-50 w-[520px] -translate-x-1/2 -translate-y-1/2 rounded-2xl border border-[var(--border)] bg-[var(--surface)] p-6 shadow-2xl">
                  <Dialog.Title className="text-lg font-bold">Alege alt contract</Dialog.Title>
                  <Dialog.Description className="mt-1 text-sm text-[var(--text-secondary)]">Selectează alternativa potrivită pentru această factură demonstrativă.</Dialog.Description>
                  <div className="mt-5 space-y-3">
                    {alternatives.map((candidate) => (
                      <div key={candidate.id} className={`rounded-xl border p-4 ${alternativeId === candidate.id ? 'border-[var(--accent)] bg-[var(--info-soft)]' : 'border-[var(--border)]'}`}>
                        <label className="flex cursor-pointer gap-3">
                          <input type="radio" name="alternative-contract" value={candidate.id} checked={alternativeId === candidate.id} onChange={() => setAlternativeId(candidate.id)} />
                          <span className="flex-1"><strong>{candidate.reference}</strong><span className="mt-1 block text-xs text-[var(--text-secondary)]">{candidate.period} · Încredere: {candidate.confidence}</span></span>
                        </label>
                        <Link to={`/contracts/${candidate.id}?invoiceId=${invoice.id}&returnTo=${encodeURIComponent(invoiceReturnTo)}`} className="ml-7 mt-2 inline-flex rounded text-xs font-semibold text-[var(--accent)] outline-none hover:underline focus-visible:ring-2 focus-visible:ring-[var(--focus)]">Inspectează detaliile</Link>
                      </div>
                    ))}
                  </div>
                  <div className="mt-6 flex justify-end gap-3">
                    <Dialog.Close asChild><Button variant="ghost">Anulează</Button></Dialog.Close>
                    <Dialog.Close asChild><Button disabled={!alternativeId} onClick={() => alternativeId && resolveContract.mutate(alternativeId)}>Confirmă selecția</Button></Dialog.Close>
                  </div>
                  <Dialog.Close className="absolute right-4 top-4 rounded-md p-1 text-[var(--text-muted)] hover:bg-[var(--surface-subtle)]" aria-label="Închide"><X className="size-4" /></Dialog.Close>
                </Dialog.Content>
              </Dialog.Portal>
            </Dialog.Root>}
          </div>
        </div>
      )}
    </div>
  )
}

function ContractResolved({ invoice, returnTo }: { invoice: Invoice; returnTo: string }) {
  const matched = invoice.contract
  return (
    <div className="card p-6">
      <div className="flex items-center gap-2 text-[var(--success)]"><CheckCircle2 className="size-5" /><span className="font-semibold">Contract asociat</span></div>
      {matched ? <div className="mt-4 rounded-xl bg-[var(--surface-subtle)] p-4">
        <div className="flex items-center justify-between"><div><div className="font-semibold">{matched.reference}</div><div className="mt-1 text-xs text-[var(--text-muted)]">{matched.supplierName}</div></div><Link to={`/contracts/${matched.id}?invoiceId=${invoice.id}&returnTo=${encodeURIComponent(returnTo)}`} aria-label={`Inspectează contractul ${matched.reference}`} className="rounded p-2 text-[var(--accent)] outline-none hover:bg-[var(--info-soft)] focus-visible:ring-2 focus-visible:ring-[var(--focus)]"><ChevronRight className="size-4" /></Link></div>
        <dl className="mt-4 grid grid-cols-5 gap-4 border-t border-[var(--border)] pt-4 text-sm">
          <div><dt className="text-xs text-[var(--text-muted)]">Perioadă</dt><dd className="mt-1 font-medium">{matched.period}</dd></div>
          <div><dt className="text-xs text-[var(--text-muted)]">Monedă</dt><dd className="mt-1 font-medium">{matched.currency}</dd></div>
          <div><dt className="text-xs text-[var(--text-muted)]">Valoare</dt><dd className="mt-1 font-medium">{formatMoney(matched.value.amount, matched.value.currency)}</dd></div>
          <div><dt className="text-xs text-[var(--text-muted)]">Tip unitate</dt><dd className="mt-1 font-medium">{matched.unitType}</dd></div>
          <div><dt className="text-xs text-[var(--text-muted)]">Termen plată</dt><dd className="mt-1 font-medium">{matched.paymentTerms}</dd></div>
        </dl>
      </div> : <div className="mt-4 rounded-xl bg-[var(--surface-subtle)] p-4"><div className="font-semibold">{invoice.selectedContractId ?? 'Contract demonstrativ asociat'}</div><div className="mt-1 text-xs text-[var(--text-muted)]">Detaliile opționale ale contractului nu sunt disponibile.</div></div>}
    </div>
  )
}

function formatMoney(amount: number, currency: string) {
  return new Intl.NumberFormat('ro-RO', { style: 'currency', currency }).format(amount)
}
