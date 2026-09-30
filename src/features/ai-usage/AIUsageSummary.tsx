import { AlertCircle, Info } from 'lucide-react'
import { useState } from 'react'
import type { AIUsagePeriodInput, AIUsageTotals } from '../../domain/ai-usage'
import { formatTokens, formatUsd } from './format'
import { currentMonthPeriod, previousMonthPeriod, validPeriod, type AIUsagePreset } from './period'

function presetOf(value: AIUsagePeriodInput): AIUsagePreset {
  const current = currentMonthPeriod(); const previous = previousMonthPeriod()
  if (value.from === current.from && value.to === current.to) return 'CURRENT_MONTH'
  if (value.from === previous.from && value.to === previous.to) return 'PREVIOUS_MONTH'
  return 'CUSTOM'
}

export function AIUsagePeriodFilter({ value, onChange }: { value: AIUsagePeriodInput; onChange: (period: AIUsagePeriodInput) => void }) {
  const [preset, setPreset] = useState<AIUsagePreset>(() => presetOf(value))
  const [draft, setDraft] = useState(value)
  const choose = (next: AIUsagePreset) => {
    setPreset(next)
    if (next === 'CURRENT_MONTH') onChange(currentMonthPeriod())
    if (next === 'PREVIOUS_MONTH') onChange(previousMonthPeriod())
    if (next === 'CUSTOM') setDraft(value)
  }
  const edit = (next: AIUsagePeriodInput) => { setDraft(next); if (validPeriod(next)) onChange(next) }
  const field = 'rounded-lg border border-[var(--border)] bg-[var(--surface)] px-2 py-1.5 text-sm'
  return <div className="flex flex-wrap items-center gap-2">
    <select aria-label="Perioada consumului AI" className={field} value={preset} onChange={(event) => choose(event.target.value as AIUsagePreset)}>
      <option value="CURRENT_MONTH">Luna curentă</option><option value="PREVIOUS_MONTH">Luna trecută</option><option value="CUSTOM">Interval personalizat</option>
    </select>
    {preset === 'CUSTOM' && <>
      <input type="date" aria-label="De la" className={field} value={draft.from} max={draft.to} onChange={(event) => edit({ ...draft, from: event.target.value })} />
      <input type="date" aria-label="Până la" className={field} value={draft.to} min={draft.from} onChange={(event) => edit({ ...draft, to: event.target.value })} />
      {!validPeriod(draft) && <span role="alert" className="text-xs text-[var(--danger)]">Interval invalid</span>}
    </>}
  </div>
}

export function AIUsageSummary({ totals, scopeLabel }: { totals: AIUsageTotals; scopeLabel: string }) {
  return <div className="space-y-3">
    <section aria-label={`Consum AI ${scopeLabel}`} className="grid grid-cols-4 gap-3">
      <Metric label="Cost estimat (USD)" value={formatUsd(totals.cost.amount)} detail={scopeLabel} />
      <Metric label="Tokeni totali" value={formatTokens(totals.totalTokens)} detail={`intrare ${formatTokens(totals.inputTokens)} · ieșire ${formatTokens(totals.outputTokens)} · raționament ${formatTokens(totals.thoughtTokens)}`} />
      <Metric label="Rulări" value={formatTokens(totals.runs)} detail={`din cache ${formatTokens(totals.cachedTokens)} tokeni`} />
      <Metric label="Apeluri către model" value={formatTokens(totals.calls)} detail={`${formatTokens(totals.failedCalls)} eșuate sau reîncercate`} />
    </section>
    <AIUsageNotices totals={totals} />
  </div>
}

export function AIUsageNotices({ totals }: { totals: AIUsageTotals }) {
  return <div className="space-y-2 text-xs">
    {totals.includesBackfill && <p className="flex items-start gap-2 rounded-lg bg-[var(--warning-soft)] p-2.5 text-[var(--warning)]"><AlertCircle className="mt-0.5 size-3.5 shrink-0" />Istoric parțial: rulările dinainte de activarea evidenței nu includ reîncercările, apelurile eșuate și tokenii de raționament.</p>}
    {totals.unpricedCalls > 0 && <p className="flex items-start gap-2 rounded-lg bg-[var(--warning-soft)] p-2.5 text-[var(--warning)]"><AlertCircle className="mt-0.5 size-3.5 shrink-0" />{totals.unpricedCalls === 1 ? 'Un apel nu are' : `${totals.unpricedCalls} apeluri nu au`} preț configurat pentru model; costul lor nu este inclus.</p>}
    <p className="flex items-start gap-2 text-[var(--text-muted)]"><Info className="mt-0.5 size-3.5 shrink-0" />Costul este estimat din prețurile publice Google valabile la momentul fiecărui apel.</p>
  </div>
}

function Metric({ label, value, detail }: { label: string; value: string; detail: string }) {
  return <div className="card p-4"><div className="text-xs font-semibold text-[var(--text-secondary)]">{label}</div><div className="mt-2 text-2xl font-bold tabular-nums">{value}</div><div className="mt-1 text-[11px] text-[var(--text-muted)]">{detail}</div></div>
}
