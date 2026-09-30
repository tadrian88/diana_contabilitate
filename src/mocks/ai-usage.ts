import type { AIUsageCall, AIUsageOverview, AIUsagePeriodInput, AIUsageRun, AIUsageRunDetail, AIUsageRunKind, AIUsageRunPage, AIUsageTotals, ClientAIUsage } from '../domain/ai-usage'

// Demo-only usage: timestamps are relative to "now" so the current month
// always has data. Prices mirror the demo model's list price per 1M tokens.
const DEMO_PRICE = { input: 0.75, cached: 0.075, output: 3.75 }
const TIME_ZONE = 'Europe/Bucharest'

interface DemoRun { clientId: string; runKind: AIUsageRunKind; runId: string; status: string; label: string; invoiceId?: string; documentId?: string; attemptNumber?: number }
export interface DemoCall extends AIUsageCall { clientId: string; runKind: AIUsageRunKind; runId: string }
export interface DemoAIUsage { runs: DemoRun[]; calls: DemoCall[] }

type CallSeed = { hoursAgo: number; operation: string; model?: string; outcome?: string; httpStatus?: number | null; input?: number; output?: number; thought?: number; cached?: number; backfill?: boolean }

export function demoAIUsage(now: Date): DemoAIUsage {
  const runs: DemoRun[] = [
    { clientId: 'client-alfa', runKind: 'ACCOUNTING_ANALYSIS', runId: 'analysis-alfa-1', status: 'PROPOSED', label: 'FA-2026-0142 · Furnizor Servicii SRL', invoiceId: 'inv-alfa-analysis' },
    { clientId: 'client-alfa', runKind: 'CONTRACT_EXTRACTION', runId: 'contractextract-alfa-1', status: 'SUCCEEDED', label: 'contract-servicii.pdf', documentId: 'doc-alfa-1', attemptNumber: 1 },
    { clientId: 'client-alfa', runKind: 'ACCOUNTING_ANALYSIS', runId: 'analysis-alfa-0', status: 'PROPOSED', label: 'FA-2026-0101 · Furnizor Utilități SA', invoiceId: 'inv-alfa-history' },
    { clientId: 'client-beta', runKind: 'ACCOUNTING_ANALYSIS', runId: 'analysis-beta-1', status: 'PARTIAL_VALIDATION', label: 'BT-7781 · Leasing Demo IFN', invoiceId: 'inv-beta-analysis' },
  ]
  const seeds: Record<string, CallSeed[]> = {
    'analysis-alfa-1': [
      { hoursAgo: 6, operation: 'ACCOUNTING_ANALYSIS', outcome: 'HTTP_ERROR', httpStatus: 429 },
      { hoursAgo: 5, operation: 'ACCOUNTING_ANALYSIS', input: 5200, output: 640, thought: 1100 },
    ],
    'contractextract-alfa-1': [
      { hoursAgo: 30, operation: 'CONTRACT_EXTRACTION', input: 18000, output: 2400, thought: 3000, cached: 2000 },
      { hoursAgo: 29.9, operation: 'CONTRACT_CLAUSE_NORMALIZATION', input: 1200, output: 300, thought: 200 },
      { hoursAgo: 29.8, operation: 'CONTRACT_CLAUSE_NORMALIZATION', model: 'gemini-experimental', input: 900, output: 150 },
    ],
    'analysis-alfa-0': [{ hoursAgo: 40, operation: 'ACCOUNTING_ANALYSIS', input: 4000, output: 500, backfill: true }],
    'analysis-beta-1': [{ hoursAgo: 12, operation: 'ACCOUNTING_ANALYSIS', input: 3000, output: 400, thought: 600 }],
  }
  const calls: DemoCall[] = []
  for (const run of runs) {
    seeds[run.runId].forEach((seed, index) => {
      const model = seed.model ?? 'gemini-3.8-flash'
      const reported = seed.input !== undefined
      const input = seed.input ?? 0, output = seed.output ?? 0, thought = seed.thought ?? 0, cached = seed.cached ?? 0
      const priced = reported && model === 'gemini-3.8-flash'
      const amount = priced ? round(((input - cached) * DEMO_PRICE.input + cached * DEMO_PRICE.cached + (output + thought) * DEMO_PRICE.output) / 1_000_000) : null
      calls.push({
        clientId: run.clientId, runKind: run.runKind, runId: run.runId, ordinal: index + 1, id: `llmcall-${run.runId}-${index + 1}`,
        occurredAt: new Date(now.getTime() - seed.hoursAgo * 3_600_000).toISOString(), operation: seed.operation, provider: 'GEMINI', model,
        outcome: seed.outcome ?? 'COMPLETED', httpStatus: seed.httpStatus === undefined ? 200 : seed.httpStatus, latencyMs: seed.backfill ? null : 4200 + index * 300,
        usageReported: reported, inputTokens: input, outputTokens: output, thoughtTokens: thought, cachedTokens: cached, toolUseTokens: 0, totalTokens: input + output + thought,
        cost: amount === null ? null : { amount, currency: 'USD' }, costStatus: !reported ? 'NO_USAGE' : priced ? 'PRICED' : 'NO_PRICE', source: seed.backfill ? 'BACKFILL' : 'LIVE',
      })
    })
  }
  return { runs, calls }
}

