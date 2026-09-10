import { useQueryClient } from '@tanstack/react-query'
import { useEffect, useRef } from 'react'
import type { PipelineStatus } from '../domain/invoice'
import { useInvoiceRepository } from './repository-context'
import { invoiceQueryKeys, useInvoices } from '../features/invoices/invoice-hooks'

const automaticNext: Partial<Record<PipelineStatus, PipelineStatus>> = {
  DOWNLOADED: 'ARCHIVED',
  ARCHIVED: 'MATCHING',
  MATCHING: 'DEDUPE_CHECKED',
  DEDUPE_CHECKED: 'HEADER_READ',
  HEADER_READ: 'LINES_READ',
  LINES_READ: 'CLASSIFIED',
  CLASSIFIED: 'READY_FOR_SAGA',
  READY_FOR_SAGA: 'EXPORTING',
  EXPORTING: 'EXPORTED',
}

export function AutomaticWorkflowRunner() {
  const repository = useInvoiceRepository()
  const queryClient = useQueryClient()
  const { data: invoices = [] } = useInvoices('all')
  const scheduled = useRef<string | null>(null)
  const candidate = invoices.find((invoice) => invoice.autoRun && automaticNext[invoice.pipelineStatus])
  const candidateKey = candidate ? `${candidate.id}:${candidate.pipelineStatus}` : null

  useEffect(() => {
    if (!candidate || !candidateKey || scheduled.current === candidateKey) return
    const next = automaticNext[candidate.pipelineStatus]
    if (!next) return
    scheduled.current = candidateKey
    const timer = window.setTimeout(async () => {
      const updated = await repository.transitionInvoice(candidate.id, next)
      queryClient.setQueryData(invoiceQueryKeys.invoice(candidate.id), updated)
      scheduled.current = null
      await queryClient.invalidateQueries({ queryKey: ['invoices'] })
    }, 220)
    return () => {
      window.clearTimeout(timer)
      if (scheduled.current === candidateKey) scheduled.current = null
    }
  }, [candidate, candidateKey, queryClient, repository])

  return null
}
