import { describe, expect, it } from 'vitest'
import type { AxiosInstance, AxiosResponse } from 'axios'
import { ApiInvoiceReadRepository } from './ApiInvoiceReadRepository'

function client(response: Partial<AxiosResponse>) {
  return {
    get: async () => response as AxiosResponse,
    post: async () => response as AxiosResponse,
  } as Pick<AxiosInstance, 'get' | 'post'>
}

describe('ApiInvoiceReadRepository', () => {
  it('uses explicit credential-free SPV connection DTOs and command endpoints', async () => {
    const calls: Array<{ method: string; url: string; key?: string }> = []
    const dto = { status: 'CONNECTED', environment: 'TEST', lastSyncStatus: 'SUCCEEDED', importAutomatic: true, configurationReady: true, identityValidation: 'NOT_AVAILABLE' }
    const repository = new ApiInvoiceReadRepository({
      get: async (url: string) => { calls.push({ method: 'GET', url }); return { status: 200, data: dto } },
      post: async (url: string, _body?: unknown, config?: { headers?: Record<string, string> }) => { calls.push({ method: 'POST', url, key: config?.headers?.['Idempotency-Key'] }); return { status: url.endsWith('/oauth/start') ? 200 : 202, data: url.endsWith('/oauth/start') ? { authorizationUrl: 'https://anaf.example/authorize' } : dto } },
    } as Pick<AxiosInstance, 'get' | 'post'>)
    await expect(repository.getSPVConnection('client-a')).resolves.toEqual(dto)
    await expect(repository.startSPVOAuth('client-a')).resolves.toBe('https://anaf.example/authorize')
    await repository.requestSPVSync('client-a'); await repository.disconnectSPV('client-a')
    expect(calls.map((item) => item.url)).toEqual(['/clients/client-a/spv', '/clients/client-a/spv/oauth/start', '/clients/client-a/spv/sync', '/clients/client-a/spv/disconnect'])
    expect(calls[2].key).toBeTruthy(); expect(calls[3].key).toBeTruthy()
    expect(JSON.stringify(dto)).not.toMatch(/token|ciphertext|secret/i)
  })
  it('uses the backend as the sole authority for complete and client-scoped invoice lists', async () => {
    const calls: Array<{ url: string; params?: unknown }> = []
    const dto = { id: 'inv-api-list', clientId: 'client-alfa', supplierName: 'Furnizor API', documentNumber: 'API-LIST-1', issueDate: '2026-09-11T10:00:00Z', total: { amount: 119, currency: 'RON' }, spvReference: 'SPV-API-LIST', pipelineStatus: 'READY_FOR_SAGA', sagaStatus: 'READY', revision: 1, activity: [], lines: [] }
    const repository = new ApiInvoiceReadRepository({
      get: async (url: string, config?: { params?: unknown }) => { calls.push({ url, params: config?.params }); return { status: 200, data: [dto] } },
      post: async () => ({ status: 200, data: dto }),
    } as Pick<AxiosInstance, 'get' | 'post'>)
    await expect(repository.listInvoices('all')).resolves.toMatchObject([{ id: 'inv-api-list', authority: 'API' }])
    await repository.listInvoices('client-alfa')
    expect(calls).toEqual([{ url: '/invoices', params: undefined }, { url: '/invoices', params: { clientId: 'client-alfa' } }])
  })

  it('maps the backend read DTO and keeps demo controls client-side', async () => {
    const repository = new ApiInvoiceReadRepository(client({ status: 200, data: {
      id: 'inv-real', clientId: 'client-alfa', supplierName: 'Furnizor Demo', documentNumber: 'REAL-1',
      issueDate: '2026-09-11T10:00:00Z', total: { amount: 123.45, currency: 'RON' }, spvReference: 'SPV-REAL-1',
      pipelineStatus: 'EXPORTED', sagaStatus: 'EXPORTED', revision: 1, activity: [],
      lines: [{ id: 'line-1', position: 1, description: 'Servicii', unit: 'BUC', vatRate: 19,
        vatValue: { amount: 19, currency: 'RON' }, quantity: 1, unitPrice: { amount: 100, currency: 'RON' },
        netValue: { amount: 100, currency: 'RON' }, totalValue: { amount: 119, currency: 'RON' } }],
    }}))
    const invoice = await repository.getInvoice('inv-real')
    expect(invoice).toMatchObject({ id: 'inv-real', total: { amount: 123.45 }, autoRun: false, scenario: 'PROCESSING' })
    expect(invoice?.pipelinePath.at(-1)).toBe('EXPORTED')
    expect(invoice?.lines[0]).toMatchObject({ vatLabel: '19%', grossValue: { amount: 119 }, classifications: [] })
  })

  it('keeps API 404 as not-found and never substitutes mock authority', async () => {
    const repository = new ApiInvoiceReadRepository(client({ status: 404, data: {} }))
    await expect(repository.getInvoice('missing')).resolves.toBeUndefined()
  })

  it('does not execute a backend-owned mutation when its persisted invoice is missing', async () => {
    let posts = 0
    const repository = new ApiInvoiceReadRepository({
      get: async () => ({ status: 404, data: {} }),
      post: async () => { posts++; return { status: 200, data: {} } },
    } as Pick<AxiosInstance, 'get' | 'post'>)
    await expect(repository.requestContract('mock-only-id')).rejects.toThrow('Invoice was not found.')
    expect(posts).toBe(0)
  })

  it('maps persisted validation tasks without requiring full invoice payloads', async () => {
    const repository = new ApiInvoiceReadRepository(client({ status: 200, data: [{
      task: { id: 'task-1', type: 'MISSING_CONTRACT', status: 'OPEN', createdAt: '2026-09-11T10:00:00Z', updatedAt: '2026-09-11T10:00:00Z', title: 'Contract lipsă', reason: 'Nu există contract.', revision: 1, contractRequested: false },
      invoice: { id: 'inv-1', clientId: 'client-alfa', supplierName: 'Furnizor', documentNumber: 'INV-1', issueDate: '2026-09-11T10:00:00Z', total: { amount: 119, currency: 'RON' }, spvReference: 'SPV-1', pipelineStatus: 'AWAITING_CONTRACT', sagaStatus: 'NOT_READY' },
      client: { id: 'client-alfa', name: 'Client Alfa', cui: 'RO-ALFA' },
    }] }))
    const items = await repository.listValidationTasks('all')
    expect(items[0]).toMatchObject({ task: { id: 'task-1', revision: 1 }, invoice: { id: 'inv-1', authority: 'API', task: { status: 'OPEN' } }, client: { id: 'client-alfa' } })
  })

  it('requests a missing contract with task revision and idempotency key', async () => {
    let posted: { url?: string; body?: unknown; key?: string } = {}
    const waitingResponse = {
      id: 'inv-1', clientId: 'client-alfa', supplierName: 'Furnizor', documentNumber: 'INV-1',
      issueDate: '2026-09-11T10:00:00Z', total: { amount: 119, currency: 'RON' }, spvReference: 'SPV-1',
      pipelineStatus: 'AWAITING_CONTRACT', sagaStatus: 'NOT_READY', revision: 1, lines: [], activity: [],
      task: { id: 'task-1', type: 'MISSING_CONTRACT', status: 'WAITING', createdAt: '2026-09-11T10:00:00Z', updatedAt: '2026-09-11T11:00:00Z', waitingSince: '2026-09-11T11:00:00Z', title: 'Contract lipsă', reason: 'Solicitat.', revision: 2, contractRequested: true },
    }
    const http = {
      get: async () => ({ status: 200, data: { ...waitingResponse, task: { ...waitingResponse.task, status: 'OPEN', revision: 1, waitingSince: undefined, contractRequested: false } } }),
      post: async (url: string, body: unknown, config: { headers: Record<string, string> }) => {
        posted = { url, body, key: config.headers['Idempotency-Key'] }
        return { status: 200, data: waitingResponse }
      },
    } as Pick<AxiosInstance, 'get' | 'post'>
    const repository = new ApiInvoiceReadRepository(http)
    const updated = await repository.requestContract('inv-1')
    expect(posted).toMatchObject({ url: '/invoices/inv-1/contract-requests', body: { taskId: 'task-1', expectedRevision: 1 } })
    expect(posted.key).toBeTruthy()
    expect(updated).toMatchObject({ pipelineStatus: 'AWAITING_CONTRACT', task: { status: 'WAITING', contractRequested: true } })
  })

  it('maps API-backed contract reads and associated invoice context', async () => {
    const contract = {
      id: 'contract-1', clientId: 'client-alfa', supplierName: 'Furnizor', supplierCui: 'RO-1', reference: 'CTR-1',
      period: '01.01.2026 – 31.12.2026', value: { amount: 100, currency: 'RON' }, currency: 'RON',
      unitType: 'BUC', paymentTerms: '30 zile', sourceReference: 'SRC-1', revision: 1,
    }
    const http = {
      get: async (url: string) => ({ status: 200, data: url.endsWith('/invoices') ? [{
        id: 'invoice-1', clientId: 'client-alfa', supplierName: 'Furnizor', documentNumber: 'INV-1',
        issueDate: '2026-09-15T10:00:00Z', total: { amount: 119, currency: 'RON' }, spvReference: 'SPV-1',
        pipelineStatus: 'EXPORTED', sagaStatus: 'EXPORTED',
      }] : url === '/contracts' ? [contract] : contract }),
      post: async () => ({ status: 200, data: {} }),
    } as Pick<AxiosInstance, 'get' | 'post'>
    const repository = new ApiInvoiceReadRepository(http)
    await expect(repository.listContracts('all')).resolves.toEqual([contract])
    await expect(repository.getContract('contract-1')).resolves.toEqual(contract)
    await expect(repository.listContractInvoices('contract-1')).resolves.toMatchObject([{ id: 'invoice-1', authority: 'API', selectedContractId: 'contract-1' }])
  })

  it('confirms only a persisted match candidate with invoice and task revisions', async () => {
    let posted: { url?: string; body?: unknown; key?: string } = {}
    const response = {
      id: 'invoice-1', clientId: 'client-alfa', supplierName: 'Furnizor', documentNumber: 'INV-1', issueDate: '2026-09-15T10:00:00Z',
      total: { amount: 119, currency: 'RON' }, spvReference: 'SPV-1', pipelineStatus: 'DEDUPE_CHECKED' as const, sagaStatus: 'NOT_READY' as const,
      revision: 3, activity: [], lines: [], selectedContractId: 'contract-1',
      contract: { id: 'contract-1', reference: 'CTR-1', supplierName: 'Furnizor', period: '01.01.2026 – 31.12.2026', value: { amount: 100, currency: 'RON' }, currency: 'RON', unitType: 'BUC', paymentTerms: '30 zile' },
    }
    const invoice = {
      ...response, pipelineStatus: 'AWAITING_MATCH_CONFIRM' as const, revision: 2, scenario: 'PROCESSING' as const,
      primaryDemo: false, pipelinePath: [], autoRun: false, authority: 'API' as const,
      task: { id: 'task-1', type: 'CONTRACT_MATCH' as const, status: 'OPEN' as const, createdAt: '2026-09-15T10:00:00Z', title: 'Confirmă', reason: 'Review', revision: 1 },
    }
    const http = {
      get: async () => ({ status: 200, data: invoice }),
      post: async (url: string, body: unknown, config: { headers: Record<string, string> }) => {
        posted = { url, body, key: config.headers['Idempotency-Key'] }
        return { status: 200, data: response }
      },
    } as Pick<AxiosInstance, 'get' | 'post'>
    const repository = new ApiInvoiceReadRepository(http)
    const updated = await repository.resolveContractMatch(invoice.id, 'contract-1')
    expect(posted).toMatchObject({
      url: '/invoices/invoice-1/contract-confirmations',
      body: { taskId: 'task-1', contractId: 'contract-1', expectedInvoiceRevision: 2, expectedTaskRevision: 1 },
    })
    expect(posted.key).toBeTruthy()
    expect(updated).toMatchObject({ pipelineStatus: 'DEDUPE_CHECKED', selectedContractId: 'contract-1', contract: { reference: 'CTR-1' } })
  })

  it('maps classifications and submits review with all optimistic revisions', async () => {
    let posted: { url?: string; body?: any; key?: string } = {}
    const dto = {
      id: 'invoice-classification', clientId: 'client-alfa', supplierName: 'Furnizor', documentNumber: 'INV-C', issueDate: '2026-09-20T10:00:00Z',
      total: { amount: 119, currency: 'RON' }, spvReference: 'SPV-C', pipelineStatus: 'AWAITING_REVIEW' as const, sagaStatus: 'NOT_READY' as const, revision: 3, activity: [],
      task: { id: 'task-c', type: 'CLASSIFICATION' as const, status: 'OPEN' as const, createdAt: '2026-09-20T10:00:00Z', title: 'Review', reason: 'Demo', revision: 1, classificationItems: [{ id: 'classification-1', lineId: 'line-1', lineLabel: 'Linia 1', dimension: 'ACCOUNT' as const, proposedValue: 'Demo', confidence: 'Opaque', explanation: 'Demo', legalBasis: 'Exemplu demonstrativ — bază legală nevalidată', status: 'PENDING' as const, revision: 1 }] },
      lines: [{ id: 'line-1', position: 1, description: 'Demo', unit: 'BUC', vatRate: 19, vatValue: { amount: 19, currency: 'RON' }, quantity: 1, unitPrice: { amount: 100, currency: 'RON' }, netValue: { amount: 100, currency: 'RON' }, totalValue: { amount: 119, currency: 'RON' }, classifications: [{ id: 'classification-1', dimension: 'ACCOUNT' as const, value: 'Demo', confidence: 'Opaque', explanation: 'Demo', legalBasis: 'Exemplu demonstrativ — bază legală nevalidată', status: 'PENDING' as const, revision: 1 }] }],
    }
    const http = {
      get: async () => ({ status: 200, data: dto }),
      post: async (url: string, body: unknown, config: { headers: Record<string, string> }) => { posted = { url, body, key: config.headers['Idempotency-Key'] }; return { status: 200, data: { ...dto, pipelineStatus: 'READY_FOR_SAGA', revision: 4, task: undefined } } },
    } as Pick<AxiosInstance, 'get' | 'post'>
    const repository = new ApiInvoiceReadRepository(http)
    const invoice = await repository.getInvoice(dto.id)
    expect(invoice?.lines[0].classifications).toHaveLength(1)
    await repository.reviewClassification(dto.id, 'classification-1', 'Corectat demo')
    expect(posted).toMatchObject({ url: '/invoices/invoice-classification/classification-decisions', body: { taskId: 'task-c', classificationId: 'classification-1', expectedInvoiceRevision: 3, expectedTaskRevision: 1, expectedClassificationRevision: 1, correctedValue: 'Corectat demo' } })
    expect(posted.key).toBeTruthy()
  })

  it('uses backend rule reads and domain-specific version/override commands', async () => {
    const rule = { id: 'rule-1', reference: 'REG-1', name: 'Demo', category: 'ACCOUNT' as const, scope: 'GLOBAL' as const, revision: 1, versions: [{ version: 1, effectiveFrom: '2026-01-01', criteria: 'Demo', result: 'Demo', legalBasis: 'Exemplu demonstrativ — bază legală nevalidată', createdAt: '2026-01-01T00:00:00Z', actor: 'Sistem' }] }
    const posts: Array<{ url: string; body: any }> = []
    const http = {
      get: async (url: string) => ({ status: 200, data: url === '/rules' ? [rule] : rule }),
      post: async (url: string, body: any) => { posts.push({ url, body }); return { status: 200, data: rule } },
    } as Pick<AxiosInstance, 'get' | 'post'>
    const repository = new ApiInvoiceReadRepository(http)
    await expect(repository.listRules('all')).resolves.toEqual([rule])
    await repository.createRuleVersion('rule-1', { criteria: 'Nou', result: 'Nou', effectiveFrom: '2027-01-01' })
    await repository.createClientOverride('rule-1', { clientId: 'client-alfa', criteria: 'Override', result: 'Override', effectiveFrom: '2027-01-01' })
    expect(posts).toMatchObject([{ url: '/rules/rule-1/versions', body: { expectedRevision: 1 } }, { url: '/rules/rule-1/client-overrides', body: { clientId: 'client-alfa' } }])
  })
})
