import { FileSearch, Search, SearchX, TriangleAlert } from 'lucide-react'
import { useMemo } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { useClientScope } from '../../app/scope-context'
import { useClients } from '../invoices/invoice-hooks'
import { useContracts } from './contract-hooks'
import { ContractIngestionSection } from './ContractIngestionSection'

export function ContractsListPage() {
  const { scope } = useClientScope()
  const { data: contracts = [], isLoading, isError } = useContracts(scope)
  const { data: clients = [] } = useClients()
  const [params, setParams] = useSearchParams()
  const query = params.get('q') ?? ''
  const supplier = params.get('supplier') ?? 'ALL'
  const suppliers = [...new Set(contracts.map((contract) => contract.supplierName))].sort((left, right) => left.localeCompare(right, 'ro'))
  const visibleContracts = useMemo(() => {
    const normalized = query.trim().toLocaleLowerCase('ro-RO')
    return contracts.filter((contract) => {
      if (normalized && !`${contract.reference} ${contract.supplierName}`.toLocaleLowerCase('ro-RO').includes(normalized)) return false
      return supplier === 'ALL' || contract.supplierName === supplier
    })
  }, [contracts, query, supplier])

  const update = (key: string, value: string) => {
    const next = new URLSearchParams(params)
    value && value !== 'ALL' ? next.set(key, value) : next.delete(key)
    setParams(next)
  }
  const returnTo = `/contracts${params.toString() ? `?${params.toString()}` : ''}`

  return <div className="space-y-6">
    <section className="flex items-end justify-between gap-6"><div><p className="eyebrow">Context contractual</p><h2 className="mt-2 text-2xl font-bold tracking-tight">Contracte</h2><p className="mt-2 text-sm text-[var(--text-secondary)]">Consultă datele contractuale folosite în fluxul demonstrativ de asociere.</p></div><div className="text-right"><div className="text-2xl font-bold tabular-nums">{visibleContracts.length}</div><div className="text-xs text-[var(--text-muted)]">din {contracts.length} contracte în context</div></div></section>
    <ContractIngestionSection clientId={scope==='all'?'':scope}/>
    <section className="card overflow-hidden">
      <div className="flex items-center gap-3 border-b border-[var(--border)] p-4" aria-label="Filtre contracte">
        <label className="relative min-w-[340px] flex-1"><span className="sr-only">Caută după referință sau furnizor</span><Search className="absolute left-3 top-1/2 size-4 -translate-y-1/2 text-[var(--text-muted)]" /><input value={query} onChange={(event) => update('q', event.target.value)} placeholder="Caută referință sau furnizor…" className="h-10 w-full rounded-lg border border-[var(--border-strong)] bg-[var(--surface)] pl-10 pr-3 text-sm outline-none focus:ring-2 focus:ring-[var(--focus)]" /></label>
        <label><span className="sr-only">Filtrează după furnizor</span><select aria-label="Filtrează după furnizor" value={supplier} onChange={(event) => update('supplier', event.target.value)} className="h-10 min-w-64 rounded-lg border border-[var(--border-strong)] bg-[var(--surface)] px-3 text-sm font-medium outline-none focus:ring-2 focus:ring-[var(--focus)]"><option value="ALL">Toți furnizorii</option>{suppliers.map((name) => <option key={name} value={name}>{name}</option>)}</select></label>
      </div>
      {isLoading ? <LoadingState /> : isError ? <ErrorState /> : contracts.length === 0 ? <EmptyState /> : visibleContracts.length === 0 ? <NoResults onReset={() => setParams({})} /> : <div className="overflow-x-auto"><table className="w-full min-w-[1280px] border-collapse text-left text-xs"><caption className="sr-only">Contractele din contextul activ de client.</caption><thead className="bg-[var(--surface-subtle)] text-[10px] font-bold uppercase tracking-wider text-[var(--text-muted)]"><tr><th className="px-4 py-3">Referință contract</th><th className="px-4 py-3">Client</th><th className="px-4 py-3">Furnizor</th><th className="px-4 py-3">Perioadă</th><th className="px-4 py-3">Monedă</th><th className="px-4 py-3">Valoare</th><th className="px-4 py-3">Tip unitate / bază comercială</th><th className="px-4 py-3">Termeni de plată</th><th className="px-4 py-3">Sursă</th></tr></thead><tbody className="divide-y divide-[var(--border)]">{visibleContracts.map((contract) => {
        const client = clients.find((candidate) => candidate.id === contract.clientId)
        const href = `/contracts/${contract.id}?returnTo=${encodeURIComponent(returnTo)}`
        return <tr key={contract.id} className="hover:bg-[var(--surface-subtle)]"><td className="px-4 py-4"><Link to={href} className="font-bold text-[var(--accent)] outline-none hover:underline focus-visible:rounded focus-visible:ring-2 focus-visible:ring-[var(--focus)]">{contract.reference}</Link></td><td className="max-w-[170px] px-4 py-4 font-medium">{client?.name ?? 'Client indisponibil'}</td><td className="max-w-[190px] px-4 py-4 font-semibold">{contract.supplierName}</td><td className="whitespace-nowrap px-4 py-4">{contract.period}</td><td className="px-4 py-4">{contract.currency}</td><td className="whitespace-nowrap px-4 py-4 font-semibold tabular-nums">{contract.hasLegacyTotalValue===false?'Vezi tarife':formatMoney(contract.value.amount, contract.value.currency)}</td><td className="px-4 py-4">{contract.unitType || 'Informație indisponibilă'}</td><td className="px-4 py-4">{contract.paymentTerms || 'Informație indisponibilă'}</td><td className="max-w-[190px] px-4 py-4"><div className="font-medium">{contract.sourceReference ?? 'Referință indisponibilă'}</div><div className="mt-1 text-[var(--text-muted)]">{contract.sourceMetadata ?? 'Metadate opționale indisponibile'}</div></td></tr>
      })}</tbody></table></div>}
    </section>
  </div>
}

function LoadingState() { return <div className="space-y-2 p-4" aria-label="Se încarcă contractele">{Array.from({ length: 5 }, (_, index) => <div key={index} className="h-16 animate-pulse rounded-lg bg-[var(--surface-subtle)]" />)}</div> }
function ErrorState() { return <div className="p-12 text-center"><TriangleAlert className="mx-auto size-8 text-[var(--danger)]" /><h3 className="mt-3 font-bold">Contractele nu au putut fi încărcate</h3><p className="mt-1 text-sm text-[var(--text-secondary)]">Serviciul de date nu este disponibil. Verifică starea API-ului.</p></div> }
function EmptyState() { return <div className="p-12 text-center"><FileSearch className="mx-auto size-8 text-[var(--text-muted)]" /><h3 className="mt-3 font-bold">Nu există contracte în acest context</h3><p className="mt-1 text-sm text-[var(--text-secondary)]">Nu au fost găsite contracte pentru clientul selectat.</p></div> }
function NoResults({ onReset }: { onReset: () => void }) { return <div className="p-12 text-center"><SearchX className="mx-auto size-8 text-[var(--text-muted)]" /><h3 className="mt-3 font-bold">Niciun rezultat</h3><p className="mt-1 text-sm text-[var(--text-secondary)]">Căutarea sau filtrul nu corespunde niciunui contract.</p><button onClick={onReset} className="mt-4 rounded-lg border border-[var(--border-strong)] px-3 py-2 text-sm font-semibold focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]">Resetează filtrele</button></div> }
function formatMoney(amount: number, currency: string) { return new Intl.NumberFormat('ro-RO', { style: 'currency', currency }).format(amount) }
