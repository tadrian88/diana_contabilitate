import { AlertCircle, ChevronDown, ChevronRight, Coins } from 'lucide-react'
import { Fragment, useState } from 'react'
import { Link } from 'react-router-dom'
import { Badge } from '../../components/ui/badge'
import { Button } from '../../components/ui/button'
import { AI_OPERATION_LABELS, AI_RUN_STATUS_LABELS, aiOutcomeLabel, type AIUsageRun } from '../../domain/ai-usage'
import { useAIUsageRun, useClientAIUsage, useClientAIUsageRuns } from './ai-usage-hooks'
import { AIUsageNotices, AIUsagePeriodFilter } from './AIUsageSummary'
import { formatDateTime, formatTokens, formatUsd } from './format'
import { currentMonthPeriod } from './period'

export function ClientAIUsageCard({ clientId }: { clientId: string }) {
  const [period, setPeriod] = useState(() => currentMonthPeriod())
  const usage = useClientAIUsage(clientId, period)
  const runs = useClientAIUsageRuns(clientId, period)
  const items = runs.data?.pages.flatMap((page) => page.items) ?? []
  const summary = usage.data
  const totals = summary?.totals

  return <section id="consum-ai" className="card overflow-hidden">
    <div className="flex items-start justify-between gap-5 border-b border-[var(--border)] px-5 py-4">
      <div><div className="flex items-center gap-2"><Coins className="size-5 text-[var(--accent)]" /><h3 className="font-bold">Consum AI</h3></div><p className="mt-1 text-xs text-[var(--text-muted)]">Tokeni și cost estimat pe fiecare rulare AI a clientului</p></div>
      <AIUsagePeriodFilter value={period} onChange={setPeriod} />
    </div>
    <div className="space-y-4 p-5">
      {usage.isLoading ? <div className="h-20 animate-pulse rounded-lg bg-[var(--surface-subtle)]" aria-label="Se încarcă consumul AI" />
        : usage.isError || !totals ? <p className="flex items-center gap-2 text-sm text-[var(--danger)]"><AlertCircle className="size-4" />Consumul AI nu a putut fi încărcat.</p>
        : <>
          <div className="grid grid-cols-4 gap-3 text-sm">
            <Info label="Cost estimat (USD)" value={formatUsd(totals.cost.amount)} />
            <Info label="Tokeni totali" value={formatTokens(totals.totalTokens)} />
            <Info label="Rulări" value={formatTokens(totals.runs)} />
            <Info label="Apeluri către model" value={`${formatTokens(totals.calls)}${totals.failedCalls > 0 ? ` (${formatTokens(totals.failedCalls)} eșuate)` : ''}`} />
          </div>
          {summary && summary.byOperation.length > 0 && <div className="flex flex-wrap gap-2">{summary.byOperation.map((item) => <Badge key={item.operation} tone="neutral">{AI_OPERATION_LABELS[item.operation] ?? item.operation} · {formatUsd(item.totals.cost.amount)} · {formatTokens(item.totals.totalTokens)} tokeni</Badge>)}</div>}
          <AIUsageNotices totals={totals} />
        </>}
      {runs.isError ? <p className="text-sm text-[var(--danger)]">Rulările nu au putut fi încărcate.</p>
        : !runs.isLoading && items.length === 0 ? <p className="text-sm text-[var(--text-muted)]">Nu există consum AI în perioada selectată.</p>
        : items.length > 0 && <div className="overflow-x-auto rounded-lg border border-[var(--border)]"><table className="w-full min-w-[760px] border-collapse text-left text-sm"><caption className="sr-only">Rulările AI ale clientului în perioada selectată.</caption>
          <thead className="bg-[var(--surface-subtle)] text-[11px] font-bold uppercase tracking-wider text-[var(--text-muted)]"><tr><th className="px-4 py-2.5">Rulare</th><th className="px-3 py-2.5">Stare</th><th className="px-3 py-2.5">Ultimul apel</th><th className="px-3 py-2.5">Apeluri</th><th className="px-3 py-2.5">Tokeni</th><th className="px-3 py-2.5">Cost estimat</th><th className="px-4 py-2.5 text-right"><span className="sr-only">Detalii</span></th></tr></thead>
          <tbody className="divide-y divide-[var(--border)]">{items.map((run) => <RunRow key={`${run.runKind}:${run.runId}`} clientId={clientId} run={run} />)}</tbody>
        </table></div>}
      {runs.hasNextPage && <Button variant="secondary" size="sm" disabled={runs.isFetchingNextPage} onClick={() => void runs.fetchNextPage()}>Încarcă mai multe</Button>}
    </div>
  </section>
}

