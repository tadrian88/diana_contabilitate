import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useRef } from 'react'
import type { ClientScope, Invoice } from '../../domain/invoice'
import { useInvoiceRepository } from '../../app/repository-context'
import { queryKeys } from '../../app/queryKeys'
import { isMockWorkflowRepository } from '../../repositories/invoiceRepository'

export const invoiceQueryKeys = {
  invoices: queryKeys.invoices.list,
  invoice: queryKeys.invoices.detail,
  clients: queryKeys.clients,
}

export function useDownloadSagaArtifact(invoice: Invoice) {
	const repository = useInvoiceRepository()
	const queryClient = useQueryClient()
	return useMutation({
		mutationFn: () => repository.downloadSagaArtifact(invoice.clientId, invoice.id),
		onSuccess: () => void queryClient.invalidateQueries({ queryKey: queryKeys.invoices.detail(invoice.id) }),
	})
}

export function useConfirmSagaImport(invoice: Invoice) {
	const repository = useInvoiceRepository()
	const queryClient = useQueryClient()
	return useMutation({
		mutationFn: (note?: string) => {
			if (!invoice.sagaExport || !invoice.revision) throw new Error('Metadatele exportului SAGA nu sunt disponibile.')
			return repository.confirmSagaImport(invoice.clientId, invoice.id, invoice.sagaExport.attemptId, invoice.revision, note)
		},
		onSuccess: (updated) => {
			queryClient.setQueryData(queryKeys.invoices.detail(invoice.id), updated)
			void queryClient.invalidateQueries({ queryKey: queryKeys.invoices.root })
		},
	})
}

export function useClients() {
  const repository = useInvoiceRepository()
  return useQuery({ queryKey: queryKeys.clients, queryFn: () => repository.listClients() })
}

export function useInvoices(scope: ClientScope) {
  const repository = useInvoiceRepository()
  return useQuery({
    queryKey: queryKeys.invoices.list(scope),
    queryFn: () => repository.listInvoices(scope),
    refetchInterval: repository.runtimeAuthority === 'API' ? 1_000 : false,
  })
}

export function useInvoice(id: string) {
  const repository = useInvoiceRepository()
  return useQuery({
    queryKey: queryKeys.invoices.detail(id),
    queryFn: async () => (await repository.getInvoice(id)) ?? null,
    refetchInterval: (query) => {
      if (repository.runtimeAuthority !== 'API') return false
      const status = query.state.data?.pipelineStatus
      // ContractAvailable is asynchronous: a waiting invoice can change externally.
      if (status === 'AWAITING_CONTRACT') return 3_000
      return status && ['DOWNLOADED', 'ARCHIVED', 'MATCHING', 'DEDUPE_CHECKED', 'HEADER_READ', 'LINES_READ', 'COMMERCIAL_VALIDATING', 'COMMERCIALLY_VALIDATED', 'CLASSIFIED', 'READY_FOR_SAGA', 'EXPORTING'].includes(status) ? 750 : false
    },
  })
}

export function useReanalyzeClassification(invoice:Invoice){
 const repository=useInvoiceRepository();const queryClient=useQueryClient()
 return useMutation({mutationFn:()=>{if(!invoice.revision)throw new Error('Revizia facturii lipsește.');return repository.reanalyzeClassification(invoice.clientId,invoice.id,invoice.revision)},onSuccess:updated=>{queryClient.setQueryData(queryKeys.invoices.detail(invoice.id),updated);void queryClient.invalidateQueries({queryKey:queryKeys.invoices.root})}})
}

export function useCommercialValidation(invoice: Invoice | undefined) {
  const repository = useInvoiceRepository()
  return useQuery({
    // A 404 while COMMERCIAL_VALIDATING means "not written yet". The invoice
    // transition must select a fresh query rather than keep that cached null.
    queryKey: ['commercial-validation', invoice?.clientId, invoice?.id, invoice?.revision, invoice?.pipelineStatus],
    queryFn: async () => (await repository.getCommercialValidation(invoice!.clientId, invoice!.id)) ?? null,
    enabled: !!invoice && !['DOWNLOADED','ARCHIVED','MATCHING','AWAITING_CONTRACT','AWAITING_MATCH_CONFIRM','DEDUPE_CHECKED','HEADER_READ','LINES_READ','DUPLICATE'].includes(invoice.pipelineStatus),
  })
}

