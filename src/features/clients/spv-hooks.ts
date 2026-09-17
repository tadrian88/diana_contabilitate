import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useInvoiceRepository } from '../../app/repository-context'
import { queryKeys } from '../../app/queryKeys'

export function useSPVConnection(clientId: string) {
  const repository = useInvoiceRepository()
  return useQuery({
    queryKey: queryKeys.spv(clientId),
    queryFn: () => repository.getSPVConnection(clientId),
    enabled: Boolean(clientId),
    refetchInterval: (query) => query.state.data?.lastSyncStatus === 'RUNNING' ? 2_000 : false,
  })
}

export function useStartSPVOAuth(clientId: string) {
  const repository = useInvoiceRepository()
  return useMutation({ mutationFn: () => repository.startSPVOAuth(clientId) })
}

export function useRequestSPVSync(clientId: string) {
  const repository = useInvoiceRepository(); const queryClient = useQueryClient()
  return useMutation({ mutationFn: () => repository.requestSPVSync(clientId), onSuccess: (value) => { queryClient.setQueryData(queryKeys.spv(clientId), value); void queryClient.invalidateQueries({queryKey:['client-settings',clientId]}); void queryClient.invalidateQueries({ queryKey: queryKeys.spv(clientId) }) } })
}

export function useDisconnectSPV(clientId: string) {
  const repository = useInvoiceRepository(); const queryClient = useQueryClient()
  return useMutation({ mutationFn: () => repository.disconnectSPV(clientId), onSuccess: (value) => {queryClient.setQueryData(queryKeys.spv(clientId), value);void queryClient.invalidateQueries({queryKey:['client-settings',clientId]})} })
}
