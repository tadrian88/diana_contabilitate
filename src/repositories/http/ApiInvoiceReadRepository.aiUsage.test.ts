import { describe, expect, it } from 'vitest'
import type { AxiosInstance } from 'axios'
import { ApiInvoiceReadRepository } from './ApiInvoiceReadRepository'

describe('ApiInvoiceReadRepository AI usage', () => {
  it('reads account, client, run-page and run-detail usage from the backend', async () => {
    const calls: Array<{ url: string; params?: unknown }> = []
    const repository = new ApiInvoiceReadRepository({
      get: async (url: string, config?: { params?: unknown }) => { calls.push({ url, params: config?.params }); return { status: 200, data: { url } } },
      post: async () => ({ status: 200, data: {} }),
    } as Pick<AxiosInstance, 'get' | 'post'>)
    const period = { from: '2026-09-01', to: '2026-09-30' }
    await repository.getAIUsageOverview(period)
    await repository.getClientAIUsage('client a', period)
    await repository.listClientAIUsageRuns('client a', period, { limit: 10, offset: 20 })
    await repository.getAIUsageRun('client a', 'CONTRACT_EXTRACTION', 'attempt/1')
    await repository.getAIUsageRun('client a', 'ACCOUNTING_ANALYSIS', 'analysis-1')
    expect(calls).toEqual([
      { url: '/ai-usage', params: period },
      { url: '/clients/client%20a/ai-usage', params: period },
      { url: '/clients/client%20a/ai-usage/runs', params: { ...period, limit: 10, offset: 20 } },
      { url: '/clients/client%20a/ai-usage/runs/contract-extraction/attempt%2F1', params: undefined },
      { url: '/clients/client%20a/ai-usage/runs/accounting-analysis/analysis-1', params: undefined },
    ])
  })
})
