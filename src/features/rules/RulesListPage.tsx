import { BookOpen, Database, Search, SearchX, ShieldCheck, TriangleAlert } from 'lucide-react'
import { useMemo } from 'react'
import type { ReactNode } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { useClientScope } from '../../app/scope-context'
import { Badge } from '../../components/ui/badge'
import type { RuleCategory, RuleScope } from '../../domain/invoice'
import { useClients } from '../invoices/invoice-hooks'
import { useApprovedKnowledge, useLegislationSources, useRevokeKnowledge, useRules } from './rule-hooks'
import { RULE_CATEGORY_LABELS, RULE_SCOPE_LABELS } from './rule-view'

type VersionFilter = 'CURRENT' | 'HISTORICAL'
type ListRow = { rule: NonNullable<ReturnType<typeof useRules>['data']>[number]; version: NonNullable<ReturnType<typeof useRules>['data']>[number]['versions'][number]; current: boolean }

export function RulesListPage() {
  const { scope } = useClientScope()
  const { data: rules = [], isLoading, isError } = useRules(scope)
  const {data:knowledge=[],isLoading:knowledgeLoading,isError:knowledgeError}=useApprovedKnowledge(scope)
  const {data:legislation=[],isLoading:legislationLoading,isError:legislationError}=useLegislationSources()
  const revoke=useRevokeKnowledge()
  const { data: clients = [] } = useClients()
  const [params, setParams] = useSearchParams()
  const query = params.get('q') ?? ''
  const category = parseCategory(params.get('category'))
  const ruleScope = parseScope(params.get('scope'))
  const versionFilter: VersionFilter = params.get('version') === 'HISTORICAL' ? 'HISTORICAL' : 'CURRENT'
  const clientId = params.get('client') ?? 'ALL'

  const rows = useMemo(() => rules.flatMap((rule): ListRow[] => {
    const verified=rule.versions.filter(version=>version.productionEligible);if(!verified.length)return []
    const latest = Math.max(...verified.map((version) => version.version))
    return rule.versions.filter(version=>version.productionEligible).filter((version) => versionFilter === 'CURRENT' ? version.version === latest : version.version !== latest).map((version) => ({ rule, version, current: version.version === latest }))
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

  const verifiedCount=rules.flatMap(rule=>rule.versions).filter(version=>version.productionEligible).length
  return <div className="space-y-8">
    <section className="flex items-end justify-between gap-6"><div><p className="eyebrow">Administrare explicabilă</p><h2 className="mt-2 text-2xl font-bold tracking-tight">Reguli și surse</h2><p className="mt-2 text-sm text-[var(--text-secondary)]">Regulile executabile, deciziile promovate explicit și sursele legislative au roluri distincte.</p></div></section>
    <section aria-labelledby="verified-rules"><div className="mb-3"><h3 id="verified-rules" className="text-lg font-bold">Reguli verificate</h3><p className="text-sm text-[var(--text-secondary)]">Logică deterministă aprobată, cu predicate și rezultate explicite.</p></div>
    <section className="card overflow-hidden">
      <div className="flex flex-wrap items-center gap-3 border-b border-[var(--border)] p-4" aria-label="Filtre reguli">
        <label className="relative min-w-[300px] flex-1"><span className="sr-only">Caută reguli</span><Search className="absolute left-3 top-1/2 size-4 -translate-y-1/2 text-[var(--text-muted)]" /><input value={query} onChange={(event) => update('q', event.target.value)} placeholder="Caută nume, criteriu sau rezultat…" className="h-10 w-full rounded-lg border border-[var(--border-strong)] bg-[var(--surface)] pl-10 pr-3 text-sm outline-none focus:ring-2 focus:ring-[var(--focus)]" /></label>
        <Filter label="Categorie" value={category} onChange={(value) => update('category', value)}><option value="ALL">Toate categoriile</option>{Object.entries(RULE_CATEGORY_LABELS).map(([value, label]) => <option key={value} value={value}>{label}</option>)}</Filter>
        <Filter label="Domeniu regulă" value={ruleScope} onChange={(value) => update('scope', value)}><option value="ALL">Orice domeniu</option><option value="GLOBAL">Regulă globală</option><option value="CLIENT_OVERRIDE">Override client</option></Filter>
        {scope === 'all' && <Filter label="Client pentru override" value={clientId} onChange={(value) => update('client', value)}><option value="ALL">Toți clienții</option>{clients.map((client) => <option key={client.id} value={client.id}>{client.name}</option>)}</Filter>}
        <Filter label="Versiuni" value={versionFilter} onChange={(value) => update('version', value)}><option value="CURRENT">Versiuni curente</option><option value="HISTORICAL">Versiuni istorice</option></Filter>
      </div>
      {isLoading ? <LoadingState /> : isError ? <ErrorState /> : verifiedCount === 0 ? <NoRules /> : rows.length === 0 ? <NoResults overrideOnly={ruleScope === 'CLIENT_OVERRIDE'} onReset={() => setParams({})} /> : <div className="overflow-x-auto"><table className="w-full min-w-[1380px] border-collapse text-left text-xs"><caption className="sr-only">Reguli verificate și versiuni din contextul activ.</caption><thead className="bg-[var(--surface-subtle)] text-[10px] font-bold uppercase tracking-wider text-[var(--text-muted)]"><tr><th className="px-4 py-3">Nume regulă</th><th className="px-4 py-3">Categorie</th><th className="px-4 py-3">Scope</th><th className="px-4 py-3">Client</th><th className="px-4 py-3">Versiune</th><th className="px-4 py-3">Efectiv de la</th><th className="px-4 py-3">Efectiv până la</th><th className="px-4 py-3">Rezultat</th><th className="px-4 py-3">Bază legală</th><th className="px-4 py-3">Ultima actualizare</th></tr></thead><tbody className="divide-y divide-[var(--border)]">{rows.map(({ rule, version, current }) => {
        const client = clients.find((candidate) => candidate.id === rule.clientId)
        const href = `/rules/${rule.id}${current ? '' : `?version=${version.version}`}${current ? `?returnTo=${encodeURIComponent(returnTo)}` : `&returnTo=${encodeURIComponent(returnTo)}`}`
        return <tr key={`${rule.id}-${version.version}`} className="hover:bg-[var(--surface-subtle)]"><td className="max-w-[220px] px-4 py-4"><Link to={href} className="font-bold text-[var(--accent)] outline-none hover:underline focus-visible:rounded focus-visible:ring-2 focus-visible:ring-[var(--focus)]">{rule.name}</Link><div className="mt-1 text-[var(--text-muted)]">{rule.reference}</div><Badge tone="success">Verificată pentru producție</Badge></td><td className="px-4 py-4">{RULE_CATEGORY_LABELS[rule.category]}</td><td className="px-4 py-4"><Badge tone={rule.scope === 'GLOBAL' ? 'neutral' : 'info'}>{RULE_SCOPE_LABELS[rule.scope]}</Badge></td><td className="max-w-[160px] px-4 py-4">{client?.name ?? (rule.scope === 'GLOBAL' ? 'Toți clienții' : 'Client indisponibil')}</td><td className="px-4 py-4"><Badge tone={current ? 'success' : 'neutral'}>Versiunea {version.version} · {current ? 'Curentă' : 'Istorică'}</Badge></td><td className="px-4 py-4">{formatDate(version.effectiveFrom)}</td><td className="px-4 py-4">{version.effectiveTo ? formatDate(version.effectiveTo) : 'Fără dată finală'}</td><td className="max-w-[190px] px-4 py-4 font-semibold">{version.result}</td><td className="max-w-[190px] px-4 py-4 text-[var(--text-secondary)]">{version.legalBasis}</td><td className="whitespace-nowrap px-4 py-4">{formatDateTime(version.createdAt)}</td></tr>
      })}</tbody></table></div>}
    </section></section>
    <section aria-labelledby="reusable-decisions"><div className="mb-3"><h3 id="reusable-decisions" className="text-lg font-bold">Decizii reutilizabile</h3><p className="text-sm text-[var(--text-secondary)]">Knowledge per client și dimensiune, creat numai după confirmarea explicită a contabilului.</p></div><div className="card overflow-hidden">{knowledgeLoading?<LoadingState/>:knowledgeError?<SectionError title="Deciziile reutilizabile nu au putut fi încărcate"/>:knowledge.length===0?<div className="p-10 text-center"><Database className="mx-auto size-8 text-[var(--text-muted)]"/><p className="mt-3 font-bold">Nu există încă decizii reutilizabile.</p></div>:<div className="divide-y divide-[var(--border)]">{knowledge.map(item=><article key={item.id} className="p-4 text-sm"><div className="flex flex-wrap items-start justify-between gap-3"><div><div className="flex gap-2"><Badge tone={item.status==='ACTIVE'?'success':item.status==='STALE'?'warning':'neutral'}>{item.status}</Badge><strong>{RULE_CATEGORY_LABELS[item.dimension]}: {knowledgeValue(item.value)}</strong></div><p className="mt-2 text-[var(--text-secondary)]">Client: {item.scope.clientDisplay} · Furnizor: {item.scope.supplierDisplay}</p><p className="mt-1 text-[var(--text-secondary)]">Identitate exactă: {identityLabel(item.scope.serviceIdentityKind)} = {item.scope.serviceIdentityValue}</p><details className="mt-2 text-xs"><summary className="cursor-pointer font-semibold">Când se aplică și de unde provine?</summary><p className="mt-2">{item.scope.documentType} · {item.scope.currency} · TVA {item.scope.vatRate} · profil v{item.scope.profileVersion}</p><p className="mt-1">Factura sursă: {item.sourceInvoiceId} · promovată de {item.promotedBy} la {formatDateTime(item.promotedAt)}</p></details></div>{item.status!=='REVOKED'&&<button className="rounded-lg border border-[var(--border-strong)] px-3 py-2 text-xs font-semibold" disabled={revoke.isPending} onClick={()=>revoke.mutate({clientId:item.scope.clientId,item})}>Nu mai folosi automat</button>}</div></article>)}</div>}</div></section>
    <section aria-labelledby="legislation-sources"><div className="mb-3"><h3 id="legislation-sources" className="text-lg font-bold">Surse legislative</h3><p className="text-sm text-[var(--text-secondary)]">Corpus legislativ global, comun tuturor clienților: documente versionate pentru evidence, retrieval și citări. Fragmentele nu sunt reguli contabile executabile.</p></div><div className="card overflow-hidden">{legislationLoading?<LoadingState/>:legislationError?<SectionError title="Sursele legislative nu au putut fi încărcate"/>:legislation.length===0?<div className="p-10 text-center"><BookOpen className="mx-auto size-8 text-[var(--text-muted)]"/><p className="mt-3 font-bold">Nu există surse legislative importate.</p></div>:<div className="divide-y divide-[var(--border)]">{legislation.map(item=><article key={item.versionId} className="flex flex-wrap items-center justify-between gap-4 p-4 text-sm"><div>{item.officialUrl.startsWith('https://')?<a href={item.officialUrl} className="font-bold text-[var(--accent)]" target="_blank" rel="noreferrer">{item.title}</a>:<span className="font-bold">{item.title}</span>}<p className="mt-1 text-[var(--text-secondary)]">{item.versionLabel} · {item.issuer} · efectivă de la {formatDate(item.effectiveFrom)}</p></div><div className="text-right">{item.testOnly&&<Badge tone="warning" className="mr-2">TEST_ONLY</Badge>}<Badge tone={item.status==='ACTIVE'?'success':'neutral'}>{item.status}</Badge><p className="mt-1 text-xs text-[var(--text-muted)]">{item.fragmentCount} fragmente de evidence</p></div></article>)}</div>}</div></section>
  </div>
}

function Filter({ label, value, onChange, children }: { label: string; value: string; onChange: (value: string) => void; children: ReactNode }) { return <label><span className="sr-only">{label}</span><select aria-label={label} value={value} onChange={(event) => onChange(event.target.value)} className="h-10 min-w-40 rounded-lg border border-[var(--border-strong)] bg-[var(--surface)] px-3 text-sm font-medium outline-none focus:ring-2 focus:ring-[var(--focus)]">{children}</select></label> }
function LoadingState() { return <div className="space-y-2 p-4" aria-label="Se încarcă regulile">{Array.from({ length: 5 }, (_, index) => <div key={index} className="h-16 animate-pulse rounded-lg bg-[var(--surface-subtle)]" />)}</div> }
function ErrorState() { return <div className="p-12 text-center"><TriangleAlert className="mx-auto size-8 text-[var(--danger)]" /><h3 className="mt-3 font-bold">Regulile nu au putut fi încărcate</h3><p className="mt-1 text-sm text-[var(--text-secondary)]">Serviciul de date nu este disponibil. Verifică starea API-ului.</p></div> }
function SectionError({ title }: { title: string }) { return <div role="alert" className="p-10 text-center"><TriangleAlert className="mx-auto size-8 text-[var(--danger)]" /><p className="mt-3 font-bold">{title}</p><p className="mt-1 text-sm text-[var(--text-secondary)]">Serviciul de date nu este disponibil. Verifică starea API-ului.</p></div> }
function NoRules() { return <div className="p-12 text-center"><ShieldCheck className="mx-auto size-8 text-[var(--text-muted)]" /><h3 className="mt-3 font-bold">Nu există încă reguli verificate.</h3><p className="mt-1 text-sm text-[var(--text-secondary)]">Regulile verificate sunt logică deterministă aprobată pentru producție. Deocamdată clasificarea folosește analiza asistată (legislație + AI) și deciziile reutilizabile confirmate de contabil.</p></div> }
function NoResults({ overrideOnly, onReset }: { overrideOnly: boolean; onReset: () => void }) { return <div className="p-12 text-center"><SearchX className="mx-auto size-8 text-[var(--text-muted)]" /><h3 className="mt-3 font-bold">{overrideOnly ? 'Nu există override-uri pentru selecția curentă' : 'Niciun rezultat'}</h3><button onClick={onReset} className="mt-4 rounded-lg border border-[var(--border-strong)] px-3 py-2 text-sm font-semibold focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]">Resetează filtrele</button></div> }
function parseCategory(value: string | null): RuleCategory | 'ALL' { return value === 'ACCOUNT' || value === 'VAT' || value === 'DEDUCTIBILITY' || value === 'VAT_TREATMENT' || value === 'VAT_DEDUCTIBILITY' || value === 'EXPENSE_TAX_TREATMENT' ? value : 'ALL' }
function parseScope(value: string | null): RuleScope | 'ALL' { return value === 'GLOBAL' || value === 'CLIENT_OVERRIDE' ? value : 'ALL' }
function formatDate(value: string) { return new Intl.DateTimeFormat('ro-RO', { dateStyle: 'medium', timeZone: 'Europe/Bucharest' }).format(new Date(`${value}T00:00:00.000Z`)) }
function formatDateTime(value: string) { return new Intl.DateTimeFormat('ro-RO', { dateStyle: 'medium', timeStyle: 'short', timeZone: 'Europe/Bucharest' }).format(new Date(value)) }
function knowledgeValue(value:import('../../domain/invoice').DomainValue){return value.account??value.kind}
function identityLabel(value:string){return ({SELLER_ITEM_ID:'ID articol furnizor',STANDARD_ITEM_ID:'ID standard',NORMALIZED_DESCRIPTION:'Descriere normalizată'} as Record<string,string>)[value]??value}
