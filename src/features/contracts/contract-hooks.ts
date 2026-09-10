import { useQuery } from '@tanstack/react-query'
import { useInvoiceRepository } from '../../app/repository-context'
import type { ClientScope } from '../../domain/invoice'

export const contractQueryKeys = {
  contracts: (scope: ClientScope) => ['contracts', scope] as const,
  contract: (id: string) => ['contract', id] as const,
}

export function useContracts(scope: ClientScope) {
  const repository = useInvoiceRepository()
  return useQuery({ queryKey: contractQueryKeys.contracts(scope), queryFn: () => repository.listContracts(scope) })
}

export function useContract(id: string) {
  const repository = useInvoiceRepository()
  return useQuery({ queryKey: contractQueryKeys.contract(id), queryFn: async () => (await repository.getContract(id)) ?? null })
}
