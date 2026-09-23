import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useInvoiceRepository } from '../../app/repository-context'
import type { Invoice, DomainValue, AccountMappingAction } from '../../domain/invoice'
import { queryKeys } from '../../app/queryKeys'

function useUpdateInvoice<T>(invoiceId: string, mutationFn: (value: T) => Promise<Invoice>) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn,
    onSuccess: (invoice) => {
      queryClient.setQueryData(queryKeys.invoices.detail(invoiceId), invoice)
      // The mutation response is the authoritative detail snapshot. Refresh
      // invoice lists without refetching this detail, otherwise an older GET
      // can overwrite consecutive review decisions in the cache.
      void queryClient.invalidateQueries({ queryKey: queryKeys.invoices.lists })
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
  return useUpdateInvoice(invoiceId, ({ itemId, value, typedValue, reason, mappingAction, expectedMappingRevision }: { itemId: string; value?: string; typedValue?: DomainValue; reason?: string; mappingAction?:AccountMappingAction;expectedMappingRevision?:number }) => repository.reviewClassification(invoiceId, itemId, value, typedValue, reason, mappingAction, expectedMappingRevision))
}
