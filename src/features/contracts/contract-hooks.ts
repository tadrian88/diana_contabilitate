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
export function useConfirmProposedCommercialRule(clientId:string,documentId:string){const repository=useInvoiceRepository();const cache=useQueryClient();return useMutation({mutationFn:({ruleId,rule}:{ruleId:string;rule?:import('../../repositories/invoiceRepository').CommercialRule})=>repository.confirmProposedCommercialRule(clientId,documentId,ruleId,rule),onSuccess:async()=>{await Promise.all([cache.invalidateQueries({queryKey:ingestionKeys.root}),cache.invalidateQueries({queryKey:queryKeys.contracts.root}),cache.invalidateQueries({queryKey:['commercial-validation']})])}})}
export function useReviseConfirmedCommercialRule(clientId:string,documentId:string){const repository=useInvoiceRepository();const cache=useQueryClient();return useMutation({mutationFn:({ruleId,rule}:{ruleId:string;rule:import('../../repositories/invoiceRepository').CommercialRule})=>repository.reviseConfirmedCommercialRule(clientId,documentId,ruleId,rule),onSuccess:async()=>{await Promise.all([cache.invalidateQueries({queryKey:ingestionKeys.root}),cache.invalidateQueries({queryKey:queryKeys.contracts.root}),cache.invalidateQueries({queryKey:['commercial-validation']})])}})}
export function useActivateReviewedServicePrices(clientId:string,documentId:string){const repository=useInvoiceRepository();const cache=useQueryClient();return useMutation({mutationFn:()=>repository.activateReviewedServicePrices(clientId,documentId),onSuccess:async()=>{await Promise.all([cache.invalidateQueries({queryKey:ingestionKeys.root}),cache.invalidateQueries({queryKey:queryKeys.contracts.root}),cache.invalidateQueries({queryKey:['commercial-validation']})])}})}
// Closes a proposed clause: the reviewer states, with a reason, that it does
// not affect invoices, or it only restates the CUIs of the contract parties.
export function useDismissProposedCommercialClause(clientId:string,documentId:string){const repository=useInvoiceRepository();const cache=useQueryClient();return useMutation({mutationFn:({ruleId,reasonCode,reason}:{ruleId:string;reasonCode:import('../../repositories/invoiceRepository').ClauseDismissalCode;reason?:string})=>repository.dismissProposedCommercialClause(clientId,documentId,ruleId,{reasonCode,reason},`dismiss:${documentId}:${ruleId}:${reasonCode}`),onSuccess:async()=>{await Promise.all([cache.invalidateQueries({queryKey:ingestionKeys.root}),cache.invalidateQueries({queryKey:queryKeys.contracts.root}),cache.invalidateQueries({queryKey:['commercial-validation']})])}})}

// Learned wording → service associations of a contract dossier, addressed
// either by the dossier (invoice view) or by one of its documents (contract view).
export function useCommercialServiceAliases(clientId:string,scope:{dossierId:string}|{documentId:string}|undefined){
  const repository=useInvoiceRepository()
  const scopeId=scope?('dossierId' in scope?scope.dossierId:scope.documentId):''
  return useQuery({queryKey:queryKeys.serviceAliases.list(clientId,scopeId),queryFn:()=>repository.listCommercialServiceAliases(clientId,scope!),enabled:Boolean(clientId&&scopeId)})
}

export function useRevokeCommercialServiceAlias(clientId:string){
  const repository=useInvoiceRepository();const cache=useQueryClient()
  return useMutation({mutationFn:(aliasId:string)=>repository.revokeCommercialServiceAlias(clientId,aliasId,`revoke:${aliasId}`),onSuccess:async()=>{await Promise.all([cache.invalidateQueries({queryKey:queryKeys.serviceAliases.root}),cache.invalidateQueries({queryKey:['commercial-validation']})])}})
}
