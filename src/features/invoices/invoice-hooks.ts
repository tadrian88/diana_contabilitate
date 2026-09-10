import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useRef } from 'react'
import type { ClientScope, Invoice } from '../../domain/invoice'
import { useInvoiceRepository } from '../../app/repository-context'

export const invoiceQueryKeys = {
  invoices: (scope: ClientScope) => ['invoices', scope] as const,
  invoice: (id: string) => ['invoice', id] as const,
  clients: ['clients'] as const,
}

export function useClients() {
  const repository = useInvoiceRepository()
  return useQuery({ queryKey: invoiceQueryKeys.clients, queryFn: () => repository.listClients() })
}

export function useInvoices(scope: ClientScope) {
  const repository = useInvoiceRepository()
  return useQuery({ queryKey: invoiceQueryKeys.invoices(scope), queryFn: () => repository.listInvoices(scope) })
}

export function useInvoice(id: string) {
  const repository = useInvoiceRepository()
  return useQuery({ queryKey: invoiceQueryKeys.invoice(id), queryFn: async () => (await repository.getInvoice(id)) ?? null })
}

export function useStartHappyPath(invoice: Invoice | undefined) {
  const repository = useInvoiceRepository()
  const queryClient = useQueryClient()
  const started = useRef(false)

  useEffect(() => {
    if (!invoice || invoice.scenario !== 'HAPPY_PATH' || invoice.pipelineStatus !== 'DOWNLOADED' || invoice.autoRun || started.current) return
    started.current = true
    void repository.startAutomaticFlow(invoice.id).then((updated) => {
      queryClient.setQueryData(invoiceQueryKeys.invoice(invoice.id), updated)
      void queryClient.invalidateQueries({ queryKey: ['invoices'] })
    })
  }, [invoice, queryClient, repository])
}
