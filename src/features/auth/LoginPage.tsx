import { Eye, EyeOff, FileText, LoaderCircle } from 'lucide-react'
import { useState, type FormEvent } from 'react'
import { Navigate, useLocation, useNavigate } from 'react-router-dom'
import { useAuth } from './AuthContext'

export function LoginPage() {
  const auth = useAuth()
  const location = useLocation()
  const navigate = useNavigate()
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [showPassword, setShowPassword] = useState(false)
  const [pending, setPending] = useState(false)
  const [error, setError] = useState('')

  if (auth.status === 'loading') return <div className="grid min-h-screen place-items-center bg-[var(--surface)]"><LoaderCircle className="size-8 animate-spin text-[var(--accent)]" aria-label="Se verifică sesiunea" /></div>
  if (auth.status === 'authenticated') return <Navigate to="/" replace />

  const submit = async (event: FormEvent) => {
    event.preventDefault()
    setError(''); setPending(true)
    try {
      await auth.login(email, password)
      const from = (location.state as { from?: { pathname?: string; search?: string } } | null)?.from
      const target = from?.pathname?.startsWith('/') && !from.pathname.startsWith('//') ? from.pathname + (from.search ?? '') : '/'
      navigate(target, { replace: true })
    } catch {
      setError('Email sau parolă incorectă.')
    } finally { setPending(false) }
  }

  return (
    <main className="grid min-h-screen bg-[var(--surface)] lg:grid-cols-2">
      <section className="auth-brand-panel relative hidden overflow-hidden p-12 text-white lg:flex lg:flex-col" aria-label="Diana">
        <div className="relative z-10 flex items-center gap-3">
          <span className="grid size-11 place-items-center rounded-xl bg-white/10 ring-1 ring-white/20"><FileText className="size-6" /></span>
          <div><div className="text-xl font-bold tracking-tight">Diana</div><div className="text-[10px] font-semibold uppercase tracking-[0.18em] text-white/60">Contabilitate digitală</div></div>
        </div>
        <div className="relative z-10 mt-auto text-sm font-semibold tracking-wide text-white/70">SPV → SAGA</div>
      </section>
      <section className="flex min-h-screen items-center justify-center px-6 py-12 sm:px-10 lg:px-16">
        <div className="w-full max-w-md">
          <div className="mb-10 flex items-center gap-3 lg:hidden"><span className="grid size-10 place-items-center rounded-xl bg-[var(--sidebar)] text-white"><FileText className="size-5" /></span><span className="text-xl font-bold">Diana</span></div>
          <div className="mb-8"><h1 className="text-3xl font-bold tracking-tight">Autentificare</h1><p className="mt-2 text-sm text-[var(--text-secondary)]">Introdu datele de acces pentru a continua.</p></div>
          <form onSubmit={submit} className="space-y-5" noValidate>
            {error && <div role="alert" className="rounded-lg border border-[var(--danger-border)] bg-[var(--danger-soft)] px-4 py-3 text-sm font-medium text-[var(--danger)]">{error}</div>}
            <div className="space-y-2"><label htmlFor="login-email" className="text-sm font-semibold">Email</label><input id="login-email" type="email" autoComplete="email" required value={email} onChange={(event) => setEmail(event.target.value)} className="auth-input" placeholder="nume@companie.ro" /></div>
            <div className="space-y-2"><label htmlFor="login-password" className="text-sm font-semibold">Parolă</label><div className="relative"><input id="login-password" type={showPassword ? 'text' : 'password'} autoComplete="current-password" required minLength={12} value={password} onChange={(event) => setPassword(event.target.value)} className="auth-input pr-12" placeholder="Introdu parola" /><button type="button" onClick={() => setShowPassword((value) => !value)} className="absolute right-3 top-1/2 -translate-y-1/2 rounded-md p-1 text-[var(--text-muted)] hover:text-[var(--text)]" aria-label={showPassword ? 'Ascunde parola' : 'Arată parola'}>{showPassword ? <EyeOff className="size-5" /> : <Eye className="size-5" />}</button></div></div>
            <button type="submit" disabled={pending || !email || password.length < 12} className="flex h-11 w-full items-center justify-center rounded-lg bg-[var(--accent)] text-sm font-semibold text-[var(--accent-contrast)] transition hover:bg-[var(--accent-hover)] focus:outline-none focus:ring-2 focus:ring-[var(--focus)] focus:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-55">{pending ? <LoaderCircle className="size-5 animate-spin" aria-label="Se autentifică" /> : 'Autentificare'}</button>
          </form>
          <p className="mt-7 text-center text-xs text-[var(--text-muted)]">Conturile sunt create de administrator.</p>
        </div>
      </section>
    </main>
  )
}
