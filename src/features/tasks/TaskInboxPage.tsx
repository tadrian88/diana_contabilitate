import { AlertCircle, ArrowRight, CheckCircle2, Clock3, Inbox, RotateCcw, SearchX } from 'lucide-react'
import { useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { Badge } from '../../components/ui/badge'
import { Button } from '../../components/ui/button'
import type { TaskStatus, TaskType } from '../../domain/invoice'
import { PIPELINE_LABELS, TASK_STATUS_LABELS, TASK_TYPE_LABELS } from '../../domain/invoice'
import { useRequestContract } from '../invoices/invoice-mutations'
import { useTaskInbox, type TaskInboxItem } from './task-inbox-hooks'

type TypeFilter = 'ALL' | TaskType

export function TaskInboxPage() {
  const [searchParams, setSearchParams] = useSearchParams()
  const { items, isLoading, isError } = useTaskInbox()
  const [notice, setNotice] = useState('')
  const status = parseStatus(searchParams.get('status'))
  const type = parseType(searchParams.get('type'))
  const openCount = items.filter((item) => item.task.status === 'OPEN').length
  const waitingCount = items.filter((item) => item.task.status === 'WAITING').length
  const statusItems = items.filter((item) => item.task.status === status)
  const visibleItems = statusItems.filter((item) => type === 'ALL' || item.task.type === type)

  const updateFilters = (next: { status?: TaskStatus; type?: TypeFilter }) => {
    const params = new URLSearchParams(searchParams)
    params.set('status', next.status ?? status)
    params.set('type', next.type ?? type)
    setSearchParams(params)
  }

  return (
    <div className="space-y-6">
      <section className="flex items-end justify-between gap-6">
        <div>
          <p className="eyebrow">Coadă operațională</p>
          <h2 className="mt-2 text-2xl font-bold tracking-tight">Task Inbox</h2>
          <p className="mt-2 max-w-2xl text-sm text-[var(--text-secondary)]">Doar facturile care necesită judecata contabilului. Facturile fără probleme nu apar aici.</p>
        </div>
        <div className="flex gap-3">
          <CountCard label="Deschise" value={openCount} tone="warning" />
          <CountCard label="În așteptare" value={waitingCount} tone="info" />
        </div>
      </section>

      <section className="card overflow-hidden" aria-labelledby="task-list-heading">
        <div className="flex items-center justify-between border-b border-[var(--border)] px-5">
          <div className="flex self-stretch" role="tablist" aria-label="Status task">
            <StatusTab label="Deschise" status="OPEN" active={status === 'OPEN'} count={openCount} onSelect={() => updateFilters({ status: 'OPEN' })} />
            <StatusTab label="În așteptare" status="WAITING" active={status === 'WAITING'} count={waitingCount} onSelect={() => updateFilters({ status: 'WAITING' })} />
            <StatusTab label="Rezolvate" status="RESOLVED" active={status === 'RESOLVED'} onSelect={() => updateFilters({ status: 'RESOLVED' })} />
          </div>
          <label className="flex items-center gap-2 text-xs font-semibold text-[var(--text-secondary)]">
            Tip task
            <select aria-label="Filtrează după tipul task-ului" value={type} onChange={(event) => updateFilters({ type: event.target.value as TypeFilter })} className="h-9 rounded-lg border border-[var(--border-strong)] bg-[var(--surface)] px-3 text-sm text-[var(--text)] outline-none focus:ring-2 focus:ring-[var(--focus)]">
              <option value="ALL">Toate tipurile</option>
              <option value="CONTRACT_MATCH">Verificare contract</option>
              <option value="MISSING_CONTRACT">Contract lipsă</option>
              <option value="CLASSIFICATION">Revizuire clasificare</option>
            </select>
          </label>
        </div>

        {notice && <div className="border-b border-[var(--success-border)] bg-[var(--success-soft)] px-5 py-3 text-sm font-medium text-[var(--success)]" role="status">{notice}</div>}
        {isLoading ? <LoadingState /> : isError ? <ErrorState /> : visibleItems.length === 0 ? <EmptyState status={status} filtered={statusItems.length > 0} onReset={() => updateFilters({ type: 'ALL' })} /> : (
          <div className="overflow-x-auto">
            <table className="w-full border-collapse text-left" aria-describedby="task-list-description">
              <caption id="task-list-description" className="sr-only">Task-uri contabile filtrate după status și tip.</caption>
              <thead className="bg-[var(--surface-subtle)] text-[11px] font-bold uppercase tracking-wider text-[var(--text-muted)]">
                <tr><th className="px-5 py-3">Task și motiv</th><th className="px-4 py-3">Factură</th><th className="px-4 py-3">Client</th><th className="px-4 py-3">Context decizie</th><th className="px-4 py-3">Status</th><th className="px-5 py-3 text-right">Acțiune</th></tr>
              </thead>
              <tbody className="divide-y divide-[var(--border)]">
                {visibleItems.map((item) => <TaskRow key={item.task.id} item={item} returnTo={buildReturnPath(status, type)} onRequested={() => setNotice('Contractul a fost solicitat. Task-ul rămâne vizibil în „În așteptare”.')} />)}
              </tbody>
            </table>
          </div>
        )}
      </section>
    </div>
  )
}

function TaskRow({ item, returnTo, onRequested }: { item: TaskInboxItem; returnTo: string; onRequested: () => void }) {
  const { task, invoice, client } = item
  const requestContract = useRequestContract(invoice.id)
  const destination = task.type === 'CLASSIFICATION' ? 'classification' : 'contract'
  const href = `/invoices/${invoice.id}?tab=${destination}&returnTo=${encodeURIComponent(returnTo)}`
  const pendingItems = task.classificationItems?.filter((review) => review.status === 'PENDING').length ?? 0
  const recommended = task.contractCandidates?.find((candidate) => candidate.recommended)

  return (
    <tr className="group align-top hover:bg-[var(--surface-subtle)]">
      <td className="px-5 py-4">
        <Link to={href} className="font-semibold text-[var(--text)] outline-none hover:text-[var(--accent)] focus-visible:rounded focus-visible:ring-2 focus-visible:ring-[var(--focus)]">{TASK_TYPE_LABELS[task.type]}</Link>
        <p className="mt-1 max-w-[300px] text-xs leading-5 text-[var(--text-secondary)]">{task.reason}</p>
        <div className="mt-2 flex items-center gap-1.5 text-[11px] text-[var(--text-muted)]"><Clock3 className="size-3" />{task.status === 'WAITING' && task.waitingSince ? `În așteptare din ${formatDateTime(task.waitingSince)}` : `Creat ${formatDateTime(task.createdAt)}`}</div>
      </td>
      <td className="px-4 py-4 text-sm">
        <div className="font-semibold">{invoice.documentNumber}</div>
        <div className="mt-1 text-xs text-[var(--text-secondary)]">{invoice.supplierName}</div>
        <div className="mt-2 text-xs"><span className="font-semibold">{formatMoney(invoice.total.amount, invoice.total.currency)}</span><span className="text-[var(--text-muted)]"> · {formatDate(invoice.issueDate)}</span></div>
      </td>
      <td className="px-4 py-4"><div className="max-w-[180px] text-sm font-medium">{client?.name}</div><div className="mt-1 text-[11px] text-[var(--text-muted)]">{client?.cui}</div></td>
      <td className="px-4 py-4 text-xs">
        <Badge tone="neutral">{PIPELINE_LABELS[invoice.pipelineStatus]}</Badge>
        {task.type === 'CONTRACT_MATCH' && recommended && <div className="mt-2 max-w-[230px]"><strong>{recommended.reference}</strong><span className="block text-[var(--text-secondary)]">Încredere: {recommended.confidence}</span><span className="mt-1 block text-[var(--text-muted)]">{recommended.reasons[0]}{(task.contractCandidates?.length ?? 0) > 1 ? ` · ${(task.contractCandidates?.length ?? 1) - 1} alternativă` : ''}</span></div>}
        {task.type === 'CLASSIFICATION' && <div className="mt-2 text-[var(--text-secondary)]"><strong className="text-[var(--text)]">{pendingItems}</strong> elemente incerte necesită revizuire.</div>}
        {task.type === 'MISSING_CONTRACT' && <div className="mt-2 text-[var(--text-secondary)]">Factura așteaptă o condiție externă.</div>}
        {task.type === 'COMMERCIAL_REVIEW' && <div className="mt-2 text-[var(--text-secondary)]">Verifică findings, datele lipsă și excepțiile în tab-ul Contract.</div>}
      </td>
      <td className="px-4 py-4"><Badge tone={task.status === 'OPEN' ? 'warning' : task.status === 'WAITING' ? 'info' : 'success'}>{TASK_STATUS_LABELS[task.status]}</Badge></td>
      <td className="px-5 py-4 text-right">
        {task.type === 'MISSING_CONTRACT' && task.status === 'OPEN' ? (
          <Button size="sm" onClick={() => requestContract.mutate(undefined, { onSuccess: onRequested })} disabled={requestContract.isPending}>Solicită contract</Button>
        ) : (
          <Link to={href} className="inline-flex h-9 items-center gap-2 rounded-lg px-3 text-xs font-semibold text-[var(--accent)] outline-none hover:bg-[var(--info-soft)] focus-visible:ring-2 focus-visible:ring-[var(--focus)]">{task.status === 'OPEN' ? 'Rezolvă' : 'Vezi context'}<ArrowRight className="size-3.5" /></Link>
        )}
      </td>
    </tr>
  )
}

function StatusTab({ label, status, active, count, onSelect }: { label: string; status: TaskStatus; active: boolean; count?: number; onSelect: () => void }) {
  return <button role="tab" aria-selected={active} data-status={status} onClick={onSelect} className={`border-b-2 px-4 py-4 text-sm font-semibold outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-[var(--focus)] ${active ? 'border-[var(--accent)] text-[var(--accent)]' : 'border-transparent text-[var(--text-secondary)] hover:text-[var(--text)]'}`}>{label}{count !== undefined && <span className="ml-2 rounded-full bg-[var(--surface-subtle)] px-2 py-0.5 text-xs tabular-nums">{count}</span>}</button>
}

function CountCard({ label, value, tone }: { label: string; value: number; tone: 'warning' | 'info' }) {
  return <div className={`min-w-32 rounded-xl border px-4 py-3 ${tone === 'warning' ? 'border-[var(--warning-border)] bg-[var(--warning-soft)]' : 'border-[var(--info-border)] bg-[var(--info-soft)]'}`}><div className="text-xs font-semibold text-[var(--text-secondary)]">{label}</div><div className={`mt-1 text-2xl font-bold tabular-nums ${tone === 'warning' ? 'text-[var(--warning)]' : 'text-[var(--info)]'}`}>{value}</div></div>
}

function LoadingState() { return <div className="space-y-3 p-5" aria-label="Se încarcă task-urile">{[1, 2, 3].map((item) => <div key={item} className="h-20 animate-pulse rounded-lg bg-[var(--surface-subtle)]" />)}</div> }
function ErrorState() { return <div className="p-12 text-center"><AlertCircle className="mx-auto size-7 text-[var(--danger)]" /><h3 className="mt-3 font-bold">Task-urile nu au putut fi încărcate</h3><p className="mt-1 text-sm text-[var(--text-secondary)]">Serviciul de date nu este disponibil. Verifică starea API-ului.</p></div> }
function EmptyState({ status, filtered, onReset }: { status: TaskStatus; filtered: boolean; onReset: () => void }) {
  if (filtered) return <div className="p-12 text-center"><SearchX className="mx-auto size-7 text-[var(--text-muted)]" /><h3 className="mt-3 font-bold">Niciun rezultat pentru filtrul curent</h3><Button className="mt-4" variant="secondary" size="sm" onClick={onReset}><RotateCcw className="size-3.5" />Resetează filtrul</Button></div>
  if (status === 'OPEN') return <div className="p-12 text-center"><CheckCircle2 className="mx-auto size-8 text-[var(--success)]" /><h3 className="mt-3 font-bold">Totul este în regulă</h3><p className="mt-1 text-sm text-[var(--text-secondary)]">Nu există facturi care necesită acum decizia contabilului.</p></div>
  return <div className="p-12 text-center"><Inbox className="mx-auto size-8 text-[var(--text-muted)]" /><h3 className="mt-3 font-bold">Nu există task-uri {status === 'WAITING' ? 'în așteptare' : 'rezolvate'}</h3></div>
}

function parseStatus(value: string | null): TaskStatus { return value === 'WAITING' || value === 'RESOLVED' ? value : 'OPEN' }
function parseType(value: string | null): TypeFilter { return value === 'CONTRACT_MATCH' || value === 'MISSING_CONTRACT' || value === 'CLASSIFICATION' ? value : 'ALL' }
function buildReturnPath(status: TaskStatus, type: TypeFilter) { return `/tasks?status=${status}&type=${type}` }
function formatMoney(amount: number, currency: string) { return new Intl.NumberFormat('ro-RO', { style: 'currency', currency }).format(amount) }
function formatDate(value: string) { return new Intl.DateTimeFormat('ro-RO', { dateStyle: 'medium', timeZone: 'Europe/Bucharest' }).format(new Date(value)) }
function formatDateTime(value: string) { return new Intl.DateTimeFormat('ro-RO', { dateStyle: 'medium', timeStyle: 'short', timeZone: 'Europe/Bucharest' }).format(new Date(value)) }
