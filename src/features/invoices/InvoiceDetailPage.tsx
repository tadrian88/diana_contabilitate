import * as Tabs from '@radix-ui/react-tabs'
import { AlertTriangle, ArrowLeft, Building2, CalendarDays, CircleDollarSign, FileText, ShieldCheck } from 'lucide-react'
import { Link, useParams, useSearchParams } from 'react-router-dom'
import { Badge } from '../../components/ui/badge'
import type { Invoice } from '../../domain/invoice'
import { PIPELINE_LABELS } from '../../domain/invoice'
import { useClients, useInvoice, useStartHappyPath } from './invoice-hooks'
import { PipelineStepper } from './PipelineStepper'
import { ContractTask } from './ContractTask'
import { ClassificationWorkspace } from './ClassificationWorkspace'
import { AttentionBadge, PipelineBadge, SagaBadge } from './InvoiceStatusBadges'
import { getUnresolvedIssueCount, SAGA_LABELS } from './invoice-view'
import { SagaExportCard } from './SagaExportCard'

const tabValues = ['summary', 'contract', 'lines', 'classification', 'history'] as const
type InvoiceTab = typeof tabValues[number]

export function InvoiceDetailPage() {
  const { invoiceId = '' } = useParams()
  const [searchParams, setSearchParams] = useSearchParams()
  const { data: invoice, isLoading } = useInvoice(invoiceId)
  const { data: clients = [] } = useClients()
  const tab = parseTab(searchParams.get('tab'))
  useStartHappyPath(invoice ?? undefined)

  const client = clients.find((candidate) => candidate.id === invoice?.clientId)
  const requestedReturnPath = searchParams.get('returnTo')
  const returnPath = requestedReturnPath?.startsWith('/tasks') || requestedReturnPath?.startsWith('/invoices') || requestedReturnPath?.startsWith('/contracts') ? requestedReturnPath : '/'
  const returnLabel = returnPath.startsWith('/tasks') ? 'Înapoi la Task Inbox' : returnPath.startsWith('/contracts') ? 'Înapoi la lista contractelor' : returnPath.startsWith('/invoices') ? 'Înapoi la lista facturilor' : 'Înapoi la Dashboard'

  if (isLoading) return <div className="card h-96 animate-pulse bg-[var(--surface-subtle)]" aria-label="Se încarcă factura" />
  if (!invoice) return <div className="card p-10 text-center"><h2 className="text-lg font-bold">Factura nu a fost găsită</h2><Link className="mt-4 inline-block text-sm font-semibold text-[var(--accent)]" to="/">Înapoi la Dashboard</Link></div>

  return (
    <div className="space-y-5">
      <Link to={returnPath} className="inline-flex items-center gap-2 rounded text-sm font-semibold text-[var(--text-secondary)] outline-none hover:text-[var(--accent)] focus-visible:ring-2 focus-visible:ring-[var(--focus)]"><ArrowLeft className="size-4" />{returnLabel}</Link>
      <section className="flex items-start justify-between gap-8">
        <div>
          <div className="flex flex-wrap items-center gap-2"><Badge tone="neutral">Factură demonstrativă</Badge><PipelineBadge status={invoice.pipelineStatus} /><AttentionBadge invoice={invoice} /><SagaBadge status={invoice.sagaStatus} /></div>
          <h2 className="mt-3 text-2xl font-bold tracking-tight">{invoice.documentNumber}</h2>
          <p className="mt-1 text-sm text-[var(--text-secondary)]">{invoice.supplierName}{invoice.supplierCui ? ` · ${invoice.supplierCui}` : ''}</p>
        </div>
        <div className="grid grid-cols-3 divide-x divide-[var(--border)] rounded-xl border border-[var(--border)] bg-[var(--surface)] shadow-sm">
          <div className="px-4 py-3"><div className="eyebrow">Client</div><div className="mt-1 flex items-center gap-2 whitespace-nowrap text-sm font-semibold"><Building2 className="size-4 text-[var(--accent)]" />{client?.name}</div></div>
          <div className="px-4 py-3"><div className="eyebrow">Valoare</div><div className="mt-1 whitespace-nowrap text-sm font-semibold">{formatMoney(invoice.total.amount, invoice.total.currency)}</div></div>
          <div className="px-4 py-3"><div className="eyebrow">Data facturii</div><div className="mt-1 whitespace-nowrap text-sm font-semibold">{formatDate(invoice.issueDate)}</div></div>
        </div>
      </section>

      <PipelineStepper invoice={invoice} />

      {invoice.autoRun && invoice.pipelineStatus !== 'EXPORTED' && (
        <div className="flex items-center gap-3 rounded-xl border border-[var(--info-border)] bg-[var(--info-soft)] px-4 py-3 text-sm text-[var(--info)]" role="status">
          <span className="size-2 animate-pulse rounded-full bg-[var(--info)]" aria-hidden="true" />
          <strong>Procesare automată în curs</strong>
          <span className="text-[var(--text-secondary)]">Etapa curentă: {PIPELINE_LABELS[invoice.pipelineStatus]}. Nu este necesară o acțiune.</span>
        </div>
      )}

      <Tabs.Root value={tab} onValueChange={(value) => {
        const next = new URLSearchParams(searchParams)
        next.set('tab', value)
        setSearchParams(next, { replace: true })
      }} className="card overflow-hidden">
        <Tabs.List aria-label="Secțiuni factură" className="flex border-b border-[var(--border)] bg-[var(--surface-raised)] px-5">
          {[
            ['summary', 'Rezumat'], ['contract', 'Contract'], ['lines', 'Linii factură'], ['classification', 'Clasificare'], ['history', 'Istoric'],
          ].map(([value, label]) => <Tabs.Trigger key={value} value={value} className="tab-trigger px-4 py-4 text-sm font-semibold text-[var(--text-secondary)] outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-[var(--focus)]">{label}</Tabs.Trigger>)}
        </Tabs.List>
        <div className="bg-[var(--app-background)] p-5">
          <Tabs.Content value="summary" className="outline-none"><SummaryTab invoice={invoice} /></Tabs.Content>
          <Tabs.Content value="contract" className="outline-none"><ContractTask invoice={invoice} /></Tabs.Content>
          <Tabs.Content value="lines" className="outline-none"><LinesTab invoice={invoice} /></Tabs.Content>
          <Tabs.Content value="classification" className="outline-none"><ClassificationWorkspace invoice={invoice} /></Tabs.Content>
          <Tabs.Content value="history" className="outline-none"><HistoryTab invoice={invoice} /></Tabs.Content>
        </div>
      </Tabs.Root>
    </div>
  )
}

