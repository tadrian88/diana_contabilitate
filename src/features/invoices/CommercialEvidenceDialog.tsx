import * as Dialog from '@radix-ui/react-dialog'
import { ChevronLeft, ChevronRight, CircleCheck, CircleHelp, CircleX, Info, TriangleAlert, X } from 'lucide-react'
import { lazy, Suspense, useMemo, useState, type KeyboardEvent } from 'react'
import { Link } from 'react-router-dom'
import { Badge } from '../../components/ui/badge'
import { Button } from '../../components/ui/button'
import type { Invoice } from '../../domain/invoice'
import type { CommercialFinding, CommercialOutcome, CommercialServiceCandidate, CommercialValidationRun } from '../../repositories/invoiceRepository'
import type { HighlightResult } from '../contracts/ContractPDFPreview'
import { invoiceFocus, lineItemDescription, unitLabel, type CommercialView } from './commercial-view'
import { useConfirmCommercialServiceAliases, useResolveCommercialValidation } from './invoice-hooks'
import { InvoiceSourceExcerpt } from './InvoiceSourceExcerpt'

// pdf.js is loaded when a contract page is first shown, not with the invoice.
const ContractPDFPreview = lazy(() => import('../contracts/ContractPDFPreview').then((module) => ({ default: module.ContractPDFPreview })))

export type EvidenceDialogState = { mode: 'verify'; checkId: string } | { mode: 'map'; lineId?: string }

const outcomeLabel: Record<CommercialOutcome, string> = { CONFORM: 'Conform', NECONFORM: 'Neconform', NEVERIFICABIL: 'Nu poate fi verificat' }
const outcomeTone = { CONFORM: 'success', NECONFORM: 'danger', NEVERIFICABIL: 'warning' } as const
const Kbd = ({ children }: { children: string }) => <kbd className="inline-flex h-[22px] min-w-[22px] items-center justify-center rounded-md border border-b-2 border-[var(--border-strong)] bg-[var(--surface)] px-1.5 font-sans text-[11px] font-semibold text-[var(--text-secondary)]">{children}</kbd>
const typing = (target: EventTarget) => target instanceof HTMLElement && (target.tagName === 'TEXTAREA' || (target.tagName === 'INPUT' && (target as HTMLInputElement).type === 'text'))

