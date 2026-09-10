import { AlertTriangle, ArrowRight, CheckCircle2, CircleDot, Clock3, Copy, FileCheck2, Inbox, Radio, TriangleAlert, XCircle } from 'lucide-react'
import { Link } from 'react-router-dom'
import { useClientScope } from '../../app/scope-context'
import { Badge } from '../../components/ui/badge'
import type { Client, Invoice } from '../../domain/invoice'
import { PIPELINE_LABELS, TASK_TYPE_LABELS } from '../../domain/invoice'
import { useClients, useInvoices } from '../invoices/invoice-hooks'
import { DEMO_CLOCK, pipelineGroups, selectDashboard } from './dashboard-selectors'

export function DashboardPage() {
  const { scope, setScope } = useClientScope()
  const invoiceQuery = useInvoices(scope)
  const clientQuery = useClients()
  const isLoading = invoiceQuery.isLoading || clientQuery.isLoading
  const isError = invoiceQuery.isError || clientQuery.isError
  const dashboard = selectDashboard(invoiceQuery.data ?? [], clientQuery.data ?? [])

  if (isLoading) return <DashboardLoading />
  if (isError) return <DashboardError />
  if (dashboard.invoices.length === 0) return <DashboardEmpty />

  const kpis = [
    { label: 'Facturi în procesare', value: dashboard.kpis.processing, icon: CircleDot, tone: 'info', to: '/invoices' },
    { label: 'Necesită atenție', value: dashboard.kpis.attention, icon: AlertTriangle, tone: 'warning', to: '/tasks?status=OPEN&type=ALL' },
    { label: 'Pregătite pentru SAGA', value: dashboard.kpis.ready, icon: FileCheck2, tone: 'info', to: '/invoices?pipeline=READY_FOR_SAGA' },
    { label: 'Exportate în SAGA', value: dashboard.kpis.exported, icon: CheckCircle2, tone: 'success', to: '/invoices?pipeline=EXPORTED&saga=EXPORTED' },
  ] as const

  return <div className="space-y-6">
    <section className="flex items-end justify-between gap-6">
      <div><p className="eyebrow">Septembrie 2026 · luna curentă demo</p><h2 className="mt-2 text-2xl font-bold tracking-tight">Situația operațională de azi</h2><p className="mt-2 text-sm text-[var(--text-secondary)]">Vezi imediat ce rulează automat, ce așteaptă și unde este necesară decizia contabilului.</p></div>
      <Badge tone="neutral"><Radio className="size-3.5" />SPV simulat · referință timp {formatTime(DEMO_CLOCK)}</Badge>
    </section>

    <section aria-label="Indicatori luna curentă" className="grid grid-cols-4 gap-4">
      {kpis.map(({ label, value, icon: Icon, tone, to }) => <Link key={label} to={to} data-kpi={label} className="card group block outline-none transition-colors hover:border-[var(--border-strong)] focus-visible:ring-2 focus-visible:ring-[var(--focus)]"><article className="p-5"><div className="flex items-center justify-between"><span className="text-sm font-medium text-[var(--text-secondary)]">{label}</span><span className={`grid size-9 place-items-center rounded-lg ${tone === 'warning' ? 'bg-[var(--warning-soft)] text-[var(--warning)]' : tone === 'success' ? 'bg-[var(--success-soft)] text-[var(--success)]' : 'bg-[var(--info-soft)] text-[var(--info)]'}`}><Icon className="size-[18px]" /></span></div><div className="mt-5 flex items-end justify-between"><div className="text-3xl font-bold tabular-nums">{value}</div><ArrowRight className="size-4 text-[var(--text-muted)] transition-transform group-hover:translate-x-0.5" /></div><div className="mt-1 text-xs text-[var(--text-muted)]">în contextul activ</div></article></Link>)}
    </section>

    <section aria-labelledby="operational-heading" className="space-y-4">
      <div><p className="eyebrow">Operational Overview</p><h3 id="operational-heading" className="mt-1 text-lg font-bold">Ce se întâmplă în pipeline</h3></div>
      <div className="grid grid-cols-[minmax(0,1.6fr)_minmax(340px,0.7fr)] gap-5">
        <div className="card overflow-hidden"><div className="border-b border-[var(--border)] px-5 py-4"><h4 className="font-bold">Distribuție pe stări</h4><p className="mt-1 text-xs text-[var(--text-muted)]">Toate stările aprobate rămân inspectabile; nu sunt introduse stări noi.</p></div><div className="divide-y divide-[var(--border)]">{pipelineGroups.map((group) => <div key={group.label} className="grid grid-cols-[150px_1fr] gap-4 px-5 py-4"><div className="text-xs font-bold text-[var(--text-secondary)]">{group.label}</div><div className="grid grid-cols-3 gap-2">{group.states.map((status) => <Link key={status} to={`/invoices?pipeline=${status}`} className="flex items-center justify-between rounded-lg border border-[var(--border)] bg-[var(--surface-raised)] px-3 py-2 outline-none hover:border-[var(--border-strong)] focus-visible:ring-2 focus-visible:ring-[var(--focus)]"><span className="truncate text-[11px] font-medium text-[var(--text-secondary)]">{PIPELINE_LABELS[status]}</span><strong className="ml-2 tabular-nums">{dashboard.pipelineCounts[status]}</strong></Link>)}</div></div>)}</div></div>
        <div className="space-y-5">
          <div className="card p-5"><div className="flex items-center justify-between"><h4 className="font-bold">Vizibilitate SAGA</h4><Badge tone={dashboard.sagaCounts.failed ? 'danger' : 'neutral'}>{dashboard.sagaCounts.failed ? <XCircle className="size-3.5" /> : <CheckCircle2 className="size-3.5" />}{dashboard.sagaCounts.failed} eșuate</Badge></div><dl className="mt-4 grid grid-cols-2 gap-3 text-sm"><SagaCount label="Pregătite" value={dashboard.sagaCounts.ready} /><SagaCount label="În export" value={dashboard.sagaCounts.exporting} /><SagaCount label="Exportate" value={dashboard.sagaCounts.exported} /><SagaCount label="Export eșuat" value={dashboard.sagaCounts.failed} /></dl>{dashboard.failedInvoices.map((invoice) => <Link key={invoice.id} to={invoiceLink(invoice, 'summary')} className="mt-4 flex items-center justify-between rounded-lg border border-[var(--danger-border)] bg-[var(--danger-soft)] p-3 text-xs outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]"><span><strong>{invoice.documentNumber}</strong><span className="mt-0.5 block text-[var(--text-secondary)]">{invoice.supplierName}</span></span><ArrowRight className="size-4 text-[var(--danger)]" /></Link>)}</div>
          <div className="card p-5"><div className="flex items-center gap-2 text-[var(--info)]"><Radio className="size-4" /><h4 className="font-bold">Intrare SPV simulată</h4></div><div className="mt-4 text-3xl font-bold tabular-nums">{dashboard.invoices.filter((invoice) => invoice.spvReference).length}</div><p className="mt-1 text-xs text-[var(--text-muted)]">facturi cu referință SPV demonstrativă în luna curentă</p></div>
        </div>
      </div>
      {scope === 'all' && <ClientOverview rows={dashboard.clients} onSelect={setScope} />}
    </section>

    <section aria-labelledby="attention-heading" className="space-y-4">
      <div><p className="eyebrow">Flux de excepții</p><h3 id="attention-heading" className="mt-1 text-lg font-bold">Necesită atenție acum</h3></div>
      <div className="grid grid-cols-[minmax(0,1.5fr)_minmax(330px,0.7fr)] gap-5">
        <div className="card overflow-hidden"><div className="flex items-center justify-between border-b border-[var(--border)] px-5 py-4"><div><h4 className="font-bold">Acțiuni contabile</h4><p className="mt-1 text-xs text-[var(--text-muted)]">Numai task-uri deschise pe care contabilul le poate rezolva acum.</p></div><Link to="/tasks?status=OPEN&type=ALL" className="rounded text-xs font-semibold text-[var(--accent)] outline-none hover:underline focus-visible:ring-2 focus-visible:ring-[var(--focus)]">Deschide task-urile</Link></div>{dashboard.openInvoices.length === 0 ? <ZeroOpen /> : <div className="divide-y divide-[var(--border)]">{dashboard.openInvoices.map((invoice) => <AttentionRow key={invoice.id} invoice={invoice} clients={clientQuery.data ?? []} />)}</div>}</div>
        <div className="card overflow-hidden"><div className="border-b border-[var(--border)] px-5 py-4"><div className="flex items-center gap-2"><Clock3 className="size-4 text-[var(--info)]" /><h4 className="font-bold">În așteptare</h4></div><p className="mt-1 text-xs text-[var(--text-muted)]">Condiție externă; nu este acțiune contabilă imediată.</p></div>{dashboard.waitingInvoices.length === 0 ? <div className="p-8 text-center"><Inbox className="mx-auto size-6 text-[var(--text-muted)]" /><p className="mt-2 text-sm text-[var(--text-secondary)]">Nu există elemente în așteptare.</p></div> : <div className="divide-y divide-[var(--border)]">{dashboard.waitingInvoices.map((invoice) => <Link key={invoice.id} to={invoiceLink(invoice, 'contract')} className="flex items-center justify-between p-4 outline-none hover:bg-[var(--surface-subtle)] focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-[var(--focus)]"><div><div className="text-sm font-bold">{invoice.documentNumber}</div><p className="mt-1 text-xs text-[var(--text-secondary)]">{invoice.task?.reason}</p></div><Badge tone="info"><Clock3 className="size-3.5" />Extern</Badge></Link>)}</div>}</div>
      </div>
    </section>
  </div>
}

