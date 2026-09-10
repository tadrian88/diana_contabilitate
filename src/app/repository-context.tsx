import { createContext, useContext, type PropsWithChildren } from 'react'
import { mockInvoiceRepository } from '../mocks/MockInvoiceRepository'
import type { InvoiceRepository } from '../repositories/invoiceRepository'

const RepositoryContext = createContext<InvoiceRepository>(mockInvoiceRepository)

export function RepositoryProvider({ repository = mockInvoiceRepository, children }: PropsWithChildren<{ repository?: InvoiceRepository }>) {
  return <RepositoryContext.Provider value={repository}>{children}</RepositoryContext.Provider>
}

export function useInvoiceRepository() {
  return useContext(RepositoryContext)
}