function SummaryTab({ invoice }: { invoice: Invoice }) {
  const cards = [
    { label: 'Valoare totală', value: formatMoney(invoice.total.amount, invoice.total.currency), icon: CircleDollarSign },
    { label: 'Data emiterii', value: formatDate(invoice.issueDate), icon: CalendarDays },
    { label: 'Referință SPV', value: invoice.spvReference, icon: ShieldCheck },
    { label: 'Status SAGA', value: sagaLabel(invoice), icon: FileText },
  ]
  return (
    <div className="space-y-4">
      <div className="grid grid-cols-4 gap-4">{cards.map(({ label, value, icon: Icon }) => <div key={label} className="card p-4"><Icon className="size-5 text-[var(--accent)]" /><div className="mt-4 text-xs text-[var(--text-muted)]">{label}</div><div className="mt-1 text-sm font-bold">{value}</div></div>)}</div>
      <div className="grid grid-cols-2 gap-4">
        <div className={`card p-5 ${getUnresolvedIssueCount(invoice) ? 'border-[var(--warning-border)]' : ''}`}><div className="eyebrow">Ce se întâmplă acum</div><p className="mt-2 text-sm font-medium">{nextAction(invoice)}</p>{invoice.task && invoice.task.status !== 'RESOLVED' && <div className="mt-3 flex items-start gap-2 text-xs text-[var(--text-secondary)]"><AlertTriangle className="mt-0.5 size-3.5 shrink-0 text-[var(--warning)]" />{invoice.task.reason}</div>}</div>
        <div className="card p-5"><div className="eyebrow">Context asociat</div><div className="mt-2 text-sm font-semibold">{invoice.contract?.reference ?? (invoice.task?.type === 'CONTRACT_MATCH' ? 'Contract în curs de validare' : 'Niciun contract asociat')}</div><div className="mt-1 text-xs text-[var(--text-muted)]">{getUnresolvedIssueCount(invoice)} probleme nerezolvate · ultima activitate: {invoice.activity.at(-1)?.label ?? 'Indisponibilă'}</div></div>
      </div>
      <SagaExportCard invoice={invoice} />
    </div>
  )
}

