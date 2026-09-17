import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useInvoiceRepository } from '../../app/repository-context'
import type { ClientScope } from '../../domain/invoice'
import type { CreateClientOverrideInput, CreateRuleVersionInput } from '../../repositories/invoiceRepository'
import { queryKeys } from '../../app/queryKeys'

export const ruleQueryKeys = {
  rules: queryKeys.rules.list,
  rule: queryKeys.rules.detail,
}

export function useRules(scope: ClientScope) {
  const repository = useInvoiceRepository()
  return useQuery({ queryKey: ruleQueryKeys.rules(scope), queryFn: () => repository.listRules(scope) })
}

export function useRule(id: string) {
  const repository = useInvoiceRepository()
  return useQuery({ queryKey: ruleQueryKeys.rule(id), queryFn: async () => (await repository.getRule(id)) ?? null })
}

export function useCreateRuleVersion(id: string) {
  const repository = useInvoiceRepository()
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (input: CreateRuleVersionInput) => repository.createRuleVersion(id, input),
    onSuccess: (rule) => {
      queryClient.setQueryData(ruleQueryKeys.rule(id), rule)
      void queryClient.invalidateQueries({ queryKey: queryKeys.rules.root })
    },
  })
}

export function useCreateClientOverride(id: string) {
  const repository = useInvoiceRepository()
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (input: CreateClientOverrideInput) => repository.createClientOverride(id, input),
    onSuccess: (rule) => {
      queryClient.setQueryData(ruleQueryKeys.rule(rule.id), rule)
      void queryClient.invalidateQueries({ queryKey: queryKeys.rules.root })
    },
  })
}
