import { useQuery } from '@tanstack/react-query'
import { useClientScope } from '../../app/scope-context'
import type { ValidationTaskInboxItem } from '../../domain/invoice'
import { useInvoiceRepository } from '../../app/repository-context'
import { queryKeys } from '../../app/queryKeys'

export type TaskInboxItem = ValidationTaskInboxItem

export function useTaskInbox() {
  const { scope } = useClientScope()
  const repository = useInvoiceRepository()
  const query = useQuery({ queryKey: queryKeys.tasks.list(scope), queryFn: () => repository.listValidationTasks(scope) })
  const items = [...(query.data ?? [])].sort((left, right) => right.task.createdAt.localeCompare(left.task.createdAt))

  return {
    items,
    isLoading: query.isLoading,
    isError: query.isError,
  }
}
