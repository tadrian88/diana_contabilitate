import { LoaderCircle } from 'lucide-react'
import { Navigate, Outlet, useLocation } from 'react-router-dom'
import { useAuth } from './AuthContext'

export function ProtectedRoute() {
  const auth = useAuth()
  const location = useLocation()
  if (auth.status === 'loading') return <div className="grid min-h-screen place-items-center bg-[var(--app-background)]"><LoaderCircle className="size-8 animate-spin text-[var(--accent)]" aria-label="Se verifică sesiunea" /></div>
  if (auth.status === 'anonymous') return <Navigate to="/login" state={{ from: location }} replace />
  return <Outlet />
}
