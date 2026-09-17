import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { useState, type PropsWithChildren } from 'react'
import type { InvoiceRepository } from '../repositories/invoiceRepository'
import { RepositoryProvider } from './repository-context'
import { ScopeProvider } from './scope-context'
import { ThemeProvider } from './theme-context'
import { AuthProvider } from '../features/auth/AuthContext'
import type { AuthUser } from '../features/auth/AuthContext'

export function AppProviders({ children, repository, authUser }: PropsWithChildren<{ repository?: InvoiceRepository; authUser?: AuthUser }>) {
  const [queryClient] = useState(() => new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: 30_000 } },
  }))

  return (
    <QueryClientProvider client={queryClient}>
      <AuthProvider initialUser={authUser}>
        <RepositoryProvider repository={repository}>
          <ThemeProvider>
            <ScopeProvider>{children}</ScopeProvider>
          </ThemeProvider>
        </RepositoryProvider>
      </AuthProvider>
    </QueryClientProvider>
  )
}
