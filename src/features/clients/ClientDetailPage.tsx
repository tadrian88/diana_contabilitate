import { AlertCircle, ArrowLeft, ArrowRight, Building2, FileText, Scale, ScrollText } from 'lucide-react'
import { useEffect } from 'react'
import { Link, useParams } from 'react-router-dom'
import { useClientScope } from '../../app/scope-context'
import { Badge } from '../../components/ui/badge'
import { PIPELINE_LABELS, TASK_TYPE_LABELS } from '../../domain/invoice'
import { useContracts } from '../contracts/contract-hooks'
import { PipelineBadge, SagaBadge } from '../invoices/InvoiceStatusBadges'
import { useClients, useInvoices } from '../invoices/invoice-hooks'
import { useRules } from '../rules/rule-hooks'
import { RULE_CATEGORY_LABELS } from '../rules/rule-view'
import { selectClientOperations } from './client-selectors'
import { lifecycleLabels } from '../../domain/client-management'
import { ClientSettings } from './ClientSettings'
import { SPVConnectionCard } from './SPVConnectionCard'

export function ClientDetailPage() {
  const { clientId = '' } = useParams()
  const { setScope } = useClientScope()
  const clientQuery = useClients()
  const invoiceQuery = useInvoices(clientId)
  const contractQuery = useContracts(clientId)
  const ruleQuery = useRules(clientId)
  const client = clientQuery.data?.find((candidate) => candidate.id === clientId)
  const isLoading = clientQuery.isLoading || invoiceQuery.isLoading || contractQuery.isLoading || ruleQuery.isLoading
  const isError = clientQuery.isError || invoiceQuery.isError || contractQuery.isError || ruleQuery.isError

  useEffect(() => { if (client) setScope(client.id) }, [client, setScope])

  if (isLoading) return <div className="card h-96 animate-pulse bg-[var(--surface-subtle)]" aria-label="Se încarcă clientul" />
  if (isError) return <div className="card p-10 text-center"><AlertCircle className="mx-auto size-8 text-[var(--danger)]" /><h2 className="mt-3 font-bold">Contextul clientului nu a putut fi încărcat</h2></div>
  if (!client) return <div className="card p-10 text-center"><Building2 className="mx-auto size-8 text-[var(--text-muted)]" /><h2 className="mt-3 text-lg font-bold">Clientul nu a fost găsit</h2><Link to="/clients" className="mt-4 inline-block text-sm font-semibold text-[var(--accent)]">Înapoi la Clienți</Link></div>

  const operations = selectClientOperations(client, invoiceQuery.data ?? [], contractQuery.data ?? [], ruleQuery.data ?? [])
  return <div className="space-y-6">
    <Link to="/clients" className="inline-flex items-center gap-2 rounded text-sm font-semibold text-[var(--text-secondary)] outline-none hover:text-[var(--accent)] focus-visible:ring-2 focus-visible:ring-[var(--focus)]"><ArrowLeft className="size-4" />Înapoi la Clienți</Link>
    <section className="flex items-end justify-between gap-6"><div><p className="eyebrow">Context operațional client</p><h2 className="mt-2 text-2xl font-bold tracking-tight">{client.name}</h2><p className="mt-1 text-sm text-[var(--text-secondary)]">{client.cui}</p></div><Badge tone="info">{lifecycleLabels[client.status??'ACTIVE']}</Badge></section>

    <nav aria-label="Navigare în contextul clientului" className="grid grid-cols-4 gap-3"><ContextLink to="/invoices" label="Facturi" detail={`${operations.invoices.length} înregistrări`} icon={FileText} /><ContextLink to="/tasks?status=OPEN&type=ALL" label="Task-uri" detail={`${operations.counts.open} deschise`} icon={ArrowRight} /><ContextLink to="/contracts" label="Contracte" detail={`${operations.contracts.length} disponibile`} icon={ScrollText} /><ContextLink to="/rules?scope=CLIENT_OVERRIDE" label="Reguli" detail={`${operations.ruleOverrides.length} override-uri`} icon={Scale} /></nav>

    <section aria-label="Stări SAGA" className="grid grid-cols-4 gap-3"><Metric label="Pregătite pentru SAGA" value={operations.counts.ready} /><Metric label="Export în curs" value={operations.counts.exporting} /><Metric label="Exportate în SAGA" value={operations.counts.exported} /><Metric label="Export eșuat" value={operations.counts.failed} danger={operations.counts.failed > 0} /></section>

    <ClientSettings clientId={client.id} />
    <SPVConnectionCard clientId={client.id} clientName={client.name} clientCUI={client.cui} />

    <div className="grid grid-cols-2 gap-5">
      <Section title="Facturi" action="Vezi toate facturile" to="/invoices">{operations.invoices.length === 0 ? <Empty text="Nu există facturi pentru acest client." /> : operations.invoices.slice(0, 5).map((invoice) => <Link key={invoice.id} to={`/invoices/${invoice.id}`} className="flex items-center justify-between gap-4 border-b border-[var(--border)] px-5 py-3 last:border-0 hover:bg-[var(--surface-subtle)]"><div><div className="text-sm font-bold">{invoice.documentNumber}</div><div className="mt-1 text-xs text-[var(--text-secondary)]">{invoice.supplierName}</div></div><div className="flex items-center gap-2"><PipelineBadge status={invoice.pipelineStatus} /><SagaBadge status={invoice.sagaStatus} /></div></Link>)}</Section>
      <div className="space-y-5"><TaskSection title="Task-uri deschise" items={operations.openInvoices} /><TaskSection title="În așteptare" items={operations.waitingInvoices} /></div>
      <Section title="Contracte" action="Vezi toate contractele" to="/contracts">{operations.contracts.length === 0 ? <Empty text="Nu există contracte pentru acest client." /> : operations.contracts.slice(0, 4).map((contract) => <Link key={contract.id} to={`/contracts/${contract.id}`} className="flex items-center justify-between border-b border-[var(--border)] px-5 py-3 text-sm last:border-0 hover:bg-[var(--surface-subtle)]"><span className="font-bold text-[var(--accent)]">{contract.reference}</span><span className="text-xs text-[var(--text-secondary)]">{contract.supplierName}</span></Link>)}</Section>
      <Section title="Override-uri client" action="Vezi regulile" to="/rules?scope=CLIENT_OVERRIDE">{operations.ruleOverrides.length === 0 ? <Empty text="Nu există override-uri pentru acest client." /> : operations.ruleOverrides.map((rule) => <Link key={rule.id} to={`/rules/${rule.id}`} className="flex items-center justify-between border-b border-[var(--border)] px-5 py-3 text-sm last:border-0 hover:bg-[var(--surface-subtle)]"><div><div className="font-bold text-[var(--accent)]">{rule.name}</div><div className="mt-1 text-xs text-[var(--text-muted)]">{rule.reference}</div></div><Badge tone="info">{RULE_CATEGORY_LABELS[rule.category]} · Override client</Badge></Link>)}</Section>
    </div>
  </div>
}

