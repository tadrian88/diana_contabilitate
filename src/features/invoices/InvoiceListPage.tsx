import { ArrowDown, ArrowUp, ArrowUpDown, CheckCircle2, Search, SearchX, TriangleAlert } from 'lucide-react'
import { useMemo } from 'react'
import type { ReactNode } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { useClientScope } from '../../app/scope-context'
import { Badge } from '../../components/ui/badge'
import type { Invoice, PipelineStatus, SagaStatus } from '../../domain/invoice'
import { PIPELINE_LABELS } from '../../domain/invoice'
import { AttentionBadge, PipelineBadge, SagaBadge } from './InvoiceStatusBadges'
import { useClients, useInvoices } from './invoice-hooks'
import { getAttentionStatus, getInvoiceConfidence, getUnresolvedIssueCount, SAGA_LABELS } from './invoice-view'

type SortKey = 'supplier' | 'number' | 'date' | 'value' | 'pipeline'
type SortDirection = 'asc' | 'desc'
type AttentionFilter = 'ALL' | 'REQUIRED' | 'CLEAR'

const pipelineOptions: PipelineStatus[] = ['DOWNLOADED', 'ARCHIVED', 'MATCHING', 'AWAITING_CONTRACT', 'AWAITING_MATCH_CONFIRM', 'DEDUPE_CHECKED', 'HEADER_READ', 'LINES_READ', 'CLASSIFIED', 'AWAITING_REVIEW', 'READY_FOR_SAGA', 'EXPORTING', 'EXPORTED', 'DUPLICATE']
const sagaOptions: SagaStatus[] = ['NOT_READY', 'READY', 'EXPORTING', 'EXPORTED', 'FAILED']

