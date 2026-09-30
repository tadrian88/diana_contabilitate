import { ChevronRight, CircleCheck, CircleDashed, CircleHelp, TriangleAlert } from 'lucide-react'
import { lazy, Suspense, useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { useSearchParams } from 'react-router-dom'
import { Button } from '../../components/ui/button'
import type { ContractDocument } from '../../repositories/invoiceRepository'
import type { HighlightResult } from './ContractPDFPreview'
import { findItem, reviewGroups, type ReviewGroup, type ReviewItem, type ReviewTone } from './contract-review-model'
import type { ContractPdf } from './useContractPdf'

// pdf.js is loaded with the first contract page shown, not with the route.
const ContractPDFPreview = lazy(() => import('./ContractPDFPreview').then((module) => ({ default: module.ContractPDFPreview })))

export const Kbd = ({ children }: { children: string }) => <kbd className="inline-flex h-[22px] min-w-[22px] items-center justify-center rounded-md border border-b-2 border-[var(--border-strong)] bg-[var(--surface)] px-1.5 font-sans text-[11px] font-semibold text-[var(--text-secondary)]">{children}</kbd>

// Shortcuts never fire while the reviewer is typing or choosing a value.
const editable = (target: EventTarget | null) => target instanceof HTMLElement && (['INPUT', 'SELECT', 'TEXTAREA'].includes(target.tagName) || target.isContentEditable)

const toneColor: Record<ReviewTone, string> = { ok: 'text-[var(--success)]', attention: 'text-[var(--warning)]', closed: 'text-[var(--text-muted)]', unknown: 'text-[var(--text-muted)]' }

function ToneIcon({ tone }: { tone: ReviewTone }) {
  const className = `mt-0.5 size-4 shrink-0 ${toneColor[tone]}`
  if (tone === 'ok') return <CircleCheck aria-hidden="true" className={className} />
  if (tone === 'attention') return <TriangleAlert aria-hidden="true" className={className} />
  if (tone === 'closed') return <CircleDashed aria-hidden="true" className={className} />
  return <CircleHelp aria-hidden="true" className={className} />
}

// The contract review page: what Diana read on the left, grouped like the
// contract, and the original PDF on the right, highlighted at the words the
// active item was read from, so each value is checked without searching.
// ↑/↓ move between items; the page keeps the active item in `?element=`.
export function ContractReviewWorkspace({ document, items, pdf, eyebrow, title, subtitle, badges, actions, nextLabel, renderDetail, onShortcut, keyHints, notice, groupActions, afterList, footer }: {
  document: ContractDocument
  items: ReviewItem[]
  pdf: ContractPdf
  eyebrow: string
  title: string
  subtitle?: ReactNode
  badges?: ReactNode
  actions?: ReactNode
  // Label of the button that jumps to the next item needing attention.
  nextLabel: (count: number) => string
  renderDetail: (item: ReviewItem) => ReactNode
  onShortcut?: (event: KeyboardEvent, item: ReviewItem) => void
  // The shortcuts onShortcut handles, next to the ↑/↓ hint.
  keyHints?: ReactNode
  // Shown between the header and the two panes (blockers, errors).
  notice?: ReactNode
  // An action in a group's heading, e.g. adding a service; the group is shown
  // even while it has no item.
  groupActions?: Partial<Record<ReviewGroup, ReactNode>>
  afterList?: ReactNode
  footer?: ReactNode
}) {
  const [params, setParams] = useSearchParams()
  const requested = findItem(items, params.get('element'))
  const [activeId, setActiveId] = useState(() => requested?.id ?? items.find((item) => item.tone === 'attention')?.id ?? items[0]?.id)
  const active = items.find((item) => item.id === activeId) ?? items[0]
  const rows = useRef(new Map<string, HTMLButtonElement>())
  const pdfPane = useRef<HTMLElement>(null)
  const attention = items.filter((item) => item.tone === 'attention')
  const [page, setPage] = useState(active?.page ?? 1)
  const [located, setLocated] = useState<{ id: string; result: HighlightResult }>()

  const select = useCallback((item: ReviewItem, via: 'keyboard' | 'pointer' | 'link') => {
    setActiveId(item.id)
    setPage(item.page ?? 1)
    setParams((current) => { const next = new URLSearchParams(current); next.set('element', item.id); return next }, { replace: true })
    if (via === 'keyboard') rows.current.get(item.id)?.focus({ preventScroll: true })
    if (via === 'pointer' && typeof window.matchMedia === 'function' && window.matchMedia('(max-width: 1023px)').matches) pdfPane.current?.scrollIntoView?.({ behavior: 'smooth', block: 'start' })
  }, [setParams])

  // A link to another element of the same page (an invoice check, the next
  // item to resolve) moves the selection without a reload.
  const requestedId = requested?.id
  useEffect(() => {
    if (requestedId && requestedId !== activeId) {
      const item = items.find((candidate) => candidate.id === requestedId)
      if (item) { setActiveId(item.id); setPage(item.page ?? 1) }
    }
    // Only a changed address moves the selection; selecting updates it too.
  }, [requestedId])

  useEffect(() => { if (active) rows.current.get(active.id)?.scrollIntoView?.({ block: 'nearest' }) }, [active])

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.defaultPrevented || event.altKey || event.ctrlKey || event.metaKey || editable(event.target) || !active) return
      const index = items.findIndex((item) => item.id === active.id)
      if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
        const target = items[index + (event.key === 'ArrowDown' ? 1 : -1)]
        event.preventDefault()
        if (target) select(target, 'keyboard')
        return
      }
      onShortcut?.(event, active)
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [items, active, select, onShortcut])

  const nextAttention = useMemo(() => {
    const index = items.findIndex((item) => item.id === active?.id)
    return [...items.slice(index + 1), ...items.slice(0, index + 1)].find((item) => item.tone === 'attention' && item.id !== active?.id)
  }, [items, active])

  const result = located?.id === active?.id ? located.result : undefined
  const locationText = !active?.highlights.length ? 'Acest element nu citează un fragment din PDF.' : pdf.scanned ? 'PDF fără text: verifică vizual pe pagina deschisă.' : !result ? 'Se caută textul în PDF…' : !result.found ? 'Textul citat nu a fost găsit pe această pagină.' : result.exact ? 'Găsit exact în PDF și evidențiat.' : 'Găsit în PDF; textul se întinde pe mai multe rânduri, a fost evidențiat începutul lui.'

  return (
    <div className="space-y-4">
      <header className="card flex flex-wrap items-center justify-between gap-4 px-5 py-4">
        <div className="min-w-0">
          <p className="eyebrow">{eyebrow}</p>
          <h2 className="mt-0.5 truncate text-xl font-bold">{title}</h2>
          {subtitle && <div className="mt-0.5 text-sm text-[var(--text-secondary)]">{subtitle}</div>}
        </div>
        <div className="flex flex-wrap items-center gap-2">
          {badges}
          {nextAttention && <Button type="button" variant="secondary" onClick={() => select(nextAttention, 'link')}>{nextLabel(attention.length)}<ChevronRight aria-hidden="true" className="ml-1 size-4" /></Button>}
          {actions}
        </div>
      </header>
      {notice}
      {pdf.scanned && <p role="status" className="rounded-lg border border-[var(--warning-border)] bg-[var(--warning-soft)] px-4 py-2.5 text-sm">PDF-ul nu are text (este scanat): Diana nu poate evidenția valorile. Verifică fiecare element vizual, pe pagina indicată.</p>}
      <div className="grid gap-4 lg:h-[calc(100vh-15rem)] lg:min-h-[560px] lg:grid-cols-[minmax(0,0.85fr)_minmax(0,1.15fr)]">
        <section aria-label="Ce a extras Diana" className="card flex min-h-0 flex-col overflow-hidden">
          <div className="flex h-11 shrink-0 items-center justify-between border-b border-[var(--border)] px-4"><span className="eyebrow">Ce a extras Diana</span><span className="hidden items-center gap-1 text-xs text-[var(--text-secondary)] md:flex"><Kbd>↑</Kbd><Kbd>↓</Kbd> elementul{keyHints}</span></div>
          <div className="min-h-0 flex-1 overflow-auto">
            {reviewGroups.map((group) => {
              const members = items.filter((item) => item.group === group.id)
              const action = groupActions?.[group.id]
              if (!members.length && !action) return null
              return (
                <section key={group.id} aria-label={group.label} className="border-b border-[var(--border)] last:border-b-0">
                  <div className="flex items-center justify-between gap-2 bg-[var(--surface-subtle)] px-4 py-2"><h3 className="eyebrow">{group.label} <span className="text-[var(--text-muted)]">({members.length})</span></h3>{action}</div>
                  <ul>
                    {members.map((item) => {
                      const isActive = item.id === active?.id
                      return (
                        <li key={item.id} className={isActive ? 'bg-[var(--info-soft)] shadow-[inset_3px_0_0_var(--accent)]' : 'border-t border-[var(--border)] first:border-t-0'}>
                          <button type="button" data-review-row="" ref={(element) => { if (element) rows.current.set(item.id, element); else rows.current.delete(item.id) }} aria-current={isActive ? 'true' : undefined} onClick={() => select(item, 'pointer')} className="flex w-full items-start gap-3 px-4 py-3 text-left outline-none hover:bg-[var(--surface-subtle)] focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-[var(--focus)]">
                            <ToneIcon tone={item.tone} />
                            <span className="min-w-0 flex-1">
                              <span className="flex items-baseline justify-between gap-3"><span className="text-xs font-semibold text-[var(--text-secondary)]">{item.label}</span>{item.page && <span className="shrink-0 text-xs text-[var(--text-muted)]">p. {item.page}</span>}</span>
                              <span className="mt-0.5 block text-sm font-semibold leading-snug">{item.value}</span>
                              <span className={`mt-0.5 block text-xs ${toneColor[item.tone]}`}>{item.status}</span>
                            </span>
                          </button>
                          {isActive && (
                            <div className="space-y-3 px-4 pb-4 pl-11 text-sm">
                              {item.highlights.length > 0 && <blockquote className="border-l-2 border-[var(--evidence-ring)] pl-3 text-xs text-[var(--text-secondary)]">{item.highlights.map((snippet) => `„${snippet}”`).join(' · ')}</blockquote>}
                              {item.detail && <p className="text-xs text-[var(--text-secondary)]">{item.detail}</p>}
                              {renderDetail(item)}
                            </div>
                          )}
                        </li>
                      )
                    })}
                  </ul>
                </section>
              )
            })}
            {afterList}
          </div>
        </section>
        <section ref={pdfPane} aria-label="Contractul original" className="card flex min-h-[70vh] flex-col overflow-hidden lg:min-h-0">
          <div className="shrink-0 border-b border-[var(--border)] px-5 py-3">
            <p className="eyebrow">{active ? `${reviewGroups.find((group) => group.id === active.group)?.label} · ${active.label}` : 'Contractul original'}</p>
            {active && <p className="mt-0.5 truncate text-sm font-semibold">{active.value}</p>}
            <p role="status" className={`mt-0.5 text-xs ${result && !result.found ? 'text-[var(--warning)]' : 'text-[var(--text-secondary)]'}`}>{locationText}</p>
          </div>
          <Suspense fallback={<p role="status" className="p-6 text-sm text-[var(--text-secondary)]">Se încarcă PDF-ul…</p>}>
            <ContractPDFPreview key={document.id} clientId={document.clientId} documentId={document.id} page={page} onPage={setPage} document={pdf.pdf} documentError={pdf.error} onRetry={pdf.retry} highlights={active?.highlights} locateAcrossPages={!active?.page} onHighlight={(value) => active && setLocated({ id: active.id, result: value })} className="flex min-h-0 flex-1 flex-col" viewportClassName="min-h-0 flex-1" />
          </Suspense>
        </section>
      </div>
      {footer}
    </div>
  )
}