export function useResolveCommercialValidation(invoice: Invoice) {
  const repository = useInvoiceRepository()
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (input: {runId:string;findingId?:string;expectedInvoiceRevision:number;action:'ACCEPT_EXCEPTION'|'WAIT_FOR_CORRECTION'|'RERUN';reason?:string}) => repository.resolveCommercialValidation(invoice.clientId, invoice.id, input),
    onSuccess: () => {
      void queryClient.invalidateQueries({queryKey:['commercial-validation', invoice.clientId, invoice.id]})
      void queryClient.invalidateQueries({queryKey:queryKeys.invoices.detail(invoice.id)})
      void queryClient.invalidateQueries({queryKey:queryKeys.invoices.root})
    },
  })
}

export function usePutCommercialVariable(invoice: Invoice, dossierId: string) {
  const repository = useInvoiceRepository()
  return useMutation({
    mutationFn: (input: {name:string;value:string;source:'MANUAL';sourceReference:string;periodStart?:string;periodEnd?:string}) => repository.putCommercialVariable(invoice.clientId, dossierId, input),
  })
}

// Confirms every chosen line → service mapping, then re-runs the validation so
// the prices of the newly mapped lines are checked in the same step. Command
// keys are derived from the run so a retry after a partial failure replays the
// mappings already saved instead of conflicting with them.
export function useConfirmCommercialServiceAliases(invoice: Invoice) {
  const repository = useInvoiceRepository()
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (input:{runId:string;expectedInvoiceRevision:number;reuseForDossier:boolean;choices:Array<{lineId:string;serviceId:string}>}) => {
      for (const choice of input.choices) {
        await repository.confirmCommercialServiceAlias(invoice.clientId,{invoiceId:invoice.id,lineId:choice.lineId,serviceId:choice.serviceId,reuseForDossier:input.reuseForDossier},`${input.runId}:${choice.lineId}:${choice.serviceId}:${input.reuseForDossier?'dossier':'invoice'}`)
      }
      return repository.resolveCommercialValidation(invoice.clientId,invoice.id,{runId:input.runId,expectedInvoiceRevision:input.expectedInvoiceRevision,action:'RERUN'},`${input.runId}:rerun-after-mapping`)
    },
    onSettled: () => {
      void queryClient.invalidateQueries({queryKey:['commercial-validation', invoice.clientId, invoice.id]})
      void queryClient.invalidateQueries({queryKey:queryKeys.invoices.detail(invoice.id)})
      void queryClient.invalidateQueries({queryKey:queryKeys.invoices.root})
      void queryClient.invalidateQueries({queryKey:queryKeys.serviceAliases.root})
    },
  })
}

export function usePutCommercialDateFact(invoice: Invoice) {
  const repository=useInvoiceRepository()
  return useMutation({mutationFn:(input:{kind:'REMITTANCE'|'RECEIPT'|'ACCEPTANCE';date:string;sourceReference:string})=>repository.putCommercialDateFact(invoice.clientId,invoice.id,input)})
}

export function useStartHappyPath(invoice: Invoice | undefined) {
  const repository = useInvoiceRepository()
  const queryClient = useQueryClient()
  const started = useRef(false)

  useEffect(() => {
    if (!isMockWorkflowRepository(repository) || !invoice || invoice.scenario !== 'HAPPY_PATH' || invoice.pipelineStatus !== 'DOWNLOADED' || invoice.autoRun || started.current) return
    started.current = true
    void repository.startAutomaticFlow(invoice.id).then((updated) => {
      queryClient.setQueryData(queryKeys.invoices.detail(invoice.id), updated)
      void queryClient.invalidateQueries({ queryKey: queryKeys.invoices.root })
    })
  }, [invoice, queryClient, repository])
}
