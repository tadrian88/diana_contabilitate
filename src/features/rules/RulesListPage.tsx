import { Search, SearchX, ShieldCheck, TriangleAlert } from 'lucide-react'
import { useMemo } from 'react'
import type { ReactNode } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { useClientScope } from '../../app/scope-context'
import { Badge } from '../../components/ui/badge'
import type { RuleCategory, RuleScope } from '../../domain/invoice'
import { useClients } from '../invoices/invoice-hooks'
import { useRules } from './rule-hooks'
import { RULE_CATEGORY_LABELS, RULE_SCOPE_LABELS } from './rule-view'

type VersionFilter = 'CURRENT' | 'HISTORICAL'
type ListRow = { rule: NonNullable<ReturnType<typeof useRules>['data']>[number]; version: NonNullable<ReturnType<typeof useRules>['data']>[number]['versions'][number]; current: boolean }

export function RulesListPage() {
  const { scope } = useClientScope()
  const { data: rules = [], isLoading, isError } = useRules(scope)
  const { data: clients = [] } = useClients()
  const [params, setParams] = useSearchParams()
  const query = params.get('q') ?? ''
  const category = parseCategory(params.get('category'))
  const ruleScope = parseScope(params.get('scope'))
  const versionFilter: VersionFilter = params.get('version') === 'HISTORICAL' ? 'HISTORICAL' : 'CURRENT'
  const clientId = params.get('client') ?? 'ALL'

  const rows = useMemo(() => rules.flatMap((rule): ListRow[] => {
    const latest = Math.max(...rule.versions.map((version) => version.version))
    return rule.versions.filter((version) => versionFilter === 'CURRENT' ? version.version === latest : version.version !== latest).map((version) => ({ rule, version, current: version.version === latest }))
  }).filter(({ rule, version }) => {
    const normalized = query.trim().toLocaleLowerCase('ro-RO')
    if (normalized && !`${rule.name} ${version.criteria} ${version.result}`.toLocaleLowerCase('ro-RO').includes(normalized)) return false
    if (category !== 'ALL' && rule.category !== category) return false
    if (ruleScope !== 'ALL' && rule.scope !== ruleScope) return false
    if (clientId !== 'ALL' && rule.scope === 'CLIENT_OVERRIDE' && rule.clientId !== clientId) return false
    return true
  }), [category, clientId, query, ruleScope, rules, versionFilter])

  const update = (key: string, value: string) => {
    const next = new URLSearchParams(params)
    value && value !== 'ALL' && !(key === 'version' && value === 'CURRENT') ? next.set(key, value) : next.delete(key)
    setParams(next)
  }
  const returnTo = `/rules${params.toString() ? `?${params.toString()}` : ''}`

  return <div className="space-y-6">
    <section className="flex items-end justify-between gap-6"><div><p className="eyebrow">Administrare explicabilă</p><h2 className="mt-2 text-2xl font-bold tracking-tight">Reguli de clasificare</h2><p className="mt-2 text-sm text-[var(--text-secondary)]">Inspectează domeniul, rezultatul și istoricul regulilor fictive fără execuție contabilă reală.</p></div><div className="text-right"><div className="text-2xl font-bold tabular-nums">{rows.length}</div><div className="text-xs text-[var(--text-muted)]">versiuni afișate</div></div></section>
    <section className="card overflow-hidden">
      <div className="flex flex-wrap items-center gap-3 border-b border-[var(--border)] p-4" aria-label="Filtre reguli">
        <label className="relative min-w-[300px] flex-1"><span className="sr-only">Caută reguli</span><Search className="absolute left-3 top-1/2 size-4 -translate-y-1/2 text-[var(--text-muted)]" /><input value={query} onChange={(event) => update('q', event.target.value)} placeholder="Caută nume, criteriu sau rezultat…" className="h-10 w-full rounded-lg border border-[var(--border-strong)] bg-[var(--surface)] pl-10 pr-3 text-sm outline-none focus:ring-2 focus:ring-[var(--focus)]" /></label>
        <Filter label="Categorie" value={category} onChange={(value) => update('category', value)}><option value="ALL">Toate categoriile</option>{Object.entries(RULE_CATEGORY_LABELS).map(([value, label]) => <option key={value} value={value}>{label}</option>)}</Filter>
        <Filter label="Domeniu regulă" value={ruleScope} onChange={(value) => update('scope', value)}><option value="ALL">Orice domeniu</option><option value="GLOBAL">Regulă globală</option><option value="CLIENT_OVERRIDE">Override client</option></Filter>
        {scope === 'all' && <Filter label="Client pentru override" value={clientId} onChange={(value) => update('client', value)}><option value="ALL">Toți clienții</option>{clients.map((client) => <option key={client.id} value={client.id}>{client.name}</option>)}</Filter>}
        <Filter label="Versiuni" value={versionFilter} onChange={(value) => update('version', value)}><option value="CURRENT">Versiuni curente</option><option value="HISTORICAL">Versiuni istorice</option></Filter>
      </div>
      {isLoading ? <LoadingState /> : isError ? <ErrorState /> : rules.length === 0 ? <NoRules /> : rows.length === 0 ? <NoResults overrideOnly={ruleScope === 'CLIENT_OVERRIDE'} onReset={() => setParams({})} /> : <div className="overflow-x-auto"><table className="w-full min-w-[1380px] border-collapse text-left text-xs"><caption className="sr-only">Reguli și versiuni din contextul activ.</caption><thead className="bg-[var(--surface-subtle)] text-[10px] font-bold uppercase tracking-wider text-[var(--text-muted)]"><tr><th className="px-4 py-3">Nume regulă</th><th className="px-4 py-3">Categorie</th><th className="px-4 py-3">Scope</th><th className="px-4 py-3">Client</th><th className="px-4 py-3">Versiune</th><th className="px-4 py-3">Efectiv de la</th><th className="px-4 py-3">Efectiv până la</th><th className="px-4 py-3">Rezultat</th><th className="px-4 py-3">Bază legală</th><th className="px-4 py-3">Ultima actualizare</th></tr></thead><tbody className="divide-y divide-[var(--border)]">{rows.map(({ rule, version, current }) => {
        const client = clients.find((candidate) => candidate.id === rule.clientId)
        const href = `/rules/${rule.id}${current ? '' : `?version=${version.version}`}${current ? `?returnTo=${encodeURIComponent(returnTo)}` : `&returnTo=${encodeURIComponent(returnTo)}`}`
        return <tr key={`${rule.id}-${version.version}`} className="hover:bg-[var(--surface-subtle)]"><td className="max-w-[220px] px-4 py-4"><Link to={href} className="font-bold text-[var(--accent)] outline-none hover:underline focus-visible:rounded focus-visible:ring-2 focus-visible:ring-[var(--focus)]">{rule.name}</Link><div className="mt-1 text-[var(--text-muted)]">{rule.reference}</div></td><td className="px-4 py-4">{RULE_CATEGORY_LABELS[rule.category]}</td><td className="px-4 py-4"><Badge tone={rule.scope === 'GLOBAL' ? 'neutral' : 'info'}>{RULE_SCOPE_LABELS[rule.scope]}</Badge></td><td className="max-w-[160px] px-4 py-4">{client?.name ?? (rule.scope === 'GLOBAL' ? 'Toți clienții' : 'Client indisponibil')}</td><td className="px-4 py-4"><Badge tone={current ? 'success' : 'neutral'}>Versiunea {version.version} · {current ? 'Curentă' : 'Istorică'}</Badge></td><td className="px-4 py-4">{formatDate(version.effectiveFrom)}</td><td className="px-4 py-4">{version.effectiveTo ? formatDate(version.effectiveTo) : 'Fără dată finală'}</td><td className="max-w-[190px] px-4 py-4 font-semibold">{version.result}</td><td className="max-w-[190px] px-4 py-4 text-[var(--text-secondary)]">{version.legalBasis}</td><td className="whitespace-nowrap px-4 py-4">{formatDateTime(version.createdAt)}</td></tr>
      })}</tbody></table></div>}
    </section>
  </div>
}