// A full-screen side-by-side view: what the e-Factura says on the left, the
// original contract page with the cited words marked on the right, and the
// verdict above both, so a check is confirmed without searching either
// document. The same frame maps uncovered lines to contractual services.
export function CommercialEvidenceDialog({ invoice, run, view, state, reviewable, onStateChange, onClose, onMapped }: { invoice: Invoice; run: CommercialValidationRun; view: CommercialView; state: EvidenceDialogState; reviewable: boolean; onStateChange: (state: EvidenceDialogState) => void; onClose: () => void; onMapped: (count: number) => void }) {
  return (
    <Dialog.Root open onOpenChange={(open) => { if (!open) onClose() }}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-40 bg-black/50" />
        <Dialog.Content aria-describedby={undefined} className="dialog-content fixed inset-2 z-50 flex flex-col overflow-hidden rounded-2xl border border-[var(--border)] bg-[var(--surface)] shadow-2xl sm:inset-5">
          {state.mode === 'verify'
            ? <VerifyPanel invoice={invoice} run={run} view={view} checkId={state.checkId} reviewable={reviewable} onNavigate={(checkId) => onStateChange({ mode: 'verify', checkId })} onClose={onClose} />
            : <MappingPanel invoice={invoice} run={run} view={view} initialLineId={state.lineId} onClose={onClose} onMapped={onMapped} />}
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}

function contractDocumentId(run: CommercialValidationRun) {
  return run.findings.flatMap((finding) => [...(finding.evidence ?? []), ...(finding.serviceCandidates ?? []).flatMap((candidate) => candidate.evidence ?? [])]).find((item) => item.documentId)?.documentId
}

function EvidencePdf({ clientId, documentId, evidence }: { clientId: string; documentId: string; evidence: Array<{ page?: number; snippet: string }> }) {
  const first = evidence.find((item) => item.page)?.page
  const [page, setPage] = useState(first ?? 1)
  const [result, setResult] = useState<HighlightResult>()
  const snippets = evidence.map((item) => item.snippet)
  return (
    <div className="flex h-full min-h-0 flex-col">
      {result && !result.found && snippets.length > 0 && <p role="status" className="border-b border-[var(--warning-border)] bg-[var(--warning-soft)] px-4 py-2 text-xs">Nu am putut localiza exact textul în PDF. Fragmentul extras din contract: „{snippets.join(' · ')}”</p>}
      {result?.found && !result.exact && <p role="status" className="border-b border-[var(--border)] bg-[var(--surface-subtle)] px-4 py-2 text-xs text-[var(--text-secondary)]">Textul este împărțit pe mai multe rânduri în PDF; a fost evidențiat începutul lui.</p>}
      <Suspense fallback={<p role="status" className="p-6 text-sm text-[var(--text-secondary)]">Se încarcă PDF-ul…</p>}>
        <ContractPDFPreview clientId={clientId} documentId={documentId} page={page} onPage={setPage} highlights={snippets} locateAcrossPages={!first} onHighlight={setResult} className="flex min-h-0 flex-1 flex-col" viewportClassName="min-h-0 flex-1" />
      </Suspense>
    </div>
  )
}

function OutcomeIcon({ outcome }: { outcome: CommercialOutcome }) {
  if (outcome === 'CONFORM') return <CircleCheck aria-hidden="true" className="size-5 text-[var(--success)]" />
  if (outcome === 'NECONFORM') return <CircleX aria-hidden="true" className="size-5 text-[var(--danger)]" />
  return <TriangleAlert aria-hidden="true" className="size-5 text-[var(--warning)]" />
}

function VerifyPanel({ invoice, run, view, checkId, reviewable, onNavigate, onClose }: { invoice: Invoice; run: CommercialValidationRun; view: CommercialView; checkId: string; reviewable: boolean; onNavigate: (checkId: string) => void; onClose: () => void }) {
  const items = view.verifiable
  const index = Math.max(0, items.findIndex((item) => item.id === checkId))
  const check = items[index]
  const resolve = useResolveCommercialValidation(invoice)
  const [reason, setReason] = useState('')
  if (!check) return <div className="p-6"><Dialog.Title className="font-bold">Nu există verificări de afișat</Dialog.Title><Button className="mt-4" variant="secondary" onClick={onClose}>Închide</Button></div>
  const previous = index > 0 ? items[index - 1] : undefined
  const next = index < items.length - 1 ? items[index + 1] : undefined
  const finding = check.findings[0]
  const documentId = check.evidence.find((item) => item.documentId)?.documentId
  const evidence = check.evidence.filter((item) => item.documentId === documentId)
  const canResolve = reviewable && check.outcome === 'NECONFORM' && check.findings.length === 1 && !finding.override
  const detail = check.code.startsWith('VAT_RATE_') && finding.calculation ? `${finding.reason} ${finding.calculation}.` : check.explanation ?? finding.reason
  const act = (action: 'ACCEPT_EXCEPTION' | 'WAIT_FOR_CORRECTION') => resolve.mutate({ runId: run.id, findingId: finding.id, expectedInvoiceRevision: invoice.revision ?? run.invoiceRevision + 1, action, reason: action === 'ACCEPT_EXCEPTION' ? reason.trim() : undefined })
  const onKeyDown = (event: KeyboardEvent) => {
    if (typing(event.target)) return
    if (event.key === 'ArrowRight' && next) { event.preventDefault(); onNavigate(next.id) }
    if (event.key === 'ArrowLeft' && previous) { event.preventDefault(); onNavigate(previous.id) }
  }
  return (
    <div className="flex min-h-0 flex-1 flex-col" onKeyDown={onKeyDown}>
      <header className="flex h-16 shrink-0 items-center justify-between gap-4 border-b border-[var(--border)] pl-6 pr-4">
        <div className="min-w-0"><p className="eyebrow">Verificare față în față</p><Dialog.Title className="mt-0.5 truncate text-lg font-bold">{check.title} · {check.scope}</Dialog.Title></div>
        <div className="flex items-center gap-2">
          <Button type="button" variant="secondary" size="icon" aria-label="Verificarea anterioară" disabled={!previous} onClick={() => previous && onNavigate(previous.id)}><ChevronLeft className="size-4" /></Button>
          <span className="min-w-16 text-center text-sm tabular-nums text-[var(--text-secondary)]">{index + 1} din {items.length}</span>
          <Button type="button" variant="secondary" size="icon" aria-label="Verificarea următoare" disabled={!next} onClick={() => next && onNavigate(next.id)}><ChevronRight className="size-4" /></Button>
          <span className="mx-1 h-7 w-px bg-[var(--border)]" />
          <Dialog.Close asChild><Button type="button" variant="secondary" size="icon" aria-label="Închide"><X className="size-4" /></Button></Dialog.Close>
        </div>
      </header>
      <section aria-label="Rezultatul verificării" className="grid shrink-0 gap-4 border-b border-[var(--border)] bg-[var(--surface-subtle)] px-6 py-4 md:grid-cols-[minmax(0,1fr)_40px_minmax(0,1fr)_minmax(0,1.5fr)] md:items-center">
        <div><p className="eyebrow">Pe factură</p><p className="mt-0.5 text-2xl font-bold tabular-nums">{check.invoiceValue ?? '—'}</p>{check.invoiceDetail && <p className="mt-0.5 text-xs text-[var(--text-secondary)]">{check.invoiceDetail}</p>}</div>
        <div className={`hidden text-center text-3xl font-semibold md:block ${check.outcome === 'CONFORM' ? 'text-[var(--success)]' : check.outcome === 'NECONFORM' ? 'text-[var(--danger)]' : 'text-[var(--warning)]'}`} aria-hidden="true">{check.outcome === 'CONFORM' ? '=' : check.outcome === 'NECONFORM' ? '≠' : '?'}</div>
        <div><p className="eyebrow">În contract</p><p className="mt-0.5 text-2xl font-bold tabular-nums">{check.contractValue ?? '—'}</p>{check.contractDetail && <p className="mt-0.5 text-xs text-[var(--text-secondary)]">{check.contractDetail}</p>}</div>
        <div className="flex flex-col items-start gap-1.5 md:border-l md:border-[var(--border)] md:pl-4"><Badge tone={outcomeTone[check.outcome]}>{outcomeLabel[check.outcome]}</Badge><p className="text-sm leading-snug text-[var(--text-secondary)]">{detail}</p>{finding.override && <p className="text-xs text-[var(--success)]">Excepție aprobată: {finding.override.reason}</p>}</div>
      </section>
      <div className="grid min-h-0 flex-1 grid-rows-2 lg:grid-cols-[minmax(0,1fr)_minmax(0,1.05fr)] lg:grid-rows-1">
        <section aria-label="Extras din factură" className="flex min-h-0 flex-col border-b border-[var(--border)] lg:border-b-0 lg:border-r">
          <div className="flex h-11 shrink-0 items-center justify-between border-b border-[var(--border)] px-5"><span className="eyebrow">Factura · e-Factura din SPV</span><span className="text-xs text-[var(--text-muted)]">{invoice.documentNumber}</span></div>
          <div className="min-h-0 flex-1"><InvoiceSourceExcerpt invoice={invoice} focus={invoiceFocus(check, invoice)} /></div>
        </section>
        <section aria-label="Contractul original" className="flex min-h-0 flex-col">
          {documentId ? <EvidencePdf key={check.id} clientId={invoice.clientId} documentId={documentId} evidence={evidence} /> : <p className="p-6 text-sm text-[var(--text-secondary)]">Această verificare nu citează un fragment din contract.</p>}
        </section>
      </div>
      <footer className="flex shrink-0 flex-wrap items-center justify-between gap-3 border-t border-[var(--border)] px-6 py-3">
        <div className="hidden items-center gap-4 text-xs text-[var(--text-secondary)] md:flex"><span className="flex items-center gap-1"><Kbd>←</Kbd><Kbd>→</Kbd> verificarea anterioară / următoare</span><span className="flex items-center gap-1"><Kbd>Esc</Kbd> închide</span></div>
        <div className="flex flex-1 flex-wrap items-center justify-end gap-2">
          {canResolve && <>
            <label className="sr-only" htmlFor="commercial-exception-reason">Motivul excepției</label>
            <input id="commercial-exception-reason" value={reason} onChange={(event) => setReason(event.target.value)} placeholder="Motiv obligatoriu pentru excepție" className="h-10 min-w-64 flex-1 rounded-lg border border-[var(--border-strong)] bg-[var(--surface)] px-3 text-sm" />
            <Button type="button" variant="secondary" disabled={resolve.isPending} onClick={() => act('WAIT_FOR_CORRECTION')}>Așteaptă corecția</Button>
            <Button type="button" variant="secondary" disabled={!reason.trim() || resolve.isPending} onClick={() => act('ACCEPT_EXCEPTION')}>Acceptă excepția</Button>
          </>}
          {documentId && <Link className="inline-flex h-10 items-center rounded-lg border border-[var(--border-strong)] px-4 text-sm font-semibold hover:bg-[var(--surface-subtle)]" to={`/contracts/documents/${encodeURIComponent(invoice.clientId)}/${encodeURIComponent(documentId)}?element=${encodeURIComponent(`rule:${finding.ruleId}`)}`}>Deschide contractul</Link>}
          <Button type="button" onClick={() => next ? onNavigate(next.id) : onClose()}>{next ? 'Următoarea verificare' : 'Gata, închide'}</Button>
        </div>
        {resolve.isError && <p role="alert" className="w-full text-right text-xs text-[var(--danger)]">Decizia nu a putut fi salvată.</p>}
      </footer>
    </div>
  )
}

const pricingLabels: Record<string, string> = { FIXED_PRICE: 'tarif fix', UNIT_RATE: 'tarif pe unitate', TIERED_PRICE: 'tarif pe praguri' }
const frequencyLabels: Record<string, string> = { MONTHLY: 'facturare lunară', QUARTERLY: 'facturare trimestrială', ANNUAL: 'facturare anuală', PER_OCCURRENCE: 'la fiecare prestare' }
const chip = (positive: boolean) => `inline-flex items-center rounded-md border px-2 py-0.5 text-xs font-medium ${positive ? 'border-[var(--success-border)] bg-[var(--success-soft)] text-[var(--success)]' : 'border-[var(--border)] bg-[var(--surface-subtle)] text-[var(--text-secondary)]'}`

function candidateSignals(candidate: CommercialServiceCandidate) {
  const words = candidate.lineWordCount ? (candidate.sharedWords?.length ? { text: `${candidate.sharedWords.length} din ${candidate.lineWordCount} cuvinte ale liniei`, positive: Boolean(candidate.suggested) } : { text: 'Niciun cuvânt comun', positive: false }) : undefined
  const unit = candidate.unitMatch === 'COMPATIBLE' ? { text: `UM compatibilă: ${candidate.lineUnit} = ${candidate.unit}`, positive: true } : candidate.unitMatch === 'INCOMPATIBLE' ? { text: `UM diferită: ${candidate.lineUnit} ≠ ${candidate.unit}`, positive: false } : undefined
  return [words, unit].filter((item): item is { text: string; positive: boolean } => Boolean(item))
}

function MappingPanel({ invoice, run, view, initialLineId, onClose, onMapped }: { invoice: Invoice; run: CommercialValidationRun; view: CommercialView; initialLineId?: string; onClose: () => void; onMapped: (count: number) => void }) {
  const mappings = view.mappings
  const [activeLineId, setActiveLineId] = useState(initialLineId && mappings.some((item) => item.line.id === initialLineId) ? initialLineId : mappings[0]?.line.id)
  const [choices, setChoices] = useState<Record<string, string>>(() => Object.fromEntries(mappings.map(({ line, finding }) => [line.id, finding.serviceCandidates?.find((candidate) => candidate.suggested)?.ruleId ?? ''])))
  const [remember, setRemember] = useState(true)
  const confirm = useConfirmCommercialServiceAliases(invoice)
  const fallbackDocumentId = useMemo(() => contractDocumentId(run), [run])
  const activeIndex = Math.max(0, mappings.findIndex((item) => item.line.id === activeLineId))
  const active = mappings[activeIndex]
  if (!active) return <div className="p-6"><Dialog.Title className="font-bold">Toate liniile sunt asociate</Dialog.Title><Button className="mt-4" variant="secondary" onClick={onClose}>Închide</Button></div>
  const candidates = active.finding.serviceCandidates ?? []
  const choice = choices[active.line.id] ?? ''
  const selected = candidates.find((candidate) => candidate.ruleId === choice)
  const suggestedId = (lineId: string) => mappings.find((item) => item.line.id === lineId)?.finding.serviceCandidates?.find((candidate) => candidate.suggested)?.ruleId
  const chosen = mappings.filter(({ line }) => choices[line.id] && choices[line.id] !== 'none')
  const allOutside = mappings.every(({ line }) => choices[line.id] === 'none')
  const documentId = selected?.evidence?.find((item) => item.documentId)?.documentId ?? fallbackDocumentId
  const choose = (value: string) => setChoices((current) => ({ ...current, [active.line.id]: value }))
  const submit = () => {
    if (!chosen.length || confirm.isPending) return
    confirm.mutate({ runId: run.id, expectedInvoiceRevision: invoice.revision ?? run.invoiceRevision + 1, reuseForDossier: remember, choices: chosen.map(({ line }) => ({ lineId: line.id, serviceId: choices[line.id] })) }, { onSuccess: () => onMapped(chosen.length) })
  }
  const onKeyDown = (event: KeyboardEvent) => {
    if (typing(event.target)) return
    if (event.key === 'ArrowDown' && activeIndex < mappings.length - 1) { event.preventDefault(); setActiveLineId(mappings[activeIndex + 1].line.id) }
    if (event.key === 'ArrowUp' && activeIndex > 0) { event.preventDefault(); setActiveLineId(mappings[activeIndex - 1].line.id) }
    const digit = Number(event.key)
    if (Number.isInteger(digit) && digit >= 1 && digit <= candidates.length + 1) { event.preventDefault(); choose(digit === candidates.length + 1 ? 'none' : candidates[digit - 1].ruleId) }
    if (event.key === 'Enter' && event.target instanceof HTMLElement && !['BUTTON', 'A'].includes(event.target.tagName)) { event.preventDefault(); submit() }
  }
  const wordings = chosen.map(({ line }) => `„${line.description}”`).join(', ')
  return (
    <div className="flex min-h-0 flex-1 flex-col" onKeyDown={onKeyDown}>
      <header className="flex h-16 shrink-0 items-center justify-between gap-4 border-b border-[var(--border)] pl-6 pr-4">
        <div className="min-w-0"><p className="eyebrow">Asociere servicii</p><Dialog.Title className="mt-0.5 truncate text-lg font-bold">Asociază liniile facturii cu serviciile din contract</Dialog.Title></div>
        <Dialog.Close asChild><Button type="button" variant="secondary" size="icon" aria-label="Închide"><X className="size-4" /></Button></Dialog.Close>
      </header>
      <p className="flex shrink-0 items-center gap-2 border-b border-[var(--info-border)] bg-[var(--info-soft)] px-6 py-2.5 text-sm"><Info aria-hidden="true" className="size-4 shrink-0 text-[var(--info)]" /><span>Alege după descriere și unitatea de măsură. <strong>Prețul nu contează aici</strong>: Diana îl compară după confirmare, ca verificare separată.</span></p>
      <div className="grid min-h-0 flex-1 lg:grid-cols-[280px_minmax(0,1fr)_minmax(0,1.1fr)]">
        <section aria-label="Linii din factură" className="flex min-h-0 flex-col border-b border-[var(--border)] lg:border-b-0 lg:border-r">
          <div className="flex h-11 shrink-0 items-center border-b border-[var(--border)] px-4"><span className="eyebrow">Linii de asociat ({mappings.length})</span></div>
          <div className="min-h-0 flex-1 space-y-2.5 overflow-auto p-3">
            {mappings.map(({ line }) => {
              const value = choices[line.id] ?? ''
              const status = !value ? 'Alege un serviciu' : value === 'none' ? 'Nu apare în contract' : value === suggestedId(line.id) ? 'Sugestie acceptată' : 'Ales manual'
              const service = mappings.find((item) => item.line.id === line.id)?.finding.serviceCandidates?.find((candidate) => candidate.ruleId === value)?.label
              const isActive = line.id === active.line.id
              return (
                <button key={line.id} type="button" aria-pressed={isActive} onClick={() => setActiveLineId(line.id)} className={`block w-full rounded-xl p-3.5 text-left ${isActive ? 'border-2 border-[var(--accent)] bg-[var(--info-soft)]' : 'border border-[var(--border)] bg-[var(--surface)] hover:bg-[var(--surface-subtle)]'}`}>
                  <span className="eyebrow">Linia {line.position}</span>
                  <span className="mt-0.5 block text-sm font-semibold">{line.description}</span>
                  {lineItemDescription(line) && <span className="mt-1 block text-xs text-[var(--text-secondary)]">„{lineItemDescription(line)}”</span>}
                  <span className="mt-1.5 block text-xs text-[var(--text-muted)]">UM {unitLabel(line.unit)} · cant. {line.quantity}</span>
                  <span className="mt-2.5 flex items-start gap-1.5 border-t border-[var(--border)] pt-2.5 text-xs">
                    {!value ? <CircleHelp aria-hidden="true" className="mt-px size-4 shrink-0 text-[var(--text-muted)]" /> : value === 'none' ? <TriangleAlert aria-hidden="true" className="mt-px size-4 shrink-0 text-[var(--warning)]" /> : <CircleCheck aria-hidden="true" className={`mt-px size-4 shrink-0 ${value === suggestedId(line.id) ? 'text-[var(--success)]' : 'text-[var(--accent)]'}`} />}
                    <span><span className="block font-semibold">{status}</span>{service && <span className="block text-[var(--text-secondary)]">{service}</span>}</span>
                  </span>
                </button>
              )
            })}
          </div>
        </section>
        <section aria-label="Servicii din contract" className="flex min-h-0 flex-col border-b border-[var(--border)] lg:border-b-0 lg:border-r">
          <div className="flex h-11 shrink-0 items-center justify-between border-b border-[var(--border)] px-4"><span className="eyebrow">Servicii pentru linia {active.line.position}</span><span className="text-xs text-[var(--text-muted)]">ordonate după potrivire</span></div>
          <div role="radiogroup" aria-label={`Serviciul contractual pentru linia ${active.line.position}`} className="min-h-0 flex-1 space-y-2.5 overflow-auto p-3">
            {candidates.map((candidate, index) => {
              const checked = choice === candidate.ruleId
              const page = candidate.evidence?.find((item) => item.page)?.page
              const meta = [candidate.unit ? `UM ${candidate.unit}` : '', frequencyLabels[candidate.billingFrequency ?? ''] ?? '', pricingLabels[candidate.pricingKind ?? ''] ?? ''].filter(Boolean).join(' · ')
              return (
                <label key={candidate.ruleId} className={`flex cursor-pointer items-start gap-3 rounded-xl p-3.5 ${checked ? 'border-2 border-[var(--accent)] bg-[var(--info-soft)]' : 'border border-[var(--border)] bg-[var(--surface)] hover:bg-[var(--surface-subtle)]'}`}>
                  <input type="radio" name={`service-${active.line.id}`} checked={checked} onChange={() => choose(candidate.ruleId)} className="mt-0.5 size-4 shrink-0 accent-[var(--accent)]" />
                  <span className="min-w-0 flex-1">
                    <span className="flex items-start justify-between gap-2"><span className="text-sm font-semibold leading-snug">{candidate.label}</span><Kbd>{String(index + 1)}</Kbd></span>
                    {meta && <span className="mt-0.5 block text-xs text-[var(--text-secondary)]">{meta}</span>}
                    <span className="mt-2 flex flex-wrap gap-1.5">
                      {candidate.suggested && <span className="inline-flex items-center rounded-md bg-[var(--accent)] px-2 py-0.5 text-xs font-semibold text-[var(--accent-contrast)]">Sugestia Diana</span>}
                      {candidateSignals(candidate).map((signal) => <span key={signal.text} className={chip(signal.positive)}>{signal.text}</span>)}
                    </span>
                    {page && <span className="mt-2 block text-xs text-[var(--text-muted)]">Sursa: contract, pagina {page}</span>}
                  </span>
                </label>
              )
            })}
            <label className={`flex cursor-pointer items-start gap-3 rounded-xl p-3.5 ${choice === 'none' ? 'border-2 border-[var(--warning)] bg-[var(--warning-soft)]' : 'border border-[var(--border)] bg-[var(--surface)] hover:bg-[var(--surface-subtle)]'}`}>
              <input type="radio" name={`service-${active.line.id}`} checked={choice === 'none'} onChange={() => choose('none')} className="mt-0.5 size-4 shrink-0 accent-[var(--accent)]" />
              <span className="min-w-0 flex-1">
                <span className="flex items-start justify-between gap-2"><span className="text-sm font-semibold">Serviciul nu apare în contract</span><Kbd>{String(candidates.length + 1)}</Kbd></span>
                <span className="mt-0.5 block text-xs text-[var(--text-secondary)]">Linia rămâne neasociată. Alege mai jos ce faci cu ea.</span>
              </span>
            </label>
            {choice === 'none' && <NotInContractActions invoice={invoice} run={run} finding={active.finding} onDone={onClose} />}
            <p className="hidden flex-wrap items-center gap-3 pt-1 text-xs text-[var(--text-secondary)] md:flex"><span className="flex items-center gap-1"><Kbd>↑</Kbd><Kbd>↓</Kbd> linia</span><span className="flex items-center gap-1"><Kbd>{`1–${candidates.length + 1}`}</Kbd> serviciul</span><span className="flex items-center gap-1"><Kbd>Enter</Kbd> confirmă</span></p>
          </div>
        </section>
        <section aria-label="Contractul original" className="flex min-h-0 flex-col">
          {documentId ? <EvidencePdf key={`${documentId}:${choice}`} clientId={invoice.clientId} documentId={documentId} evidence={selected?.evidence?.filter((item) => item.documentId === documentId) ?? []} /> : <p className="p-6 text-sm text-[var(--text-secondary)]">Documentul contractual nu este disponibil pentru previzualizare.</p>}
        </section>
      </div>
      <footer className="flex shrink-0 flex-wrap items-center justify-between gap-3 border-t border-[var(--border)] px-6 py-3">
        <div className="flex max-w-3xl items-start gap-2.5">
          <input id="remember-service-mapping" type="checkbox" checked={remember} onChange={(event) => setRemember(event.target.checked)} className="mt-0.5 size-4 shrink-0 accent-[var(--accent)]" />
          <div><label htmlFor="remember-service-mapping" className="cursor-pointer text-sm font-semibold">Ține minte pentru facturile viitoare din acest contract</label><p className="mt-0.5 text-xs text-[var(--text-secondary)]">{remember ? `${wordings ? `Formulările ${wordings} vor fi recunoscute automat` : 'Formulările confirmate vor fi recunoscute automat'} pe facturile viitoare din acest contract. Poți revoca oricând din pagina contractului.` : 'Asocierea se aplică doar acestei facturi; pe factura următoare vei fi întrebat din nou.'}</p></div>
        </div>
        <div className="flex gap-2">
          <Dialog.Close asChild><Button type="button" variant="secondary">Anulează</Button></Dialog.Close>
          {chosen.length || !allOutside ? <Button type="button" disabled={!chosen.length || confirm.isPending} onClick={submit}>{confirm.isPending ? 'Se salvează…' : chosen.length > 1 ? `Confirmă toate (${chosen.length}) și revalidează` : chosen.length === 1 ? 'Confirmă asocierea și revalidează' : 'Alege cel puțin un serviciu'}</Button> : <p className="max-w-xs text-xs text-[var(--text-secondary)]">Nicio linie nu are serviciu în contract: cere corecția facturii sau acceptă excepția, mai sus.</p>}
        </div>
        {confirm.isError && <p role="alert" className="w-full text-right text-xs text-[var(--danger)]">Asocierile nu au putut fi salvate integral. Reîncearcă: cele deja salvate nu se dublează.</p>}
      </footer>
    </div>
  )
}

// A line whose service is not in the contract has three real outcomes: the
// supplier corrects the invoice, the reviewer accepts the line as a reasoned
// exception, or the contract gains the service through an annex.
function NotInContractActions({ invoice, run, finding, onDone }: { invoice: Invoice; run: CommercialValidationRun; finding: CommercialFinding; onDone: () => void }) {
  const resolve = useResolveCommercialValidation(invoice)
  const [reason, setReason] = useState('')
  const waiting = invoice.task?.status === 'WAITING'
  const act = (action: 'ACCEPT_EXCEPTION' | 'WAIT_FOR_CORRECTION') => resolve.mutate({ runId: run.id, findingId: finding.id, expectedInvoiceRevision: invoice.revision ?? run.invoiceRevision + 1, action, reason: action === 'ACCEPT_EXCEPTION' ? reason.trim() : undefined }, { onSuccess: onDone })
  return (
    <div role="group" aria-label="Ce faci cu linia care nu apare în contract" className="space-y-3 rounded-xl border border-[var(--warning-border)] bg-[var(--surface)] p-3.5 text-xs">
      <div>
        <p className="font-semibold">Furnizorul a facturat greșit?</p>
        <p className="mt-0.5 text-[var(--text-secondary)]">Factura rămâne în așteptare până sosește factura corectată.</p>
        {waiting ? <p className="mt-1.5 text-[var(--text-muted)]">Factura așteaptă deja corecția.</p> : <Button type="button" variant="secondary" size="sm" className="mt-1.5" disabled={resolve.isPending} onClick={() => act('WAIT_FOR_CORRECTION')}>Cere corecția facturii</Button>}
      </div>
      <div className="border-t border-[var(--border)] pt-3">
        <p className="font-semibold">Serviciul este corect, dar nu apare în contract?</p>
        <p className="mt-0.5 text-[var(--text-secondary)]">Acceptă linia ca excepție, cu motiv. Celelalte verificări deschise ale facturii rămân de rezolvat.</p>
        <div className="mt-1.5 flex flex-wrap gap-2">
          <label className="sr-only" htmlFor={`exception-${finding.id}`}>Motivul excepției</label>
          <input id={`exception-${finding.id}`} value={reason} onChange={(event) => setReason(event.target.value)} placeholder="Motiv obligatoriu" className="h-9 min-w-48 flex-1 rounded-lg border border-[var(--border-strong)] bg-[var(--surface)] px-3 text-sm" />
          <Button type="button" variant="secondary" size="sm" disabled={!reason.trim() || resolve.isPending} onClick={() => act('ACCEPT_EXCEPTION')}>Acceptă excepția</Button>
        </div>
      </div>
      <div className="border-t border-[var(--border)] pt-3">
        <p className="font-semibold">Serviciul a fost adăugat printr-un act adițional?</p>
        <p className="mt-0.5 text-[var(--text-secondary)]">Încarcă documentul, confirmă-l, apoi revino și rulează din nou validarea.</p>
        <Link className="mt-1.5 inline-block font-semibold text-[var(--accent)] underline" to={`/contracts/upload?clientId=${encodeURIComponent(invoice.clientId)}&invoiceId=${encodeURIComponent(invoice.id)}`}>Încarcă anexa cu acest serviciu</Link>
      </div>
      {resolve.isError && <p role="alert" className="text-[var(--danger)]">Decizia nu a putut fi salvată.</p>}
    </div>
  )
}
