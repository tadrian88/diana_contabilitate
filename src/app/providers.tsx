import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { useState, type PropsWithChildren } from 'react'
import type { InvoiceRepository } from '../repositories/invoiceRepository'
import { RepositoryProvider } from './repository-context'
import { ScopeProvider } from './scope-context'
import { ThemeProvider } from './theme-context'
import { AutomaticWorkflowRunner } from './AutomaticWorkflowRunner'

export function AppProviders({ children, repository }: PropsWithChildren<{ repository?: InvoiceRepository }>) {
  const [queryClient] = useState(() => new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: 30_000 } },
  }))

  return (
    <QueryClientProvider client={queryClient}>
      <RepositoryProvider repository={repository}>
        <ThemeProvider>
          <ScopeProvider>
            <AutomaticWorkflowRunner />
            {children}
          </ScopeProvider>
        </ThemeProvider>
      </RepositoryProvider>
    </QueryClientProvider>
  )
}
