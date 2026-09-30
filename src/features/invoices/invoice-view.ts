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

/** An invoice issued by the accounting client (Vânzări V1, D-124). */
export function isIssued(invoice: Pick<Invoice, 'direction'>) {
  return invoice.direction === 'OUTGOING'
}

export interface Counterparty { role: 'Furnizor' | 'Client'; name: string; identifier?: string }

/** The other party: the supplier of a received invoice, the customer of an issued one. */
export function counterparty(invoice: Pick<Invoice, 'direction' | 'supplierName' | 'supplierCui' | 'customerName' | 'customerIdentifier'>): Counterparty {
  if (isIssued(invoice)) return { role: 'Client', name: invoice.customerName ?? 'Client indisponibil', identifier: invoice.customerIdentifier }
  return { role: 'Furnizor', name: invoice.supplierName, identifier: invoice.supplierCui }
}

/**
 * D-128: an issued invoice follows the client profile for VAT chargeability.
 * A „TVA la încasare” note printed on it that contradicts the profile is only
 * a warning to check with the accountant.
 */
export function vatCashNoteWarning(invoice: Pick<Invoice, 'direction' | 'sourceFacts' | 'accountingSnapshot'>): string | undefined {
  if (!isIssued(invoice) || invoice.sourceFacts?.cashAccounting !== 'YES') return undefined
  if (invoice.accountingSnapshot?.profile?.cashAccounting !== 'NO') return undefined
  return 'Factura emisă poartă mențiunea „TVA la încasare”, dar profilul clientului nu aplică TVA la încasare. Diana urmează profilul (TVA exigibil la emitere, 4427); verifică mențiunea cu contabilul.'
}