function ClientOverview({ rows, onSelect }: { rows: ReturnType<typeof selectDashboard>['clients']; onSelect: (id: string) => void }) {
  return <div className="card overflow-hidden"><div className="border-b border-[var(--border)] px-5 py-4"><h4 className="font-bold">Context pe clienți</h4><p className="mt-1 text-xs text-[var(--text-muted)]">Identifică rapid clienții cu acțiuni sau condiții externe în așteptare.</p></div><table className="w-full border-collapse text-left text-xs"><caption className="sr-only">Situația operațională pe clienți</caption><thead className="bg-[var(--surface-subtle)] text-[10px] font-bold uppercase tracking-wider text-[var(--text-muted)]"><tr><th className="px-5 py-3">Client</th><th className="px-4 py-3">Facturi</th><th className="px-4 py-3">Acțiuni deschise</th><th className="px-4 py-3">În așteptare</th><th className="px-4 py-3">În procesare</th><th className="px-4 py-3">Pregătite SAGA</th><th className="px-5 py-3">Exportate</th></tr></thead><tbody className="divide-y divide-[var(--border)]">{rows.map((row) => <tr key={row.client.id}><td className="px-5 py-4"><button onClick={() => onSelect(row.client.id)} className="rounded text-left font-bold text-[var(--accent)] outline-none hover:underline focus-visible:ring-2 focus-visible:ring-[var(--focus)]">{row.client.name}</button><div className="mt-1 text-[var(--text-muted)]">{row.client.cui}</div></td><td className="px-4 py-4 tabular-nums">{row.invoiceCount}</td><td className="px-4 py-4 font-bold tabular-nums">{row.openCount}</td><td className="px-4 py-4 tabular-nums">{row.waitingCount}</td><td className="px-4 py-4 tabular-nums">{row.processingCount}</td><td className="px-4 py-4 tabular-nums">{row.readyCount}</td><td className="px-5 py-4 tabular-nums">{row.exportedCount}</td></tr>)}</tbody></table></div>
}

