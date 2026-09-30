import { AlertCircle, ArrowRight, Coins } from 'lucide-react'
import { useState } from 'react'
import { Link } from 'react-router-dom'
import { useClientScope } from '../../app/scope-context'
import { Badge } from '../../components/ui/badge'
import { useAIUsageOverview } from './ai-usage-hooks'
import { AIUsagePeriodFilter, AIUsageSummary } from './AIUsageSummary'
import { formatTokens, formatUsd } from './format'
import { currentMonthPeriod, formatDay } from './period'

// The account view: every client the signed-in accountant can access.
export function AIUsagePage() {
  const [period, setPeriod] = useState(() => currentMonthPeriod())
  const { scope, setScope } = useClientScope()
  const overview = useAIUsageOverview(period)
  const rows = (overview.data?.clients ?? []).filter((client) => scope === 'all' || client.clientId === scope)

  return <div className="space-y-6">
    <section className="flex items-end justify-between gap-6">
      <div><p className="eyebrow">Contul tău</p><h2 className="mt-2 text-2xl font-bold tracking-tight">Consum AI</h2><p className="mt-2 text-sm text-[var(--text-secondary)]">Tokeni și cost estimat al rulărilor AI pentru toți clienții la care ai acces.</p></div>
      <AIUsagePeriodFilter value={period} onChange={setPeriod} />
    </section>
    {overview.isLoading ? <div className="card h-40 animate-pulse bg-[var(--surface-subtle)]" aria-label="Se încarcă consumul AI" />
      : overview.isError || !overview.data ? <div className="card p-10 text-center"><AlertCircle className="mx-auto size-8 text-[var(--danger)]" /><h3 className="mt-3 font-bold">Consumul AI nu a putut fi încărcat</h3></div>
      : <>
        <AIUsageSummary totals={overview.data.totals} scopeLabel={`Total cont · ${formatDay(overview.data.period.from)} – ${formatDay(overview.data.period.to)}`} />
        <section className="card overflow-hidden" aria-labelledby="ai-usage-clients-heading">
          <div className="flex items-center justify-between border-b border-[var(--border)] px-5 py-4"><h3 id="ai-usage-clients-heading" className="font-bold">Consum pe client</h3>{scope !== 'all' && <Badge tone="info">Filtrat pe clientul selectat</Badge>}</div>
          {rows.length === 0 ? <div className="p-10 text-center"><Coins className="mx-auto size-8 text-[var(--text-muted)]" /><p className="mt-3 text-sm text-[var(--text-muted)]">Nu există clienți în contextul activ.</p></div>
            : <div className="overflow-x-auto"><table className="w-full min-w-[860px] border-collapse text-left"><caption className="sr-only">Consum AI pe client în perioada selectată.</caption>
              <thead className="bg-[var(--surface-subtle)] text-[11px] font-bold uppercase tracking-wider text-[var(--text-muted)]"><tr><th className="px-5 py-3">Client</th><th className="px-4 py-3">Rulări</th><th className="px-4 py-3">Apeluri</th><th className="px-4 py-3">Tokeni intrare</th><th className="px-4 py-3">Tokeni ieșire + raționament</th><th className="px-4 py-3">Cost estimat</th><th className="px-5 py-3 text-right">Detalii</th></tr></thead>
              <tbody className="divide-y divide-[var(--border)]">{rows.map(({ clientId, clientName, totals }) => <tr key={clientId} data-client={clientId} className="hover:bg-[var(--surface-subtle)]">
                <td className="px-5 py-4 font-bold">{clientName}{totals.includesBackfill && <span className="ml-2 text-[10px] font-semibold uppercase text-[var(--warning)]">istoric parțial</span>}</td>
                <td className="px-4 py-4 text-sm tabular-nums">{formatTokens(totals.runs)}</td>
                <td className="px-4 py-4 text-sm tabular-nums">{formatTokens(totals.calls)}{totals.failedCalls > 0 && <span className="ml-1 text-xs text-[var(--warning)]">({formatTokens(totals.failedCalls)} eșuate)</span>}</td>
                <td className="px-4 py-4 text-sm tabular-nums">{formatTokens(totals.inputTokens)}</td>
                <td className="px-4 py-4 text-sm tabular-nums">{formatTokens(totals.outputTokens + totals.thoughtTokens)}</td>
                <td className="px-4 py-4 text-sm font-bold tabular-nums">{formatUsd(totals.cost.amount)}</td>
                <td className="px-5 py-4 text-right"><Link to={`/clients/${clientId}#consum-ai`} onClick={() => setScope(clientId)} className="inline-flex items-center gap-2 rounded-lg px-3 py-2 text-xs font-semibold text-[var(--accent)] outline-none hover:bg-[var(--info-soft)] focus-visible:ring-2 focus-visible:ring-[var(--focus)]">Rulări client<ArrowRight className="size-3.5" /></Link></td>
              </tr>)}</tbody>
            </table></div>}
        </section>
      </>}
  </div>
}
