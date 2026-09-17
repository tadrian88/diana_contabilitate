import { createContext, useCallback, useContext, useEffect, useMemo, useState, type PropsWithChildren } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { apiClient } from './auth-api'

export interface AuthUser { id: string; email: string; persona: 'CONTABIL' }
type AuthStatus = 'loading' | 'authenticated' | 'anonymous'

interface AuthValue {
  status: AuthStatus
  user: AuthUser | null
  login(email: string, password: string): Promise<void>
  logout(): Promise<void>
}

const AuthContext = createContext<AuthValue | null>(null)

export function AuthProvider({ children, initialUser }: PropsWithChildren<{ initialUser?: AuthUser }>) {
  const queryClient = useQueryClient()
  const [status, setStatus] = useState<AuthStatus>(initialUser ? 'authenticated' : 'loading')
  const [user, setUser] = useState<AuthUser | null>(initialUser ?? null)
  const anonymous = useCallback(() => { queryClient.clear(); setUser(null); setStatus('anonymous') }, [queryClient])

  useEffect(() => {
    if (initialUser) return
    let active = true
    apiClient.get<{ user: AuthUser }>('/auth/session')
      .then(({ data }) => { if (active) { setUser(data.user); setStatus('authenticated') } })
      .catch(() => { if (active) anonymous() })
    const onUnauthorized = () => anonymous()
    window.addEventListener('diana:unauthorized', onUnauthorized)
    return () => { active = false; window.removeEventListener('diana:unauthorized', onUnauthorized) }
  }, [anonymous, initialUser])

  const value = useMemo<AuthValue>(() => ({
    status,
    user,
    login: async (email, password) => {
      const { data } = await apiClient.post<{ user: AuthUser }>('/auth/login', { email, password })
      setUser(data.user); setStatus('authenticated')
    },
    logout: async () => {
      try { await apiClient.post('/auth/logout') } finally { anonymous() }
    },
  }), [anonymous, status, user])

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}

export function useAuth() {
  const value = useContext(AuthContext)
  if (!value) throw new Error('useAuth must be used inside AuthProvider')
  return value
}
