import { AlertCircle, ArrowRight, Building2 } from 'lucide-react'
import { useState } from 'react'
import { lifecycleLabels } from '../../domain/client-management'
import { useClientDetail } from './client-management-hooks'
import { Link } from 'react-router-dom'
import { useClientScope } from '../../app/scope-context'
import { Badge } from '../../components/ui/badge'
import { useContracts } from '../contracts/contract-hooks'
import { useClients, useInvoices } from '../invoices/invoice-hooks'
import { useRules } from '../rules/rule-hooks'
import { selectClientOperations } from './client-selectors'

export function ClientsListPage() {
  const [search,setSearch]=useState('');const [status,setStatus]=useState('ALL')
  const { scope, setScope } = useClientScope()
  const clientQuery = useClients()
  const invoiceQuery = useInvoices('all')
  const contractQuery = useContracts('all')
  const ruleQuery = useRules('all')
  const isLoading = clientQuery.isLoading || invoiceQuery.isLoading || contractQuery.isLoading || ruleQuery.isLoading
  const isError = clientQuery.isError || invoiceQuery.isError || contractQuery.isError || ruleQuery.isError
  const clients = (clientQuery.data ?? []).filter((client) => (scope === 'all' || client.id === scope) && (status==='ALL'||(client.status??'ACTIVE')===status) && `${client.name} ${client.cui}`.toLocaleLowerCase('ro-RO').includes(search.toLocaleLowerCase('ro-RO')))
  const rows = clients.map((client) => selectClientOperations(client, invoiceQuery.data ?? [], contractQuery.data ?? [], ruleQuery.data ?? []))

  return <div className="space-y-6">
    <section className="flex items-end justify-between gap-6"><div><p className="eyebrow">Context multi-client</p><h2 className="mt-2 text-2xl font-bold tracking-tight">Clienți</h2><p className="mt-2 text-sm text-[var(--text-secondary)]">Gestionare companii și configurare contabilă, ANAF / SPV și export SAGA.</p></div><Link to="/clients/new" className="rounded-lg bg-[var(--accent)] px-4 py-2 font-semibold text-white">Adaugă client</Link><Badge tone="neutral">{scope === 'all' ? 'Toți clienții' : 'Client selectat'}</Badge></section>
    <div className="flex gap-3"><input aria-label="Caută client după denumire sau CUI" placeholder="Caută denumire sau CUI" className="rounded-lg border border-[var(--border)] bg-[var(--surface)] p-2" value={search} onChange={e=>setSearch(e.target.value)}/><select aria-label="Filtrează după starea clientului" className="rounded-lg border border-[var(--border)] bg-[var(--surface)] p-2" value={status} onChange={e=>setStatus(e.target.value)}><option value="ALL">Toate stările</option>{Object.entries(lifecycleLabels).map(([s,label])=><option key={s} value={s}>{label}</option>)}</select></div>
    <section className="card overflow-hidden" aria-labelledby="clients-list-heading">
      <h3 id="clients-list-heading" className="sr-only">Lista clienților</h3>
      {isLoading ? <LoadingState /> : isError ? <ErrorState /> : rows.length === 0 ? <EmptyState /> : <div className="overflow-x-auto"><table className="w-full min-w-[980px] border-collapse text-left"><caption className="sr-only">Clienți și indicatori operaționali derivați.</caption><thead className="bg-[var(--surface-subtle)] text-[11px] font-bold uppercase tracking-wider text-[var(--text-muted)]"><tr><th className="px-5 py-3">Client</th><th className="px-4 py-3">Facturi în procesare</th><th className="px-4 py-3">Task-uri deschise</th><th className="px-4 py-3">În așteptare</th><th className="px-4 py-3">Pregătite pentru SAGA</th><th className="px-4 py-3">Exportate în SAGA</th><th className="px-5 py-3 text-right">Acțiune</th></tr></thead><tbody className="divide-y divide-[var(--border)]">{rows.map(({ client, counts }) => <tr key={client.id} data-client={client.id} className="hover:bg-[var(--surface-subtle)]"><td className="px-5 py-4"><Link to={`/clients/${client.id}`} onClick={() => setScope(client.id)} className="font-bold text-[var(--accent)] outline-none hover:underline focus-visible:rounded focus-visible:ring-2 focus-visible:ring-[var(--focus)]">{client.name}</Link><div className="mt-1 text-xs text-[var(--text-muted)]"><span>{client.cui}</span> · {lifecycleLabels[client.status??'ACTIVE']}</div><ClientSummary clientId={client.id}/></td><Count value={counts.processing} /><Count value={counts.open} attention /><Count value={counts.waiting} /><Count value={counts.ready} /><Count value={counts.exported} /><td className="px-5 py-4 text-right"><Link to={`/clients/${client.id}`} onClick={() => setScope(client.id)} className="inline-flex items-center gap-2 rounded-lg px-3 py-2 text-xs font-semibold text-[var(--accent)] outline-none hover:bg-[var(--info-soft)] focus-visible:ring-2 focus-visible:ring-[var(--focus)]">Vezi context<ArrowRight className="size-3.5" /></Link></td></tr>)}</tbody></table></div>}
    </section>
  </div>
}

function Count({ value, attention = false }: { value: number; attention?: boolean }) { return <td className={`px-4 py-4 text-sm font-bold tabular-nums ${attention && value > 0 ? 'text-[var(--warning)]' : ''}`}>{value}</td> }
function LoadingState() { return <div className="space-y-3 p-5" aria-label="Se încarcă clienții">{[1, 2].map((item) => <div key={item} className="h-16 animate-pulse rounded-lg bg-[var(--surface-subtle)]" />)}</div> }
function ErrorState() { return <div className="p-12 text-center"><AlertCircle className="mx-auto size-8 text-[var(--danger)]" /><h3 className="mt-3 font-bold">Clienții nu au putut fi încărcați</h3><p className="mt-1 text-sm text-[var(--text-secondary)]">Serviciul de date nu este disponibil. Verifică starea API-ului.</p></div> }
function EmptyState() { return <div className="p-12 text-center"><Building2 className="mx-auto size-8 text-[var(--text-muted)]" /><h3 className="mt-3 font-bold">Nu există clienți în contextul activ</h3></div> }

function ClientSummary({clientId}:{clientId:string}){const q=useClientDetail(clientId);if(q.isLoading)return <p className="text-xs">Se încarcă configurarea…</p>;if(!q.data)return <p className="text-xs">Stare configurare indisponibilă</p>;const r=q.data.onboarding;return <p className="mt-1 text-xs text-[var(--text-muted)]">ANAF: {r.anaf.status==='READY'?'autorizat':'configurare necesară'} · SAGA: {r.sagaConfigurationReady?'export configurat':'neconfigurat'} · {r.overall==='CONFIGURATION_INCOMPLETE'?'configurare parțială':r.classification.status!=='READY'?'reguli producție în așteptare':'automatizare configurată'}</p>}
