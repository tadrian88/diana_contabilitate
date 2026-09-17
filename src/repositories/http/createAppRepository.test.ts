import { afterEach, describe, expect, it, vi } from 'vitest'
import { createAppRepository } from './createAppRepository'
import { ApiInvoiceReadRepository } from './ApiInvoiceReadRepository'

afterEach(() => vi.unstubAllEnvs())
describe('real environment repository', () => {
  it('rejects mock repository configuration under the real label', () => {
    vi.stubEnv('VITE_ENVIRONMENT_LABEL', 'LOCAL — REAL INTEGRATIONS')
    vi.stubEnv('VITE_BACKEND_READS_ENABLED', 'false')
    expect(() => createAppRepository()).toThrow('Real environment requires the HTTP repository.')
  })
  it('selects HTTP with the real profile enabled', () => {
    vi.stubEnv('VITE_ENVIRONMENT_LABEL', 'LOCAL — REAL INTEGRATIONS')
    vi.stubEnv('VITE_BACKEND_READS_ENABLED', 'true')
    expect(createAppRepository()).toBeInstanceOf(ApiInvoiceReadRepository)
  })
})
