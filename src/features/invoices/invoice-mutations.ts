import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useInvoiceRepository } from '../../app/repository-context'
import type { Invoice } from '../../domain/invoice'

function useUpdateInvoice<T>(invoiceId: string, mutationFn: (value: T) => Promise<Invoice>) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn,
    onSuccess: (invoice) => {
      queryClient.setQueryData(['invoice', invoiceId], invoice)
      void queryClient.invalidateQueries({ queryKey: ['invoices'] })
      void queryClient.invalidateQueries({ queryKey: ['contracts'] })
    },
  })
}

export function useResolveContract(invoiceId: string) {
  const repository = useInvoiceRepository()
  return useUpdateInvoice(invoiceId, (contractId: string) => repository.resolveContractMatch(invoiceId, contractId))
}

export function useRequestContract(invoiceId: string) {
  const repository = useInvoiceRepository()
  return useUpdateInvoice(invoiceId, () => repository.requestContract(invoiceId))
}

export function useReviewClassification(invoiceId: string) {
  const repository = useInvoiceRepository()
  return useUpdateInvoice(invoiceId, ({ itemId, value }: { itemId: string; value?: string }) => repository.reviewClassification(invoiceId, itemId, value))
}
