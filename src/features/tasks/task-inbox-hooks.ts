import { useMemo } from 'react'
import { useClients, useInvoices } from '../invoices/invoice-hooks'
import { useClientScope } from '../../app/scope-context'
import type { Client, Invoice, ValidationTask } from '../../domain/invoice'

export interface TaskInboxItem {
  task: ValidationTask
  invoice: Invoice
  client?: Client
}

export function useTaskInbox() {
  const { scope } = useClientScope()
  const invoiceQuery = useInvoices(scope)
  const clientQuery = useClients()
  const items = useMemo(() => {
    const clients = new Map((clientQuery.data ?? []).map((client) => [client.id, client]))
    return (invoiceQuery.data ?? [])
      .filter((invoice): invoice is Invoice & { task: ValidationTask } => Boolean(invoice.task))
      .map((invoice) => ({ invoice, task: invoice.task, client: clients.get(invoice.clientId) }))
      .sort((left, right) => right.task.createdAt.localeCompare(left.task.createdAt))
  }, [clientQuery.data, invoiceQuery.data])

  return {
    items,
    isLoading: invoiceQuery.isLoading || clientQuery.isLoading,
    isError: invoiceQuery.isError || clientQuery.isError,
  }
}

