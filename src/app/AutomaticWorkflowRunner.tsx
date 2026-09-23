import { useQueryClient } from '@tanstack/react-query'
import { useEffect, useRef } from 'react'
import type { PipelineStatus } from '../domain/invoice'
import { useInvoiceRepository } from './repository-context'
import { useInvoices } from '../features/invoices/invoice-hooks'
import { queryKeys } from './queryKeys'
import { isMockWorkflowRepository } from '../repositories/invoiceRepository'

const automaticNext: Partial<Record<PipelineStatus, PipelineStatus>> = {
  DOWNLOADED: 'ARCHIVED',
  ARCHIVED: 'MATCHING',
  MATCHING: 'DEDUPE_CHECKED',
  DEDUPE_CHECKED: 'HEADER_READ',
  HEADER_READ: 'LINES_READ',
  LINES_READ: 'COMMERCIAL_VALIDATING',
  COMMERCIAL_VALIDATING: 'COMMERCIALLY_VALIDATED',
  COMMERCIALLY_VALIDATED: 'CLASSIFIED',
  CLASSIFIED: 'READY_FOR_SAGA',
  READY_FOR_SAGA: 'EXPORTING',
  EXPORTING: 'EXPORTED',
}

export function AutomaticWorkflowRunner() {
  const repository = useInvoiceRepository()
  const queryClient = useQueryClient()
  const { data: invoices = [] } = useInvoices('all')
  const scheduled = useRef<string | null>(null)
  const candidate = repository.runtimeAuthority === 'MOCK' ? invoices.find((invoice) => invoice.autoRun && automaticNext[invoice.pipelineStatus]) : undefined
  const candidateKey = candidate ? `${candidate.id}:${candidate.pipelineStatus}` : null

  useEffect(() => {
    if (!isMockWorkflowRepository(repository) || !candidate || !candidateKey || scheduled.current === candidateKey) return
    const next = automaticNext[candidate.pipelineStatus]
    if (!next) return
    scheduled.current = candidateKey
    const timer = window.setTimeout(async () => {
      const updated = await repository.transitionInvoice(candidate.id, next)
      queryClient.setQueryData(queryKeys.invoices.detail(candidate.id), updated)
      scheduled.current = null
      await queryClient.invalidateQueries({ queryKey: queryKeys.invoices.root })
    }, 220)
    return () => {
      window.clearTimeout(timer)
      if (scheduled.current === candidateKey) scheduled.current = null
    }
  }, [candidate, candidateKey, queryClient, repository])

  return null
}
