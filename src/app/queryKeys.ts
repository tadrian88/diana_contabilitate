import type { ClientScope } from '../domain/invoice'

export const queryKeys = {
  clients: ['clients'] as const,
  spv: (clientId: string) => ['clients', clientId, 'spv'] as const,
  invoices: {
    root: ['invoices'] as const,
    lists: ['invoices', 'list'] as const,
    list: (scope: ClientScope) => ['invoices', 'list', scope] as const,
    detail: (id: string) => ['invoices', 'detail', id] as const,
  },
  tasks: {
    root: ['validation-tasks'] as const,
    list: (scope: ClientScope) => ['validation-tasks', scope] as const,
  },
  contracts: {
    root: ['contracts'] as const,
    list: (scope: ClientScope) => ['contracts', 'list', scope] as const,
    detail: (id: string) => ['contracts', 'detail', id] as const,
    invoices: (id: string) => ['contracts', 'detail', id, 'invoices'] as const,
  },
  rules: {
    root: ['rules'] as const,
    list: (scope: ClientScope) => ['rules', 'list', scope] as const,
    detail: (id: string) => ['rules', 'detail', id] as const,
  },
}
