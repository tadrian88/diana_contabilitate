import {useMutation,useQuery,useQueryClient} from '@tanstack/react-query'
import {useInvoiceRepository} from '../../app/repository-context'
import {queryKeys} from '../../app/queryKeys'
import type {ClientWrite} from '../../domain/client-management'
export function useClientDetail(id:string){const repo=useInvoiceRepository();return useQuery({queryKey:['client-settings',id],queryFn:()=>repo.getClientDetail(id),enabled:!!id})}
export function useClientWrite(){const repo=useInvoiceRepository();const cache=useQueryClient();return useMutation({mutationFn:({input,key}:{input:ClientWrite;key:string})=>repo.writeClient(input,key),onSuccess:d=>{cache.setQueryData(['client-settings',d.client.id],d);void cache.invalidateQueries({queryKey:queryKeys.clients});void cache.invalidateQueries({queryKey:queryKeys.spv(d.client.id)});void cache.invalidateQueries({queryKey:['client-settings',d.client.id]})}})}

export function clientWriteError(error:unknown):string{const message=(error as {response?:{data?:{message?:unknown}}})?.response?.data?.message;return typeof message==='string'?message:'Salvarea a eșuat. Verifică datele și reîncarcă setările pentru revizia curentă.'}