function Filter({ label, value, onChange, children }: { label: string; value: string; onChange: (value: string) => void; children: ReactNode }) { return <label><span className="sr-only">{label}</span><select aria-label={label} value={value} onChange={(event) => onChange(event.target.value)} className="h-10 min-w-40 rounded-lg border border-[var(--border-strong)] bg-[var(--surface)] px-3 text-sm font-medium outline-none focus:ring-2 focus:ring-[var(--focus)]">{children}</select></label> }
function LoadingState() { return <div className="space-y-2 p-4" aria-label="Se încarcă regulile">{Array.from({ length: 5 }, (_, index) => <div key={index} className="h-16 animate-pulse rounded-lg bg-[var(--surface-subtle)]" />)}</div> }
function ErrorState() { return <div className="p-12 text-center"><TriangleAlert className="mx-auto size-8 text-[var(--danger)]" /><h3 className="mt-3 font-bold">Regulile nu au putut fi încărcate</h3><p className="mt-1 text-sm text-[var(--text-secondary)]">Repository-ul demonstrativ a returnat o eroare.</p></div> }
function NoRules() { return <div className="p-12 text-center"><ShieldCheck className="mx-auto size-8 text-[var(--text-muted)]" /><h3 className="mt-3 font-bold">Nu există reguli în acest context</h3></div> }
function NoResults({ overrideOnly, onReset }: { overrideOnly: boolean; onReset: () => void }) { return <div className="p-12 text-center"><SearchX className="mx-auto size-8 text-[var(--text-muted)]" /><h3 className="mt-3 font-bold">{overrideOnly ? 'Nu există override-uri pentru selecția curentă' : 'Niciun rezultat'}</h3><button onClick={onReset} className="mt-4 rounded-lg border border-[var(--border-strong)] px-3 py-2 text-sm font-semibold focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]">Resetează filtrele</button></div> }
function parseCategory(value: string | null): RuleCategory | 'ALL' { return value === 'ACCOUNT' || value === 'VAT' || value === 'DEDUCTIBILITY' ? value : 'ALL' }
function parseScope(value: string | null): RuleScope | 'ALL' { return value === 'GLOBAL' || value === 'CLIENT_OVERRIDE' ? value : 'ALL' }
function formatDate(value: string) { return new Intl.DateTimeFormat('ro-RO', { dateStyle: 'medium', timeZone: 'Europe/Bucharest' }).format(new Date(`${value}T00:00:00.000Z`)) }
function formatDateTime(value: string) { return new Intl.DateTimeFormat('ro-RO', { dateStyle: 'medium', timeStyle: 'short', timeZone: 'Europe/Bucharest' }).format(new Date(value)) }
