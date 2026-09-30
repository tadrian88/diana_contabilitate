// AI usage and cost: tokens and estimated USD cost of every model call,
// aggregated per run, per client and per account (all clients the
// accountant can access). Costs are estimates from public list prices.
export type AIUsageRunKind = 'ACCOUNTING_ANALYSIS' | 'CONTRACT_EXTRACTION'
export type AIUsageCostStatus = 'PRICED' | 'NO_PRICE' | 'NO_USAGE'

export interface AIUsagePeriodInput { from: string; to: string }
export interface AIUsagePeriod extends AIUsagePeriodInput { timeZone: string }
export interface AIUsageCost { amount: number; currency: 'USD' }

export interface AIUsageTotals {
  runs: number; calls: number; failedCalls: number; unpricedCalls: number; unreportedCalls: number
  inputTokens: number; outputTokens: number; thoughtTokens: number; cachedTokens: number; totalTokens: number
  cost: AIUsageCost; includesBackfill: boolean
}

export interface AIUsageOverview { period: AIUsagePeriod; totals: AIUsageTotals; clients: Array<{ clientId: string; clientName: string; totals: AIUsageTotals }> }

export interface ClientAIUsage {
  clientId: string; period: AIUsagePeriod; totals: AIUsageTotals
  byOperation: Array<{ operation: string; totals: AIUsageTotals }>
  byModel: Array<{ provider: string; model: string; totals: AIUsageTotals }>
}

export interface AIUsageRun {
  runKind: AIUsageRunKind; runId: string; status: string; label: string
  invoiceId?: string; documentId?: string; attemptNumber?: number
  firstCallAt: string; lastCallAt: string; models: string[]; backfill: boolean; totals: AIUsageTotals
}

export interface AIUsageRunPage { items: AIUsageRun[]; total: number; limit: number; offset: number }

export interface AIUsageCall {
  ordinal: number; id: string; occurredAt: string; operation: string; provider: string; model: string
  outcome: string; providerStatus?: string; httpStatus: number | null; latencyMs: number | null; usageReported: boolean
  inputTokens: number; outputTokens: number; thoughtTokens: number; cachedTokens: number; toolUseTokens: number; totalTokens: number
  cost: AIUsageCost | null; costStatus: AIUsageCostStatus; source: 'LIVE' | 'BACKFILL'
}

export interface AIUsageRunDetail { clientId: string; run: AIUsageRun; calls: AIUsageCall[] }

export const AI_RUN_KIND_PATH: Record<AIUsageRunKind, string> = { ACCOUNTING_ANALYSIS: 'accounting-analysis', CONTRACT_EXTRACTION: 'contract-extraction' }

export const AI_OPERATION_LABELS: Record<string, string> = {
  ACCOUNTING_ANALYSIS: 'Analiză contabilă', CONTRACT_EXTRACTION: 'Extragere contract', CONTRACT_CLAUSE_NORMALIZATION: 'Normalizare clauze',
}

export const AI_RUN_STATUS_LABELS: Record<string, string> = {
  RUNNING: 'În curs', PROPOSED: 'Propunere', PARTIAL_VALIDATION: 'Validare parțială', VALIDATION_FAILED: 'Validare eșuată',
  PROVIDER_FAILED: 'Eroare model', SUPERSEDED: 'Înlocuită', NOT_NEEDED: 'Nu a fost necesară',
  STARTED: 'În curs', SUCCEEDED: 'Reușită', FAILED: 'Eșuată',
}

export function aiOutcomeLabel(call: Pick<AIUsageCall, 'outcome' | 'httpStatus'>): string {
  switch (call.outcome) {
    case 'COMPLETED': return 'Finalizat'
    case 'PROVIDER_INCOMPLETE': return 'Răspuns incomplet'
    case 'PROVIDER_FAILED': return 'Eșuat la model'
    case 'HTTP_ERROR': return `Eroare HTTP ${call.httpStatus ?? ''}`.trim()
    case 'INVALID_ENVELOPE': return 'Răspuns invalid'
    case 'TRANSPORT_ERROR': return 'Fără răspuns'
    default: return call.outcome
  }
}
