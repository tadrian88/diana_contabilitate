import { keepPreviousData, useInfiniteQuery, useQuery } from '@tanstack/react-query'
import { queryKeys } from '../../app/queryKeys'
import { useInvoiceRepository } from '../../app/repository-context'
import type { AIUsagePeriodInput, AIUsageRunKind } from '../../domain/ai-usage'

export const AI_USAGE_RUN_PAGE_SIZE = 10

export function useAIUsageOverview(period: AIUsagePeriodInput) {
  const repository = useInvoiceRepository()
  return useQuery({ queryKey: queryKeys.aiUsage.overview(period.from, period.to), queryFn: () => repository.getAIUsageOverview(period), staleTime: 60_000, placeholderData: keepPreviousData })
}

export function useClientAIUsage(clientId: string, period: AIUsagePeriodInput) {
  const repository = useInvoiceRepository()
  return useQuery({ queryKey: queryKeys.aiUsage.client(clientId, period.from, period.to), queryFn: () => repository.getClientAIUsage(clientId, period), enabled: Boolean(clientId), staleTime: 60_000, placeholderData: keepPreviousData })
}

export function useClientAIUsageRuns(clientId: string, period: AIUsagePeriodInput) {
  const repository = useInvoiceRepository()
  return useInfiniteQuery({
    queryKey: queryKeys.aiUsage.runs(clientId, period.from, period.to),
    queryFn: ({ pageParam }) => repository.listClientAIUsageRuns(clientId, period, { limit: AI_USAGE_RUN_PAGE_SIZE, offset: pageParam }),
    initialPageParam: 0,
    getNextPageParam: (last) => last.offset + last.items.length < last.total ? last.offset + last.items.length : undefined,
    enabled: Boolean(clientId), staleTime: 60_000,
  })
}

export function useAIUsageRun(clientId: string, runKind: AIUsageRunKind, runId: string, enabled: boolean) {
  const repository = useInvoiceRepository()
  return useQuery({ queryKey: queryKeys.aiUsage.run(clientId, runKind, runId), queryFn: () => repository.getAIUsageRun(clientId, runKind, runId), enabled, staleTime: 60_000 })
}
