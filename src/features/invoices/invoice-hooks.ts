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
      return status && ['DOWNLOADED', 'ARCHIVED', 'MATCHING', 'DEDUPE_CHECKED', 'HEADER_READ', 'LINES_READ', 'CLASSIFIED', 'READY_FOR_SAGA', 'EXPORTING'].includes(status) ? 750 : false
    },
  })
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
