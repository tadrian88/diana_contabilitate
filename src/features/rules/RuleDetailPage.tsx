import { LegalSourceLink } from './LegalSourceLink'
import { ArrowLeft, CalendarRange, GitBranch, Scale, ShieldCheck, UserRound } from 'lucide-react'
import { useState } from 'react'
import { Link, useLocation, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { Badge } from '../../components/ui/badge'
import { Button } from '../../components/ui/button'
import type { RuleVersion } from '../../domain/invoice'
import { useClients } from '../invoices/invoice-hooks'
import { ClientOverrideDialog, NewVersionDialog } from './RuleDialogs'
import { useRule } from './rule-hooks'
import { RULE_CATEGORY_LABELS, RULE_SCOPE_LABELS } from './rule-view'

export function RuleDetailPage() {
  const { ruleId = '' } = useParams()
  const [params] = useSearchParams()
  const location = useLocation()
  const navigate = useNavigate()
  const { data: rule, isLoading, isError } = useRule(ruleId)
  const { data: clients = [] } = useClients()
  const [versionOpen, setVersionOpen] = useState(false)
  const [overrideOpen, setOverrideOpen] = useState(false)
  const requestedReturn = params.get('returnTo')
  const returnTo = requestedReturn?.startsWith('/rules') || requestedReturn?.startsWith('/invoices/') ? requestedReturn : '/rules'

  if (isLoading) return <div className="card h-96 animate-pulse bg-[var(--surface-subtle)]" aria-label="Se încarcă regula" />
  if (isError) return <div className="card p-10 text-center"><h2 className="font-bold">Regula nu a putut fi încărcată</h2><p className="mt-2 text-sm text-[var(--text-secondary)]">Serviciul de date nu este disponibil. Verifică starea API-ului.</p></div>
  if (!rule) return <div className="card p-10 text-center"><h2 className="text-lg font-bold">Regula nu a fost găsită</h2><Link to="/rules" className="mt-4 inline-block text-sm font-semibold text-[var(--accent)]">Înapoi la Reguli</Link></div>

  const latestNumber = Math.max(...rule.versions.map((version) => version.version))
  const requestedVersion = params.get('version') ? Number(params.get('version')) : latestNumber
  const selected = rule.versions.find((version) => version.version === requestedVersion)
  if (!selected) return <div className="card p-10 text-center"><h2 className="text-lg font-bold">Versiunea istorică nu este disponibilă</h2><Link to={`/rules/${rule.id}`} className="mt-4 inline-block text-sm font-semibold text-[var(--accent)]">Vezi versiunea curentă</Link></div>
  const current = selected.version === latestNumber
  const client = clients.find((candidate) => candidate.id === rule.clientId)
  const detailReturn = `${location.pathname}${location.search}`

  return <div className="space-y-5">
    <Link to={returnTo} className="inline-flex items-center gap-2 rounded text-sm font-semibold text-[var(--text-secondary)] outline-none hover:text-[var(--accent)] focus-visible:ring-2 focus-visible:ring-[var(--focus)]"><ArrowLeft className="size-4" />{returnTo.startsWith('/invoices/') ? 'Înapoi la clasificarea facturii' : 'Înapoi la lista regulilor'}</Link>
    <section className="flex items-start justify-between gap-8"><div><div className="flex flex-wrap items-center gap-2"><Badge tone={rule.scope === 'GLOBAL' ? 'neutral' : 'info'}>{RULE_SCOPE_LABELS[rule.scope]}</Badge><Badge tone="neutral">{RULE_CATEGORY_LABELS[rule.category]}</Badge><Badge tone={selected.productionEligible ? "success" : "warning"}>{selected.productionEligible ? "Verificată pentru producție" : "Demo / neverificată — fără automatizare în producție"}</Badge><Badge tone={current ? 'success' : 'neutral'}>Versiunea {selected.version} · {current ? 'Curentă' : 'Istorică'}</Badge></div><h2 className="mt-3 text-2xl font-bold tracking-tight">{rule.name}</h2><p className="mt-1 text-sm text-[var(--text-secondary)]">{rule.reference}{client ? ` · ${client.name}` : ' · aplicabilitate globală de bază'}</p></div>{current && <div className="flex gap-3"><Button variant="secondary" onClick={() => setVersionOpen(true)}><GitBranch className="size-4" />Creează versiune nouă</Button>{rule.scope === 'GLOBAL' && <Button onClick={() => setOverrideOpen(true)}><UserRound className="size-4" />Creează override pentru client</Button>}</div>}</section>

    {!current && <div className="rounded-xl border border-[var(--border)] bg-[var(--surface-subtle)] p-4 text-sm text-[var(--text-secondary)]"><strong>Versiune istorică read-only.</strong> Pentru modificări, deschide versiunea curentă și creează o versiune nouă.</div>}
    <section className="grid grid-cols-[1fr_1fr_0.8fr] gap-4"><InfoCard label="Criteriu / pattern" value={selected.criteria} /><InfoCard label="Rezultat clasificare" value={selected.result} /><div className="card p-5"><CalendarRange className="size-5 text-[var(--accent)]" /><div className="mt-4 text-xs text-[var(--text-muted)]">Perioadă efectivă</div><div className="mt-1 text-sm font-bold">{formatDate(selected.effectiveFrom)} — {selected.effectiveTo ? formatDate(selected.effectiveTo) : 'fără dată finală'}</div></div></section>
    <section className="card p-5"><div className="flex items-center gap-2 text-[var(--warning)]"><Scale className="size-5" /><h3 className="font-bold">{selected.productionEligible ? "Sursă contabilă / legală" : "Context legal demonstrativ"}</h3></div><p className="mt-3 text-sm text-[var(--text-secondary)]">{selected.legalBasis}</p>{!selected.productionEligible && <p className="mt-2 text-xs text-[var(--text-muted)]">Regulă neverificată pentru execuție automată în producție. Crearea unei versiuni sau completarea unui text nu reprezintă verificare contabilă.</p>}{selected.rulePackVersion && <p className="mt-2 text-xs">Pachet: {selected.rulePackVersion}</p>}{selected.provenance && <div className="mt-3 space-y-2 text-xs"><p>{selected.provenance.sourceTitle} · {selected.provenance.legalInstrument} · {selected.provenance.reference}</p><p>Verificat de {selected.provenance.verifiedBy} la {selected.provenance.verifiedAt}</p><p>{selected.provenance.notes}</p><LegalSourceLink url={selected.provenance.sourceURL} /></div>}</section>

    <section className="card overflow-hidden"><div className="border-b border-[var(--border)] px-5 py-4"><h3 className="font-bold">Istoric versiuni</h3><p className="mt-1 text-xs text-[var(--text-muted)]">Versiunile anterioare sunt păstrate și nu pot fi editate în loc.</p></div><div className="divide-y divide-[var(--border)]">{[...rule.versions].sort((left, right) => right.version - left.version).map((version) => <VersionHistory key={version.version} version={version} previous={rule.versions.find((candidate) => candidate.version === version.version - 1)} current={version.version === latestNumber} ruleId={rule.id} returnTo={detailReturn} scope={RULE_SCOPE_LABELS[rule.scope]} />)}</div></section>
    <NewVersionDialog rule={rule} current={rule.versions.find((version) => version.version === latestNumber)!} open={versionOpen} onOpenChange={setVersionOpen} />
    {rule.scope === 'GLOBAL' && <ClientOverrideDialog rule={rule} current={rule.versions.find((version) => version.version === latestNumber)!} clients={clients} open={overrideOpen} onOpenChange={setOverrideOpen} onCreated={() => navigate('/rules')} />}
  </div>
}

function InfoCard({ label, value }: { label: string; value: string }) { return <div className="card p-5"><ShieldCheck className="size-5 text-[var(--accent)]" /><div className="mt-4 text-xs text-[var(--text-muted)]">{label}</div><div className="mt-1 text-sm font-bold leading-6">{value}</div></div> }
function VersionHistory({ version, previous, current, ruleId, returnTo, scope }: { version: RuleVersion; previous?: RuleVersion; current: boolean; ruleId: string; returnTo: string; scope: string }) { return <article className="p-5"><div className="flex items-start justify-between gap-5"><div><div className="flex items-center gap-2"><Badge tone={current ? 'success' : 'neutral'}>Versiunea {version.version} · {current ? 'Curentă' : 'Istorică'}</Badge><Badge tone="neutral">{scope}</Badge></div><div className="mt-3 text-xs text-[var(--text-muted)]">{formatDate(version.effectiveFrom)} — {version.effectiveTo ? formatDate(version.effectiveTo) : 'fără dată finală'} · {version.actor} · {formatDateTime(version.createdAt)}</div></div>{!current && <Link to={`/rules/${ruleId}?version=${version.version}&returnTo=${encodeURIComponent(returnTo)}`} className="rounded text-xs font-semibold text-[var(--accent)] outline-none hover:underline focus-visible:ring-2 focus-visible:ring-[var(--focus)]">Inspectează versiunea</Link>}</div><div className="mt-4 grid grid-cols-2 gap-4 text-sm"><div><div className="eyebrow">Criteriu</div><p className="mt-2 text-[var(--text-secondary)]">{version.criteria}</p></div><div><div className="eyebrow">Rezultat</div><p className="mt-2 font-semibold">{version.result}</p></div></div><p className="mt-3 text-xs text-[var(--text-muted)]">{version.legalBasis}</p>{previous && <div className="mt-4 rounded-lg bg-[var(--surface-subtle)] p-3 text-xs"><strong>Schimbare față de versiunea {previous.version}:</strong><div className="mt-2 grid grid-cols-2 gap-3"><span className="text-[var(--text-secondary)]">Criteriu anterior: {previous.criteria}</span><span>Rezultat anterior: {previous.result}</span></div></div>}</article> }
function formatDate(value: string) { return new Intl.DateTimeFormat('ro-RO', { dateStyle: 'medium', timeZone: 'Europe/Bucharest' }).format(new Date(`${value}T00:00:00.000Z`)) }
function formatDateTime(value: string) { return new Intl.DateTimeFormat('ro-RO', { dateStyle: 'medium', timeStyle: 'short', timeZone: 'Europe/Bucharest' }).format(new Date(value)) }