function AttentionRow({ invoice, clients }: { invoice: Invoice; clients: Client[] }) {
  const tab = invoice.task?.type === 'CLASSIFICATION' ? 'classification' : 'contract'
  const client = clients.find((candidate) => candidate.id === invoice.clientId)
  return <Link to={invoiceLink(invoice, tab)} className="grid grid-cols-[170px_1fr_190px_150px] items-center gap-4 px-5 py-4 outline-none hover:bg-[var(--surface-subtle)] focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-[var(--focus)]"><div><Badge tone="warning"><AlertTriangle className="size-3.5" />{invoice.task ? TASK_TYPE_LABELS[invoice.task.type] : 'Acțiune'}</Badge><div className="mt-2 text-xs font-semibold">{invoice.documentNumber}</div></div><div><div className="text-sm font-semibold">{invoice.supplierName}</div><p className="mt-1 line-clamp-2 text-xs text-[var(--text-secondary)]">{invoice.task?.reason}</p></div><div className="text-xs text-[var(--text-secondary)]">{client?.name ?? 'Client indisponibil'}</div><div className="flex items-center justify-end gap-2 text-right text-xs font-semibold text-[var(--accent)]">{PIPELINE_LABELS[invoice.pipelineStatus]}<ArrowRight className="size-3.5 shrink-0" /></div></Link>
}

function SagaCount({ label, value }: { label: string; value: number }) { return <div className="rounded-lg bg-[var(--surface-subtle)] p-3"><dt className="text-xs text-[var(--text-muted)]">{label}</dt><dd className="mt-1 text-xl font-bold tabular-nums">{value}</dd></div> }
function ZeroOpen() { return <div className="p-10 text-center"><CheckCircle2 className="mx-auto size-8 text-[var(--success)]" /><h4 className="mt-3 font-bold">Nimic nu necesită acțiune acum</h4><p className="mt-1 text-sm text-[var(--text-secondary)]">Nu există task-uri deschise în contextul activ.</p></div> }
function DashboardLoading() { return <div className="space-y-4" aria-label="Se încarcă dashboard-ul"><div className="grid grid-cols-4 gap-4">{[1, 2, 3, 4].map((item) => <div key={item} className="card h-32 animate-pulse bg-[var(--surface-subtle)]" />)}</div><div className="card h-80 animate-pulse bg-[var(--surface-subtle)]" /></div> }
function DashboardError() { return <div className="card p-12 text-center"><TriangleAlert className="mx-auto size-8 text-[var(--danger)]" /><h2 className="mt-3 font-bold">Dashboard-ul nu a putut fi încărcat</h2><p className="mt-1 text-sm text-[var(--text-secondary)]">Repository-ul demonstrativ a returnat o eroare.</p></div> }
function DashboardEmpty() { return <div className="card p-12 text-center"><Copy className="mx-auto size-8 text-[var(--text-muted)]" /><h2 className="mt-3 font-bold">Nu există facturi în luna curentă</h2><p className="mt-1 text-sm text-[var(--text-secondary)]">Contextul de client selectat nu conține facturi pentru perioada demonstrativă.</p></div> }
function invoiceLink(invoice: Invoice, tab: string) { return `/invoices/${invoice.id}?tab=${tab}&returnTo=${encodeURIComponent('/')}` }
function formatTime(value: string) { return new Intl.DateTimeFormat('ro-RO', { hour: '2-digit', minute: '2-digit', timeZone: 'Europe/Bucharest' }).format(new Date(value)) }
