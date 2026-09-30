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
  knowledge: {root:['approved-knowledge'] as const,list:(scope:ClientScope)=>['approved-knowledge',scope] as const},
  serviceAliases: {root:['commercial-service-aliases'] as const,list:(clientId:string,scopeId:string)=>['commercial-service-aliases',clientId,scopeId] as const},
  legislation: ['legislation-sources'] as const,
  aiUsage: {
    root: ['ai-usage'] as const,
    overview: (from: string, to: string) => ['ai-usage', 'overview', from, to] as const,
    client: (clientId: string, from: string, to: string) => ['ai-usage', 'client', clientId, from, to] as const,
    runs: (clientId: string, from: string, to: string) => ['ai-usage', 'runs', clientId, from, to] as const,
    run: (clientId: string, runKind: string, runId: string) => ['ai-usage', 'run', clientId, runKind, runId] as const,
  },
}
