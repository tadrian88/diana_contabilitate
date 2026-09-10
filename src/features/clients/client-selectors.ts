import type { ClassificationRule, Client, Contract, Invoice } from '../../domain/invoice'

export function selectClientOperations(client: Client, invoices: Invoice[], contracts: Contract[], rules: ClassificationRule[]) {
  const clientInvoices = invoices.filter((invoice) => invoice.clientId === client.id)
  const openInvoices = clientInvoices.filter((invoice) => invoice.task?.status === 'OPEN')
  const waitingInvoices = clientInvoices.filter((invoice) => invoice.task?.status === 'WAITING')

  return {
    client,
    invoices: clientInvoices,
    openInvoices,
    waitingInvoices,
    contracts: contracts.filter((contract) => contract.clientId === client.id),
    ruleOverrides: rules.filter((rule) => rule.scope === 'CLIENT_OVERRIDE' && rule.clientId === client.id),
    counts: {
      processing: clientInvoices.filter((invoice) => !['EXPORTED', 'DUPLICATE'].includes(invoice.pipelineStatus)).length,
      open: openInvoices.length,
      waiting: waitingInvoices.length,
      ready: clientInvoices.filter((invoice) => invoice.sagaStatus === 'READY').length,
      exporting: clientInvoices.filter((invoice) => invoice.sagaStatus === 'EXPORTING').length,
      exported: clientInvoices.filter((invoice) => invoice.sagaStatus === 'EXPORTED').length,
      failed: clientInvoices.filter((invoice) => invoice.sagaStatus === 'FAILED').length,
    },
  }
}
