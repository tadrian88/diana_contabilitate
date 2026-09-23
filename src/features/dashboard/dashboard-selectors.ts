import type { Client, Invoice, PipelineStatus } from '../../domain/invoice'

export const DEMO_CLOCK = '2026-09-09T15:00:00.000Z'

export const pipelineGroups: { label: string; states: PipelineStatus[] }[] = [
  { label: 'Intrare și asociere', states: ['DOWNLOADED', 'ARCHIVED', 'MATCHING'] },
  { label: 'Excepții', states: ['AWAITING_CONTRACT', 'AWAITING_MATCH_CONFIRM', 'AWAITING_COMMERCIAL_REVIEW', 'AWAITING_REVIEW'] },
  { label: 'Procesare', states: ['DEDUPE_CHECKED', 'HEADER_READ', 'LINES_READ', 'COMMERCIAL_VALIDATING', 'COMMERCIALLY_VALIDATED', 'CLASSIFIED'] },
  { label: 'SAGA', states: ['READY_FOR_SAGA', 'EXPORTING', 'EXPORTED'] },
  { label: 'Terminal', states: ['DUPLICATE'] },
]

export function selectDashboard(invoices: Invoice[], clients: Client[], now = DEMO_CLOCK) {
  const currentMonthInvoices = invoices.filter((invoice) => isSameUtcMonth(invoice.issueDate, now))
  const openInvoices = currentMonthInvoices.filter((invoice) => invoice.task?.status === 'OPEN')
  const waitingInvoices = currentMonthInvoices.filter((invoice) => invoice.task?.status === 'WAITING')
  const failedInvoices = currentMonthInvoices.filter((invoice) => invoice.sagaStatus === 'FAILED')

  return {
    invoices: currentMonthInvoices,
    openInvoices,
    waitingInvoices,
    failedInvoices,
    kpis: {
      processing: currentMonthInvoices.filter((invoice) => !['EXPORTED', 'DUPLICATE'].includes(invoice.pipelineStatus)).length,
      attention: openInvoices.length,
      ready: currentMonthInvoices.filter((invoice) => invoice.pipelineStatus === 'READY_FOR_SAGA').length,
      exported: currentMonthInvoices.filter((invoice) => invoice.pipelineStatus === 'EXPORTED').length,
    },
    pipelineCounts: Object.fromEntries(pipelineGroups.flatMap((group) => group.states).map((status) => [status, currentMonthInvoices.filter((invoice) => invoice.pipelineStatus === status).length])) as Record<PipelineStatus, number>,
    sagaCounts: {
      ready: currentMonthInvoices.filter((invoice) => invoice.sagaStatus === 'READY').length,
      exporting: currentMonthInvoices.filter((invoice) => invoice.sagaStatus === 'EXPORTING').length,
      exported: currentMonthInvoices.filter((invoice) => invoice.sagaStatus === 'EXPORTED').length,
      failed: failedInvoices.length,
    },
    clients: clients.map((client) => {
      const clientInvoices = currentMonthInvoices.filter((invoice) => invoice.clientId === client.id)
      return {
        client,
        invoiceCount: clientInvoices.length,
        openCount: clientInvoices.filter((invoice) => invoice.task?.status === 'OPEN').length,
        waitingCount: clientInvoices.filter((invoice) => invoice.task?.status === 'WAITING').length,
        processingCount: clientInvoices.filter((invoice) => !['EXPORTED', 'DUPLICATE'].includes(invoice.pipelineStatus)).length,
        readyCount: clientInvoices.filter((invoice) => invoice.pipelineStatus === 'READY_FOR_SAGA').length,
        exportedCount: clientInvoices.filter((invoice) => invoice.pipelineStatus === 'EXPORTED').length,
      }
    }),
  }
}

function isSameUtcMonth(value: string, reference: string) {
  const date = new Date(value)
  const clock = new Date(reference)
  return date.getUTCFullYear() === clock.getUTCFullYear() && date.getUTCMonth() === clock.getUTCMonth()
}
