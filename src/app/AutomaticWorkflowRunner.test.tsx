import { act, render } from '@testing-library/react'
import type { AxiosInstance } from 'axios'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ApiInvoiceReadRepository } from '../repositories/http/ApiInvoiceReadRepository'
import { AppProviders } from './providers'

describe('AutomaticWorkflowRunner authority boundary', () => {
  afterEach(() => vi.useRealTimers())

  it('observes an API invoice without issuing any frontend progression command', async () => {
    vi.useFakeTimers()
    const post = vi.fn()
    const get = vi.fn(async () => ({ status: 200, data: [{
      id: 'api-downloaded', clientId: 'client-alfa', supplierName: 'Furnizor API', documentNumber: 'API-1',
      issueDate: '2026-09-11T10:00:00Z', total: { amount: 119, currency: 'RON' }, spvReference: 'SPV-API-1',
      pipelineStatus: 'DOWNLOADED', sagaStatus: 'NOT_READY', revision: 1, activity: [], lines: [],
    }] }))
    const repository = new ApiInvoiceReadRepository({ get, post } as unknown as Pick<AxiosInstance, 'get' | 'post'>)

    render(<AppProviders repository={repository}><div>API runtime</div></AppProviders>)
    await act(async () => { await Promise.resolve(); await Promise.resolve() })
    await act(async () => { vi.advanceTimersByTime(2_000); await Promise.resolve() })

    expect(get).toHaveBeenCalled()
    expect(post).not.toHaveBeenCalled()
  })
})