const round = (value: number) => Math.round(value * 1e10) / 1e10

export function bucharestDay(iso: string): string {
  return new Intl.DateTimeFormat('en-CA', { timeZone: TIME_ZONE, year: 'numeric', month: '2-digit', day: '2-digit' }).format(new Date(iso))
}

const inPeriod = (call: DemoCall, period: AIUsagePeriodInput) => { const day = bucharestDay(call.occurredAt); return day >= period.from && day <= period.to }

export function totalsOf(calls: DemoCall[]): AIUsageTotals {
  const sum = (pick: (call: DemoCall) => number) => calls.reduce((total, call) => total + pick(call), 0)
  return {
    runs: new Set(calls.map((call) => `${call.runKind}:${call.runId}`)).size, calls: calls.length,
    failedCalls: calls.filter((call) => call.outcome !== 'COMPLETED').length, unpricedCalls: calls.filter((call) => call.costStatus === 'NO_PRICE').length,
    unreportedCalls: calls.filter((call) => call.costStatus === 'NO_USAGE').length,
    inputTokens: sum((call) => call.inputTokens), outputTokens: sum((call) => call.outputTokens), thoughtTokens: sum((call) => call.thoughtTokens),
    cachedTokens: sum((call) => call.cachedTokens), totalTokens: sum((call) => call.totalTokens),
    cost: { amount: round(sum((call) => call.cost?.amount ?? 0)), currency: 'USD' }, includesBackfill: calls.some((call) => call.source === 'BACKFILL'),
  }
}

const period = (value: AIUsagePeriodInput) => ({ from: value.from, to: value.to, timeZone: TIME_ZONE })

export function demoOverview(data: DemoAIUsage, clients: Array<{ id: string; name: string }>, value: AIUsagePeriodInput): AIUsageOverview {
  const calls = data.calls.filter((call) => clients.some((client) => client.id === call.clientId) && inPeriod(call, value))
  const rows = clients.map((client) => ({ clientId: client.id, clientName: client.name, totals: totalsOf(calls.filter((call) => call.clientId === client.id)) }))
  rows.sort((left, right) => right.totals.cost.amount - left.totals.cost.amount || left.clientName.localeCompare(right.clientName, 'ro'))
  return { period: period(value), totals: totalsOf(calls), clients: rows }
}

export function demoClientUsage(data: DemoAIUsage, clientId: string, value: AIUsagePeriodInput): ClientAIUsage {
  const calls = data.calls.filter((call) => call.clientId === clientId && inPeriod(call, value))
  const operations = [...new Set(calls.map((call) => call.operation))]
  const models = [...new Set(calls.map((call) => call.model))]
  return {
    clientId, period: period(value), totals: totalsOf(calls),
    byOperation: operations.map((operation) => ({ operation, totals: totalsOf(calls.filter((call) => call.operation === operation)) })),
    byModel: models.map((model) => ({ provider: 'GEMINI', model, totals: totalsOf(calls.filter((call) => call.model === model)) })),
  }
}

function runSummary(run: DemoRun, calls: DemoCall[]): AIUsageRun {
  const times = calls.map((call) => call.occurredAt).sort()
  const totals = totalsOf(calls)
  return { runKind: run.runKind, runId: run.runId, status: run.status, label: run.label, invoiceId: run.invoiceId, documentId: run.documentId, attemptNumber: run.attemptNumber,
    firstCallAt: times[0], lastCallAt: times[times.length - 1], models: [...new Set(calls.map((call) => call.model))].sort(), backfill: totals.includesBackfill, totals }
}

export function demoRunPage(data: DemoAIUsage, clientId: string, value: AIUsagePeriodInput, page: { limit: number; offset: number }): AIUsageRunPage {
  const summaries = data.runs.filter((run) => run.clientId === clientId)
    .map((run) => ({ run, calls: data.calls.filter((call) => call.runId === run.runId && inPeriod(call, value)) }))
    .filter((item) => item.calls.length > 0)
    .map((item) => runSummary(item.run, item.calls))
    .sort((left, right) => right.lastCallAt.localeCompare(left.lastCallAt))
  return { items: summaries.slice(page.offset, page.offset + page.limit), total: summaries.length, limit: page.limit, offset: page.offset }
}

export function demoRunDetail(data: DemoAIUsage, clientId: string, runKind: AIUsageRunKind, runId: string): AIUsageRunDetail {
  const run = data.runs.find((candidate) => candidate.clientId === clientId && candidate.runKind === runKind && candidate.runId === runId)
  if (!run) throw new Error('Rularea nu a fost găsită.')
  const calls = data.calls.filter((call) => call.runId === runId)
  return { clientId, run: runSummary(run, calls), calls: calls.map(publicCall) }
}

function publicCall(call: DemoCall): AIUsageCall {
  const result: Partial<DemoCall> = { ...call }
  delete result.clientId; delete result.runKind; delete result.runId
  return result as AIUsageCall
}