function LinesTab({ invoice }: { invoice: Invoice }) {
  const pendingLineIds = new Set(invoice.task?.classificationItems?.filter((item) => item.status === 'PENDING').map((item) => item.lineId))
  const linesReadIndex = invoice.pipelinePath.indexOf('LINES_READ')
  const currentIndex = invoice.pipelinePath.indexOf(invoice.pipelineStatus)
  if (linesReadIndex < 0 || currentIndex < linesReadIndex || invoice.lines.length === 0) return <div className="card p-10 text-center"><FileText className="mx-auto size-8 text-[var(--text-muted)]" /><h3 className="mt-3 font-bold">Liniile facturii nu sunt disponibile</h3><p className="mt-1 text-sm text-[var(--text-secondary)]">Pipeline-ul nu a ajuns la citirea liniilor sau datele opționale lipsesc.</p></div>
  return (
    <div className="card overflow-hidden">
      <table className="w-full border-collapse text-left text-xs"><caption className="sr-only">Liniile sursă ale facturii</caption><thead className="bg-[var(--surface-subtle)] text-[10px] font-bold uppercase tracking-wider text-[var(--text-muted)]"><tr><th className="px-3 py-3">#</th><th className="px-3 py-3">Denumire articol/serviciu</th><th className="px-3 py-3">UM</th><th className="px-3 py-3">TVA</th><th className="px-3 py-3">Cantitate</th><th className="px-3 py-3">Preț unitar</th><th className="px-3 py-3">Valoare</th><th className="px-3 py-3">TVA valoare</th><th className="px-3 py-3">Total</th><th className="px-3 py-3">Informații suplimentare</th></tr></thead><tbody className="divide-y divide-[var(--border)]">{invoice.lines.map((line) => <tr key={line.id}><td className="px-3 py-4 text-[var(--text-muted)]">{line.position}</td><td className="max-w-[240px] px-3 py-4"><div className="font-semibold">{line.description}</div>{pendingLineIds.has(line.id) && <Badge className="mt-2" tone="warning">Clasificare de revizuit</Badge>}</td><td className="px-3 py-4">{line.unit}</td><td className="max-w-[130px] px-3 py-4">{line.vatLabel}</td><td className="px-3 py-4 tabular-nums">{line.quantity}</td><td className="whitespace-nowrap px-3 py-4">{formatMoney(line.unitPrice.amount, line.unitPrice.currency)}</td><td className="whitespace-nowrap px-3 py-4">{formatMoney(line.netValue.amount, line.netValue.currency)}</td><td className="whitespace-nowrap px-3 py-4">{formatMoney(line.vatValue.amount, line.vatValue.currency)}</td><td className="whitespace-nowrap px-3 py-4 font-semibold">{formatMoney(line.grossValue.amount, line.grossValue.currency)}</td><td className="max-w-[180px] px-3 py-4 text-[var(--text-secondary)]">{line.additionalInfo ?? '—'}</td></tr>)}</tbody></table>
    </div>
  )
}

function HistoryTab({ invoice }: { invoice: Invoice }) {
  return <div className="card p-5"><ol className="space-y-0">{[...invoice.activity].reverse().map((event, index) => <li key={event.id} className="relative flex gap-4 pb-6 last:pb-0"><span className="relative z-10 mt-1.5 size-2.5 shrink-0 rounded-full bg-[var(--accent)] ring-4 ring-[var(--info-soft)]" />{index < invoice.activity.length - 1 && <span className="absolute left-[4px] top-4 h-full w-px bg-[var(--border)]" />}<div className="min-w-0 flex-1"><div className="flex items-center justify-between gap-4"><div className="font-semibold">{event.label}</div><div className="text-xs text-[var(--text-muted)]">{event.actor} · {formatDateTime(event.timestamp)}</div></div><p className="mt-1 text-sm text-[var(--text-secondary)]">{event.detail}</p>{(event.before || event.after) && <div className="mt-3 flex items-center gap-2 text-xs"><span className="rounded-md bg-[var(--surface-subtle)] px-2 py-1 text-[var(--text-secondary)]">{event.before ?? '—'}</span><span aria-hidden="true">→</span><span className="rounded-md bg-[var(--info-soft)] px-2 py-1 font-semibold text-[var(--info)]">{event.after ?? '—'}</span></div>}</div></li>)}</ol></div>
}

function sagaLabel(invoice: Invoice) {
  return SAGA_LABELS[invoice.sagaStatus]
}

function nextAction(invoice: Invoice) {
  if (invoice.sagaStatus === 'FAILED') return 'Exportul SAGA simulat a eșuat. Nu există comportament de reîncercare aprobat.'
  if (invoice.pipelineStatus === 'AWAITING_CONTRACT') return 'Așteaptă furnizarea externă a contractului.'
  if (invoice.pipelineStatus === 'AWAITING_MATCH_CONFIRM') return 'Contabilul confirmă contractul recomandat sau selectează o alternativă.'
  if (invoice.pipelineStatus === 'AWAITING_REVIEW') return 'Contabilul revizuiește numai clasificările incerte.'
  if (invoice.pipelineStatus === 'EXPORTING' && invoice.sagaExport?.artifactStatus === 'GENERATED') return 'Fișierul este pregătit. Importă-l în SAGA și confirmă manual importul.'
  if (invoice.pipelineStatus === 'EXPORTED' && invoice.sagaExport?.confirmationType === 'HUMAN') return 'Procesare finalizată prin confirmarea manuală a importului în SAGA.'
  if (invoice.pipelineStatus === 'EXPORTED') return 'Procesare finalizată. Factura a ajuns în starea SAGA simulată.'
  if (invoice.pipelineStatus === 'DUPLICATE') return 'Stare terminală. Factura nu continuă către SAGA.'
  return 'Procesarea automată continuă fără intervenția contabilului.'
}

function parseTab(value: string | null): InvoiceTab { return tabValues.includes(value as InvoiceTab) ? value as InvoiceTab : 'summary' }

function formatMoney(amount: number, currency: string) { return new Intl.NumberFormat('ro-RO', { style: 'currency', currency }).format(amount) }
function formatDate(value: string) { return new Intl.DateTimeFormat('ro-RO', { dateStyle: 'medium', timeZone: 'Europe/Bucharest' }).format(new Date(value)) }
function formatDateTime(value: string) { return new Intl.DateTimeFormat('ro-RO', { dateStyle: 'medium', timeStyle: 'short', timeZone: 'Europe/Bucharest' }).format(new Date(value)) }