function ContextLink({ to, label, detail, icon: Icon }: { to: string; label: string; detail: string; icon: typeof FileText }) { return <Link to={to} className="card group flex items-center gap-3 p-4 outline-none hover:border-[var(--border-strong)] focus-visible:ring-2 focus-visible:ring-[var(--focus)]"><span className="grid size-9 place-items-center rounded-lg bg-[var(--info-soft)] text-[var(--info)]"><Icon className="size-4" /></span><span><span className="block text-sm font-bold">{label}</span><span className="text-xs text-[var(--text-muted)]">{detail}</span></span><ArrowRight className="ml-auto size-4 text-[var(--text-muted)] transition-transform group-hover:translate-x-0.5" /></Link> }
function Metric({ label, value, danger = false }: { label: string; value: number; danger?: boolean }) { return <div className="card p-4"><div className="text-xs font-semibold text-[var(--text-secondary)]">{label}</div><div className={`mt-2 text-2xl font-bold tabular-nums ${danger ? 'text-[var(--danger)]' : ''}`}>{value}</div></div> }
function Section({ title, action, to, children }: { title: string; action: string; to: string; children: React.ReactNode }) { return <section className="card overflow-hidden"><div className="flex items-center justify-between border-b border-[var(--border)] px-5 py-4"><h3 className="font-bold">{title}</h3><Link to={to} className="rounded text-xs font-semibold text-[var(--accent)] outline-none hover:underline focus-visible:ring-2 focus-visible:ring-[var(--focus)]">{action}</Link></div>{children}</section> }
function TaskSection({ title, items }: { title: string; items: ReturnType<typeof selectClientOperations>['openInvoices'] }) { return <Section title={title} action="Vezi task-urile" to={`/tasks?status=${title === 'În așteptare' ? 'WAITING' : 'OPEN'}&type=ALL`}>{items.length === 0 ? <Empty text={`Nu există task-uri ${title.toLocaleLowerCase('ro-RO')}.`} /> : items.map((invoice) => <Link key={invoice.id} to={`/invoices/${invoice.id}?tab=${invoice.task?.type === 'CLASSIFICATION' ? 'classification' : 'contract'}`} className="flex items-center justify-between border-b border-[var(--border)] px-5 py-3 text-sm last:border-0 hover:bg-[var(--surface-subtle)]"><span><strong>{invoice.documentNumber}</strong><span className="mt-1 block text-xs text-[var(--text-secondary)]">{invoice.task ? TASK_TYPE_LABELS[invoice.task.type] : 'Task indisponibil'}</span></span><Badge tone={invoice.task?.status === 'WAITING' ? 'info' : 'warning'}>{invoice.task?.status === 'WAITING' ? 'În așteptare' : PIPELINE_LABELS[invoice.pipelineStatus]}</Badge></Link>)}</Section> }
function Empty({ text }: { text: string }) { return <p className="px-5 py-6 text-sm text-[var(--text-muted)]">{text}</p> }
