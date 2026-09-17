import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useInvoiceRepository } from '../../app/repository-context'
import type { Invoice, DomainValue } from '../../domain/invoice'
import { queryKeys } from '../../app/queryKeys'

function useUpdateInvoice<T>(invoiceId: string, mutationFn: (value: T) => Promise<Invoice>) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn,
    onSuccess: (invoice) => {
      queryClient.setQueryData(queryKeys.invoices.detail(invoiceId), invoice)
      void queryClient.invalidateQueries({ queryKey: queryKeys.invoices.root })
      void queryClient.invalidateQueries({ queryKey: queryKeys.contracts.root })
      void queryClient.invalidateQueries({ queryKey: queryKeys.tasks.root })
    },
    onError: () => void queryClient.invalidateQueries({ queryKey: queryKeys.invoices.detail(invoiceId) }),
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
  return useUpdateInvoice(invoiceId, ({ itemId, value, typedValue, reason }: { itemId: string; value?: string; typedValue?: DomainValue; reason?: string }) => repository.reviewClassification(invoiceId, itemId, value, typedValue, reason))
}
