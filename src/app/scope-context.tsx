import { createContext, useContext, useMemo, useState, type PropsWithChildren } from 'react'
import type { ClientScope } from '../domain/invoice'

interface ScopeContextValue {
  scope: ClientScope
  setScope: (scope: ClientScope) => void
}

const ScopeContext = createContext<ScopeContextValue | undefined>(undefined)

export function ScopeProvider({ children }: PropsWithChildren) {
  const [scope, setScope] = useState<ClientScope>('all')
  const value = useMemo(() => ({ scope, setScope }), [scope])
  return <ScopeContext.Provider value={value}>{children}</ScopeContext.Provider>
}

export function useClientScope() {
  const context = useContext(ScopeContext)
  if (!context) throw new Error('useClientScope trebuie folosit în ScopeProvider.')
  return context
}

