import { useQuery } from '@tanstack/react-query'
import { useInvoiceRepository } from '../../app/repository-context'
import type { ClientScope } from '../../domain/invoice'
import { queryKeys } from '../../app/queryKeys'

export const contractQueryKeys = {
  contracts: queryKeys.contracts.list,
  contract: queryKeys.contracts.detail,
  invoices: queryKeys.contracts.invoices,
}

export function useContractInvoices(id: string) {
  const repository = useInvoiceRepository()
  return useQuery({ queryKey: contractQueryKeys.invoices(id), queryFn: () => repository.listContractInvoices(id), enabled: Boolean(id) })
}

export function useContracts(scope: ClientScope) {
  const repository = useInvoiceRepository()
  return useQuery({ queryKey: contractQueryKeys.contracts(scope), queryFn: () => repository.listContracts(scope) })
}

export function useContract(id: string) {
  const repository = useInvoiceRepository()
  return useQuery({ queryKey: contractQueryKeys.contract(id), queryFn: async () => (await repository.getContract(id)) ?? null })
}
