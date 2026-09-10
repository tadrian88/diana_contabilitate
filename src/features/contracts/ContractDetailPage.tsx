import { ArrowLeft, Building2, CalendarRange, FileText, Landmark, ReceiptText } from 'lucide-react'
import { Link, useParams, useSearchParams } from 'react-router-dom'
import { Badge } from '../../components/ui/badge'
import { useClients, useInvoice, useInvoices } from '../invoices/invoice-hooks'
import { AttentionBadge, PipelineBadge } from '../invoices/InvoiceStatusBadges'
import { useContract } from './contract-hooks'

export function ContractDetailPage() {
  const { contractId = '' } = useParams()
  const [params] = useSearchParams()
  const { data: contract, isLoading, isError } = useContract(contractId)
  const { data: invoices = [] } = useInvoices('all')
  const { data: clients = [] } = useClients()
  const contextInvoiceId = params.get('invoiceId') ?? ''
  const { data: contextInvoice } = useInvoice(contextInvoiceId)
  const requestedReturn = params.get('returnTo')
  const returnTo = requestedReturn?.startsWith('/invoices/') || requestedReturn?.startsWith('/contracts') ? requestedReturn : '/contracts'

  if (isLoading) return <div className="card h-96 animate-pulse bg-[var(--surface-subtle)]" aria-label="Se încarcă detaliul contractului" />
  if (isError) return <div className="card p-10 text-center"><h2 className="font-bold">Contractul nu a putut fi încărcat</h2><p className="mt-2 text-sm text-[var(--text-secondary)]">Repository-ul demonstrativ a returnat o eroare.</p></div>
  if (!contract) return <div className="card p-10 text-center"><h2 className="text-lg font-bold">Contractul nu a fost găsit</h2><Link to="/contracts" className="mt-4 inline-block text-sm font-semibold text-[var(--accent)]">Înapoi la Contracte</Link></div>

  const client = clients.find((candidate) => candidate.id === contract.clientId)
  const associatedInvoices = invoices.filter((invoice) => invoice.selectedContractId === contract.id)
  const matchCandidate = contextInvoice?.task?.type === 'CONTRACT_MATCH' ? contextInvoice.task.contractCandidates?.find((candidate) => candidate.id === contract.id) : undefined
  const detailReturn = `/contracts/${contract.id}${params.toString() ? `?${params.toString()}` : ''}`

  return <div className="space-y-5">
    <Link to={returnTo} className="inline-flex items-center gap-2 rounded text-sm font-semibold text-[var(--text-secondary)] outline-none hover:text-[var(--accent)] focus-visible:ring-2 focus-visible:ring-[var(--focus)]"><ArrowLeft className="size-4" />{returnTo.startsWith('/invoices/') ? 'Înapoi la revizuirea facturii' : 'Înapoi la lista contractelor'}</Link>
    <section className="flex items-end justify-between gap-8"><div><div className="flex items-center gap-2"><Badge tone="neutral">Contract demonstrativ</Badge>{matchCandidate?.recommended && <Badge tone="info">Recomandat pentru revizuire</Badge>}</div><h2 className="mt-3 text-2xl font-bold tracking-tight">{contract.reference}</h2><p className="mt-1 text-sm text-[var(--text-secondary)]">{contract.supplierName}</p></div><div className="text-right"><div className="eyebrow">Client</div><div className="mt-1 flex items-center gap-2 font-semibold"><Building2 className="size-4 text-[var(--accent)]" />{client?.name ?? 'Client indisponibil'}</div></div></section>

    <section className="grid grid-cols-3 gap-4" aria-label="Date contractuale">
      <InfoCard icon={CalendarRange} label="Perioadă efectivă" value={contract.period} />
      <InfoCard icon={Landmark} label="Valoare și monedă" value={formatMoney(contract.value.amount, contract.currency)} />
      <InfoCard icon={ReceiptText} label="Tip unitate / bază comercială" value={contract.unitType || 'Informație indisponibilă'} />
      <InfoCard icon={FileText} label="Termeni de plată" value={contract.paymentTerms || 'Informație indisponibilă'} />
      <InfoCard icon={FileText} label="Referință sursă" value={contract.sourceReference ?? 'Referință indisponibilă'} />
      <InfoCard icon={FileText} label="Metadate sursă" value={contract.sourceMetadata ?? 'Metadate opționale indisponibile'} />
    </section>

    {matchCandidate && <section className="card p-5"><div className="flex items-center justify-between"><div><p className="eyebrow">Context asociere pentru {contextInvoice?.documentNumber}</p><h3 className="mt-1 font-bold">Semnale demonstrative disponibile</h3></div><Badge tone="info">Încredere: {matchCandidate.confidence}</Badge></div><ul className="mt-4 grid grid-cols-2 gap-3">{matchCandidate.reasons.map((reason) => <li key={reason} className="rounded-lg border border-[var(--border)] bg-[var(--surface-subtle)] p-3 text-sm text-[var(--text-secondary)]">{reason}</li>)}</ul><p className="mt-4 text-xs text-[var(--text-muted)]">Semnalele sunt fictive și nu demonstrează o potrivire juridică sau contabilă. Vizualizarea nu rezolvă task-ul.</p></section>}

    <section className="card overflow-hidden"><div className="border-b border-[var(--border)] px-5 py-4"><h3 className="font-bold">Facturi asociate</h3><p className="mt-1 text-xs text-[var(--text-muted)]">Asocierea reflectă starea curentă comună a aplicației.</p></div>{associatedInvoices.length === 0 ? <div className="p-10 text-center text-sm text-[var(--text-secondary)]">Nicio factură nu este asociată în prezent acestui contract.</div> : <table className="w-full border-collapse text-left text-xs"><caption className="sr-only">Facturi asociate contractului</caption><thead className="bg-[var(--surface-subtle)] text-[10px] font-bold uppercase tracking-wider text-[var(--text-muted)]"><tr><th className="px-5 py-3">Număr factură</th><th className="px-4 py-3">Dată</th><th className="px-4 py-3">Valoare</th><th className="px-4 py-3">Pipeline</th><th className="px-5 py-3">Atenție</th></tr></thead><tbody className="divide-y divide-[var(--border)]">{associatedInvoices.map((invoice) => <tr key={invoice.id} className="hover:bg-[var(--surface-subtle)]"><td className="px-5 py-4"><Link to={`/invoices/${invoice.id}?tab=contract&returnTo=${encodeURIComponent(detailReturn)}`} className="font-bold text-[var(--accent)] outline-none hover:underline focus-visible:rounded focus-visible:ring-2 focus-visible:ring-[var(--focus)]">{invoice.documentNumber}</Link></td><td className="px-4 py-4">{formatDate(invoice.issueDate)}</td><td className="px-4 py-4 font-semibold">{formatMoney(invoice.total.amount, invoice.total.currency)}</td><td className="px-4 py-4"><PipelineBadge status={invoice.pipelineStatus} /></td><td className="px-5 py-4"><AttentionBadge invoice={invoice} /></td></tr>)}</tbody></table>}</section>
  </div>
}

function InfoCard({ icon: Icon, label, value }: { icon: typeof FileText; label: string; value: string }) { return <div className="card p-5"><Icon className="size-5 text-[var(--accent)]" /><div className="mt-4 text-xs text-[var(--text-muted)]">{label}</div><div className="mt-1 text-sm font-bold">{value}</div></div> }
function formatMoney(amount: number, currency: string) { return new Intl.NumberFormat('ro-RO', { style: 'currency', currency }).format(amount) }
function formatDate(value: string) { return new Intl.DateTimeFormat('ro-RO', { dateStyle: 'medium', timeZone: 'Europe/Bucharest' }).format(new Date(value)) }
