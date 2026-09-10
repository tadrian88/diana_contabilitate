import type { Invoice, SagaStatus, TaskStatus } from '../../domain/invoice'

export function getUnresolvedIssueCount(invoice: Invoice) {
  const task = invoice.task
  if (!task || task.status === 'RESOLVED') return 0
  if (task.type === 'CLASSIFICATION') return task.classificationItems?.filter((item) => item.status === 'PENDING').length ?? 0
  return 1
}

export function getAttentionStatus(invoice: Invoice): TaskStatus | 'CLEAR' {
  return invoice.task?.status === 'OPEN' || invoice.task?.status === 'WAITING' ? invoice.task.status : 'CLEAR'
}

export function getInvoiceConfidence(invoice: Invoice) {
  if (invoice.confidence) return invoice.confidence
  if (invoice.task?.type === 'CONTRACT_MATCH') return invoice.task.contractCandidates?.find((candidate) => candidate.recommended)?.confidence
  return undefined
}

export const SAGA_LABELS: Record<SagaStatus, string> = {
  NOT_READY: 'Nu a ajuns la SAGA',
  READY: 'Pregătită pentru SAGA',
  EXPORTING: 'Export în curs',
  EXPORTED: 'Exportată în SAGA',
  FAILED: 'Export eșuat',
}