function RunRow({ clientId, run }: { clientId: string; run: AIUsageRun }) {
  const [open, setOpen] = useState(false)
  const Chevron = open ? ChevronDown : ChevronRight
  return <Fragment>
    <tr className="hover:bg-[var(--surface-subtle)]">
      <td className="px-4 py-3"><RunLabel clientId={clientId} run={run} /><div className="mt-1 text-xs text-[var(--text-muted)]">{run.runKind === 'ACCOUNTING_ANALYSIS' ? 'Analiză contabilă' : 'Extragere contract'} · {run.models.join(', ')}{run.backfill && ' · istoric parțial'}</div></td>
      <td className="px-3 py-3"><Badge tone="neutral">{AI_RUN_STATUS_LABELS[run.status] ?? (run.status || '—')}</Badge></td>
      <td className="px-3 py-3 text-xs text-[var(--text-secondary)]">{formatDateTime(run.lastCallAt)}</td>
      <td className="px-3 py-3 tabular-nums">{formatTokens(run.totals.calls)}{run.totals.failedCalls > 0 && <span className="ml-1 text-xs text-[var(--warning)]">({formatTokens(run.totals.failedCalls)} eșuate)</span>}</td>
      <td className="px-3 py-3 tabular-nums">{formatTokens(run.totals.totalTokens)}</td>
      <td className="px-3 py-3 font-semibold tabular-nums">{formatUsd(run.totals.cost.amount)}{run.totals.unpricedCalls > 0 && <span className="ml-1 text-xs text-[var(--warning)]">+ fără preț</span>}</td>
      <td className="px-4 py-3 text-right"><Button variant="ghost" size="sm" aria-expanded={open} aria-label={`${open ? 'Ascunde' : 'Arată'} apelurile rulării ${run.label || run.runId}`} onClick={() => setOpen(!open)}><Chevron className="size-4" />Apeluri</Button></td>
    </tr>
    {open && <tr><td colSpan={7} className="bg-[var(--surface-subtle)] px-4 py-3"><RunCalls clientId={clientId} run={run} /></td></tr>}
  </Fragment>
}

function RunLabel({ clientId, run }: { clientId: string; run: AIUsageRun }) {
  const className = 'font-semibold text-[var(--accent)] hover:underline'
  if (run.runKind === 'ACCOUNTING_ANALYSIS' && run.invoiceId) return <Link className={className} to={`/invoices/${run.invoiceId}`}>Factura {run.label || run.invoiceId}</Link>
  if (run.runKind === 'CONTRACT_EXTRACTION' && run.documentId) return <Link className={className} to={`/contracts/documents/${clientId}/${run.documentId}`}>{run.label || run.documentId}{run.attemptNumber ? ` · încercarea ${run.attemptNumber}` : ''}</Link>
  return <span className="font-semibold">{run.label || run.runId}</span>
}

function RunCalls({ clientId, run }: { clientId: string; run: AIUsageRun }) {
  const detail = useAIUsageRun(clientId, run.runKind, run.runId, true)
  if (detail.isLoading) return <p className="text-xs text-[var(--text-muted)]">Se încarcă apelurile…</p>
  if (detail.isError || !detail.data) return <p className="text-xs text-[var(--danger)]">Apelurile nu au putut fi încărcate.</p>
  return <ol aria-label={`Apelurile rulării ${run.label || run.runId}`} className="space-y-1.5">{detail.data.calls.map((call) => <li key={call.id} className="grid grid-cols-[2rem_1fr_auto] items-baseline gap-3 text-xs">
    <span className="font-bold text-[var(--text-muted)]">#{call.ordinal}</span>
    <span><strong>{AI_OPERATION_LABELS[call.operation] ?? call.operation}</strong> · {aiOutcomeLabel(call)} · {formatDateTime(call.occurredAt)}{call.latencyMs !== null && ` · ${(call.latencyMs / 1000).toLocaleString('ro-RO', { maximumFractionDigits: 1 })} s`}
      <span className="block text-[var(--text-muted)]">{call.usageReported ? `intrare ${formatTokens(call.inputTokens)} (cache ${formatTokens(call.cachedTokens)}) · ieșire ${formatTokens(call.outputTokens)} · raționament ${formatTokens(call.thoughtTokens)} · ${call.model}` : `Fără tokeni raportați · ${call.model}`}{call.source === 'BACKFILL' && ' · import istoric'}</span></span>
    <span className="font-semibold tabular-nums">{call.cost ? formatUsd(call.cost.amount) : call.costStatus === 'NO_PRICE' ? 'Fără preț' : '—'}</span>
  </li>)}</ol>
}

function Info({ label, value }: { label: string; value: string }) { return <div className="rounded-lg bg-[var(--surface-subtle)] p-3"><div className="text-xs text-[var(--text-muted)]">{label}</div><div className="mt-1 font-semibold tabular-nums">{value}</div></div> }
