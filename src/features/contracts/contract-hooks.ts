import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useInvoiceRepository } from '../../app/repository-context'
import type { ClientScope } from '../../domain/invoice'
import { queryKeys } from '../../app/queryKeys'
import { ingestionKeys } from './contract-ingestion-hooks'

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

export function useDeleteContract(){const repository=useInvoiceRepository();const cache=useQueryClient();return useMutation({mutationFn:({id,revision}:{id:string;revision:number})=>repository.deleteContract(id,revision),onSuccess:async()=>{await Promise.all([cache.invalidateQueries({queryKey:queryKeys.contracts.root}),cache.invalidateQueries({queryKey:ingestionKeys.root})])}})}
export function useArchiveContract(){const repository=useInvoiceRepository();const cache=useQueryClient();return useMutation({mutationFn:({id,revision}:{id:string;revision:number})=>repository.archiveContract(id,revision),onSuccess:async()=>{await cache.invalidateQueries({queryKey:queryKeys.contracts.root})}})}
export function useConfirmProposedCommercialRule(clientId:string,documentId:string){const repository=useInvoiceRepository();const cache=useQueryClient();return useMutation({mutationFn:({ruleId,rule}:{ruleId:string;rule?:import('../../repositories/invoiceRepository').CommercialRule})=>repository.confirmProposedCommercialRule(clientId,documentId,ruleId,rule),onSuccess:async()=>{await Promise.all([cache.invalidateQueries({queryKey:ingestionKeys.root}),cache.invalidateQueries({queryKey:queryKeys.contracts.root})])}})}
export function useActivateReviewedServicePrices(clientId:string,documentId:string){const repository=useInvoiceRepository();const cache=useQueryClient();return useMutation({mutationFn:()=>repository.activateReviewedServicePrices(clientId,documentId),onSuccess:async()=>{await Promise.all([cache.invalidateQueries({queryKey:ingestionKeys.root}),cache.invalidateQueries({queryKey:queryKeys.contracts.root})])}})}
