import type { Invoice, SagaStatus, TaskStatus, TaskType } from '../../domain/invoice'

export const invoiceTabs = ['summary', 'contract', 'lines', 'classification', 'history'] as const
export type InvoiceTab = typeof invoiceTabs[number]

// The tab where a task of this type is worked on.
export function taskTab(type: TaskType): InvoiceTab {
  return type === 'CLASSIFICATION' ? 'classification' : 'contract'
}

// Where the invoice's next human action lives: an open task decides, else a
// status waiting for a person; everything else opens on the summary.
export function actionTab(invoice: Invoice): InvoiceTab {
  if (invoice.task && invoice.task.status !== 'RESOLVED') return taskTab(invoice.task.type)
  if (invoice.pipelineStatus === 'AWAITING_CONTRACT' || invoice.pipelineStatus === 'AWAITING_MATCH_CONFIRM' || invoice.pipelineStatus === 'AWAITING_COMMERCIAL_REVIEW') return 'contract'
  if (invoice.pipelineStatus === 'AWAITING_REVIEW') return 'classification'
  return 'summary'
}

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
