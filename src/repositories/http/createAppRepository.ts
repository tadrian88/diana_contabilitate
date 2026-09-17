import { mockInvoiceRepository } from '../../mocks/MockInvoiceRepository'
import type { InvoiceRepository } from '../invoiceRepository'
import { ApiInvoiceReadRepository } from './ApiInvoiceReadRepository'

export function createAppRepository(): InvoiceRepository {
  if (import.meta.env.VITE_ENVIRONMENT_LABEL === 'LOCAL — REAL INTEGRATIONS' && import.meta.env.VITE_BACKEND_READS_ENABLED !== 'true') throw new Error('Real environment requires the HTTP repository.')
  if (import.meta.env.VITE_BACKEND_READS_ENABLED !== 'true') return mockInvoiceRepository
  return new ApiInvoiceReadRepository()
}