export function InvoiceListPage() {
  const { scope } = useClientScope()
  const { data: invoices = [], isLoading, isError } = useInvoices(scope)
  const { data: clients = [] } = useClients()
  const [params, setParams] = useSearchParams()
  const query = params.get('q') ?? ''
  const pipeline = parsePipeline(params.get('pipeline'))
  const saga = parseSaga(params.get('saga'))
  const attention = parseAttention(params.get('attention'))
  const sort = parseSort(params.get('sort'))
  const direction: SortDirection = params.get('direction') === 'asc' ? 'asc' : 'desc'

  const visibleInvoices = useMemo(() => {
    const normalizedQuery = query.trim().toLocaleLowerCase('ro-RO')
    const filtered = invoices.filter((invoice) => {
      if (normalizedQuery && !`${invoice.supplierName} ${invoice.documentNumber}`.toLocaleLowerCase('ro-RO').includes(normalizedQuery)) return false
      if (pipeline !== 'ALL' && invoice.pipelineStatus !== pipeline) return false
      if (saga !== 'ALL' && invoice.sagaStatus !== saga) return false
      const hasAttention = getAttentionStatus(invoice) !== 'CLEAR'
      if (attention === 'REQUIRED' && !hasAttention) return false
      if (attention === 'CLEAR' && hasAttention) return false
      return true
    })
    return filtered.sort((left, right) => compareInvoices(left, right, sort, direction))
  }, [attention, direction, invoices, pipeline, query, saga, sort])

  const update = (key: string, value: string) => {
    const next = new URLSearchParams(params)
    value && value !== 'ALL' ? next.set(key, value) : next.delete(key)
    setParams(next)
  }

  const updateSort = (key: SortKey) => {
    const next = new URLSearchParams(params)
    next.set('sort', key)
    next.set('direction', sort === key && direction === 'asc' ? 'desc' : 'asc')
    setParams(next)
  }

  const returnTo = `/invoices${params.toString() ? `?${params.toString()}` : ''}`

  return (
    <div className="space-y-6">
      <section className="flex items-end justify-between gap-6">
        <div><p className="eyebrow">Spațiu operațional</p><h2 className="mt-2 text-2xl font-bold tracking-tight">Facturi</h2><p className="mt-2 text-sm text-[var(--text-secondary)]">Găsește rapid o factură și înțelege starea procesării fără revizuire inutilă.</p></div>
        <div className="text-right"><div className="text-2xl font-bold tabular-nums">{visibleInvoices.length}</div><div className="text-xs text-[var(--text-muted)]">din {invoices.length} facturi în context</div></div>
      </section>

      <section className="card overflow-hidden">
        <div className="flex items-center gap-3 border-b border-[var(--border)] p-4" aria-label="Filtre facturi">
          <label className="relative min-w-[300px] flex-1"><span className="sr-only">Caută după furnizor sau număr factură</span><Search className="absolute left-3 top-1/2 size-4 -translate-y-1/2 text-[var(--text-muted)]" /><input value={query} onChange={(event) => update('q', event.target.value)} placeholder="Caută furnizor sau număr factură…" className="h-10 w-full rounded-lg border border-[var(--border-strong)] bg-[var(--surface)] pl-10 pr-3 text-sm outline-none focus:ring-2 focus:ring-[var(--focus)]" /></label>
          <FilterSelect label="Status pipeline" value={pipeline} onChange={(value) => update('pipeline', value)}><option value="ALL">Toate stările</option>{pipelineOptions.map((value) => <option key={value} value={value}>{pipelineLabel(value)}</option>)}</FilterSelect>
          <FilterSelect label="Atenție" value={attention} onChange={(value) => update('attention', value)}><option value="ALL">Orice atenție</option><option value="REQUIRED">Necesită atenție</option><option value="CLEAR">Fără intervenție</option></FilterSelect>
          <FilterSelect label="Status SAGA" value={saga} onChange={(value) => update('saga', value)}><option value="ALL">Orice status SAGA</option>{sagaOptions.map((value) => <option key={value} value={value}>{sagaLabel(value)}</option>)}</FilterSelect>
        </div>

        {isLoading ? <LoadingState /> : isError ? <ErrorState /> : invoices.length === 0 ? <EmptyState /> : visibleInvoices.length === 0 ? <NoResults onReset={() => setParams({})} /> : (
          <div className="overflow-x-auto">
            <table className="w-full min-w-[1380px] border-collapse text-left text-xs">
              <caption className="sr-only">Lista facturilor din contextul de client activ.</caption>
              <thead className="bg-[var(--surface-subtle)] text-[10px] font-bold uppercase tracking-wider text-[var(--text-muted)]">
                <tr>
                  <SortableHeader label="Furnizor" sortKey="supplier" current={sort} direction={direction} onSort={updateSort} />
                  <SortableHeader label="Număr factură" sortKey="number" current={sort} direction={direction} onSort={updateSort} />
                  <th className="px-3 py-3">Client</th>
                  <SortableHeader label="Valoare" sortKey="value" current={sort} direction={direction} onSort={updateSort} />
                  <SortableHeader label="Dată" sortKey="date" current={sort} direction={direction} onSort={updateSort} />
                  <th className="px-3 py-3">Contract</th>
                  <SortableHeader label="Status pipeline" sortKey="pipeline" current={sort} direction={direction} onSort={updateSort} />
                  <th className="px-3 py-3">Număr probleme</th>
                  <th className="px-3 py-3">Încredere</th>
                  <th className="px-3 py-3">Status SAGA</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-[var(--border)]">
                {visibleInvoices.map((invoice) => {
                  const client = clients.find((candidate) => candidate.id === invoice.clientId)
                  const issueCount = getUnresolvedIssueCount(invoice)
                  const confidence = getInvoiceConfidence(invoice)
                  const href = `/invoices/${invoice.id}?tab=summary&returnTo=${encodeURIComponent(returnTo)}`
                  return (
                    <tr key={invoice.id} className="align-middle hover:bg-[var(--surface-subtle)]" data-invoice-id={invoice.id}>
                      <td className="max-w-[190px] px-3 py-4"><div className="truncate font-semibold text-sm">{invoice.supplierName}</div><div className="mt-1 truncate text-[var(--text-muted)]">{invoice.supplierCui ?? 'CUI indisponibil'}</div></td>
                      <td className="px-3 py-4"><Link to={href} className="font-bold text-[var(--accent)] outline-none hover:underline focus-visible:rounded focus-visible:ring-2 focus-visible:ring-[var(--focus)]">{invoice.documentNumber}</Link></td>
                      <td className="max-w-[170px] px-3 py-4"><span className="line-clamp-2 font-medium">{client?.name ?? 'Client indisponibil'}</span></td>
                      <td className="whitespace-nowrap px-3 py-4 font-semibold tabular-nums">{formatMoney(invoice.total.amount, invoice.total.currency)}</td>
                      <td className="whitespace-nowrap px-3 py-4">{formatDate(invoice.issueDate)}</td>
                      <td className="max-w-[170px] px-3 py-4"><ContractCell invoice={invoice} /></td>
                      <td className="px-3 py-4"><div className="space-y-1.5"><PipelineBadge status={invoice.pipelineStatus} />{getAttentionStatus(invoice) !== 'CLEAR' && <div><AttentionBadge invoice={invoice} /></div>}</div></td>
                      <td className="px-3 py-4"><span className={`inline-flex min-w-7 justify-center rounded-full px-2 py-1 font-bold tabular-nums ${issueCount ? 'bg-[var(--warning-soft)] text-[var(--warning)]' : 'bg-[var(--surface-subtle)] text-[var(--text-secondary)]'}`}>{issueCount}</span></td>
                      <td className="max-w-[150px] px-3 py-4">{confidence ? <span className="font-medium">{confidence}</span> : <span className="text-[var(--text-muted)]">Nu se aplică</span>}</td>
                      <td className="px-3 py-4"><SagaBadge status={invoice.sagaStatus} /></td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        )}
      </section>
    </div>
  )
}

function ContractCell({ invoice }: { invoice: Invoice }) {
  if (invoice.contract) return <div><div className="font-semibold">{invoice.contract.reference}</div><div className="mt-1 text-[var(--text-muted)]">Contract asociat</div></div>
  if (invoice.task?.type === 'CONTRACT_MATCH') return <div><Badge tone="warning">Necesită validare</Badge><div className="mt-1 text-[var(--text-muted)]">{invoice.task.contractCandidates?.length ?? 0} {invoice.task.contractCandidates?.length === 1 ? 'candidat' : 'candidați'}</div></div>
  if (invoice.task?.type === 'MISSING_CONTRACT') return <div><Badge tone={invoice.task.status === 'WAITING' ? 'info' : 'warning'}>Contract lipsă</Badge><div className="mt-1 text-[var(--text-muted)]">{invoice.task.status === 'WAITING' ? 'Solicitat, în așteptare' : 'Acțiune necesară'}</div></div>
  return <span className="text-[var(--text-muted)]">Nu se aplică</span>
}

function SortableHeader({ label, sortKey, current, direction, onSort }: { label: string; sortKey: SortKey; current: SortKey; direction: SortDirection; onSort: (key: SortKey) => void }) {
  const Icon = current !== sortKey ? ArrowUpDown : direction === 'asc' ? ArrowUp : ArrowDown
  return <th className="px-3 py-3"><button onClick={() => onSort(sortKey)} className="inline-flex items-center gap-1 rounded outline-none hover:text-[var(--text)] focus-visible:ring-2 focus-visible:ring-[var(--focus)]">{label}<Icon className="size-3" aria-hidden="true" /></button></th>
}

function FilterSelect({ label, value, onChange, children }: { label: string; value: string; onChange: (value: string) => void; children: ReactNode }) {
  return <label className="text-[10px] font-bold uppercase tracking-wider text-[var(--text-muted)]"><span className="sr-only">{label}</span><select aria-label={label} value={value} onChange={(event) => onChange(event.target.value)} className="h-10 min-w-44 rounded-lg border border-[var(--border-strong)] bg-[var(--surface)] px-3 text-sm font-medium normal-case tracking-normal text-[var(--text)] outline-none focus:ring-2 focus:ring-[var(--focus)]">{children}</select></label>
}

function LoadingState() { return <div className="space-y-2 p-4" aria-label="Se încarcă lista facturilor">{Array.from({ length: 6 }, (_, index) => <div key={index} className="h-16 animate-pulse rounded-lg bg-[var(--surface-subtle)]" />)}</div> }
function ErrorState() { return <div className="p-12 text-center"><TriangleAlert className="mx-auto size-8 text-[var(--danger)]" /><h3 className="mt-3 font-bold">Facturile nu au putut fi încărcate</h3><p className="mt-1 text-sm text-[var(--text-secondary)]">Repository-ul demonstrativ a returnat o eroare.</p></div> }
function EmptyState() { return <div className="p-12 text-center"><CheckCircle2 className="mx-auto size-8 text-[var(--success)]" /><h3 className="mt-3 font-bold">Nu există facturi în acest context</h3><p className="mt-1 text-sm text-[var(--text-secondary)]">Nu au fost găsite facturi pentru clientul selectat.</p></div> }
function NoResults({ onReset }: { onReset: () => void }) { return <div className="p-12 text-center"><SearchX className="mx-auto size-8 text-[var(--text-muted)]" /><h3 className="mt-3 font-bold">Niciun rezultat</h3><p className="mt-1 text-sm text-[var(--text-secondary)]">Căutarea sau filtrele curente nu corespund niciunei facturi.</p><button onClick={onReset} className="mt-4 rounded-lg border border-[var(--border-strong)] px-3 py-2 text-sm font-semibold focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]">Resetează filtrele</button></div> }

function compareInvoices(left: Invoice, right: Invoice, key: SortKey, direction: SortDirection) {
  const values: Record<SortKey, [string | number, string | number]> = {
    supplier: [left.supplierName, right.supplierName], number: [left.documentNumber, right.documentNumber], date: [left.issueDate, right.issueDate], value: [left.total.amount, right.total.amount], pipeline: [left.pipelineStatus, right.pipelineStatus],
  }
  const [leftValue, rightValue] = values[key]
  const result = typeof leftValue === 'number' && typeof rightValue === 'number' ? leftValue - rightValue : String(leftValue).localeCompare(String(rightValue), 'ro')
  return direction === 'asc' ? result : -result
}

function parseSort(value: string | null): SortKey { return value === 'supplier' || value === 'number' || value === 'value' || value === 'pipeline' ? value : 'date' }
function parsePipeline(value: string | null): PipelineStatus | 'ALL' { return pipelineOptions.includes(value as PipelineStatus) ? value as PipelineStatus : 'ALL' }
function parseSaga(value: string | null): SagaStatus | 'ALL' { return sagaOptions.includes(value as SagaStatus) ? value as SagaStatus : 'ALL' }
function parseAttention(value: string | null): AttentionFilter { return value === 'REQUIRED' || value === 'CLEAR' ? value : 'ALL' }
function pipelineLabel(value: PipelineStatus) { return PIPELINE_LABELS[value] }
function sagaLabel(value: SagaStatus) { return SAGA_LABELS[value] }
function formatMoney(amount: number, currency: string) { return new Intl.NumberFormat('ro-RO', { style: 'currency', currency }).format(amount) }
function formatDate(value: string) { return new Intl.DateTimeFormat('ro-RO', { dateStyle: 'medium', timeZone: 'Europe/Bucharest' }).format(new Date(value)) }
