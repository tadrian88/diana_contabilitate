import * as Select from '@radix-ui/react-select'
import { Building2, Check, ChevronDown, ClipboardCheck, FileText, Gauge, LogOut, Moon, Scale, ScrollText, Sun, UserRound } from 'lucide-react'
import { NavLink, Outlet, useLocation, useNavigate } from 'react-router-dom'
import type { ReactNode } from 'react'
import { Button } from '../components/ui/button'
import { useClients } from '../features/invoices/invoice-hooks'
import { useClientScope } from './scope-context'
import { useInvoiceRepository } from './repository-context'
import { useTheme } from './theme-context'
import { useAuth } from '../features/auth/AuthContext'
import { AutomaticWorkflowRunner } from './AutomaticWorkflowRunner'

const navItems = [
  { label: 'Dashboard', icon: Gauge, to: '/', enabled: true },
  { label: 'Task-uri', icon: ClipboardCheck, to: '/tasks', enabled: true },
  { label: 'Facturi', icon: FileText, to: '/invoices', enabled: true },
  { label: 'Contracte', icon: ScrollText, to: '/contracts', enabled: true },
  { label: 'Reguli', icon: Scale, to: '/rules', enabled: true },
  { label: 'Clienți', icon: Building2, to: '/clients', enabled: true },
]

export function AppShell() {
  const location = useLocation()
  const navigate = useNavigate()
  const repository = useInvoiceRepository()
  const { data: clients = [] } = useClients()
  const { scope, setScope } = useClientScope()
  const { theme, toggleTheme } = useTheme()
  const auth = useAuth()
  const activeClient = clients.find((client) => client.id === scope)
  const pageTitle = location.pathname.startsWith('/invoices/') ? 'Detaliu factură' : location.pathname === '/invoices' ? 'Facturi' : location.pathname.startsWith('/contracts/') ? 'Detaliu contract' : location.pathname === '/contracts' ? 'Contracte' : location.pathname.startsWith('/rules/') ? 'Detaliu regulă' : location.pathname === '/rules' ? 'Reguli' : location.pathname.startsWith('/clients/') ? 'Context client' : location.pathname === '/clients' ? 'Clienți' : location.pathname === '/tasks' ? 'Task Inbox' : 'Dashboard operațional'

  const changeScope = async (nextScope: string) => {
    let destination: string | undefined
    if (nextScope !== 'all') {
      const invoiceId = matchDetailId(location.pathname, 'invoices')
      const contractId = location.pathname==='/contracts/upload'?undefined:matchDetailId(location.pathname, 'contracts')
      const ruleId = matchDetailId(location.pathname, 'rules')
      const clientId = matchDetailId(location.pathname, 'clients')
      if (invoiceId && (await repository.getInvoice(invoiceId))?.clientId !== nextScope) destination = '/invoices'
      if (contractId && (await repository.getContract(contractId))?.clientId !== nextScope) destination = '/contracts'
      const documentClientId=location.pathname.match(/^\/contracts\/documents\/([^/]+)\//)?.[1]
      if(documentClientId&&documentClientId!==nextScope)destination='/contracts'
      const rule = ruleId ? await repository.getRule(ruleId) : undefined
      if (rule?.scope === 'CLIENT_OVERRIDE' && rule.clientId !== nextScope) destination = '/rules'
      if (clientId && clientId !== nextScope) destination = `/clients/${nextScope}`
    } else if (matchDetailId(location.pathname, 'clients')) destination = '/clients'
    setScope(nextScope)
    if (destination) navigate(destination)
  }

  return (
    <div className="grid min-h-screen grid-cols-[236px_1fr] bg-[var(--app-background)]">
      <AutomaticWorkflowRunner />
      <aside className="sticky top-0 flex h-screen flex-col bg-[var(--sidebar)] px-4 py-5 text-[var(--sidebar-text)]">
        <div className="mb-8 flex items-center gap-3 px-2">
          <div className="grid size-9 place-items-center rounded-xl bg-white/10 ring-1 ring-white/15">
            <FileText className="size-5" aria-hidden="true" />
          </div>
          <div>
            <div className="font-bold tracking-tight">ContaFlow</div>
            <div className="text-[10px] font-semibold uppercase tracking-[0.16em] text-[var(--sidebar-muted)]">SPV → SAGA</div>
          </div>
        </div>

        <nav aria-label="Navigație principală" className="space-y-1">
          {navItems.map(({ label, icon: Icon, to, enabled }) => enabled && to ? (
            <NavLink key={label} to={to} end={to === '/'} className={({ isActive }) => `flex items-center gap-3 rounded-lg px-3 py-2.5 text-sm font-medium ${isActive ? 'bg-white/10 text-white' : 'text-[var(--sidebar-muted)] hover:bg-white/5 hover:text-white'}`}>
              <Icon className="size-[18px]" aria-hidden="true" />{label}
            </NavLink>
          ) : (
            <div key={label} aria-disabled="true" className="flex items-center gap-3 rounded-lg px-3 py-2.5 text-sm font-medium text-[var(--sidebar-muted)] opacity-55">
              <Icon className="size-[18px]" aria-hidden="true" />{label}<span className="ml-auto text-[9px] uppercase">Ulterior</span>
            </div>
          ))}
        </nav>

        <div className="mt-auto rounded-xl border border-white/10 bg-white/5 p-3">
          <div className="text-xs font-semibold">{import.meta.env.VITE_ENVIRONMENT_LABEL ?? 'Frontend · Demo controlat'}</div>
          <div className="mt-1 text-[11px] leading-4 text-[var(--sidebar-muted)]">{import.meta.env.VITE_BACKEND_READS_ENABLED === 'true' ? 'Date persistate · integrare ANAF configurată pe server' : 'Date fictive · fără conexiuni SPV sau SAGA reale'}</div>
        </div>
      </aside>

      <div className="min-w-0">
        <header className="sticky top-0 z-20 flex h-[72px] items-center justify-between border-b border-[var(--border)] bg-[color:var(--surface-raised)]/95 px-8 backdrop-blur">
          <div>
            <div className="eyebrow">Frontend validation</div>
            <h1 className="mt-0.5 text-lg font-bold tracking-tight">{pageTitle}</h1>
          </div>
          <div className="flex items-center gap-3">
            <div className="mr-1 text-right">
              <div className="text-[10px] font-bold uppercase tracking-wider text-[var(--text-muted)]">Context activ</div>
              <div className="text-xs font-semibold" data-testid="active-scope">{activeClient?.name ?? 'Toți clienții'}</div>
            </div>
            <Select.Root value={scope} onValueChange={(value) => { void changeScope(value) }}>
              <Select.Trigger aria-label="Selectează clientul" className="flex h-10 min-w-56 items-center justify-between gap-3 rounded-lg border border-[var(--border-strong)] bg-[var(--surface)] px-3 text-sm font-medium focus:outline-none focus:ring-2 focus:ring-[var(--focus)]">
                <Select.Value />
                <Select.Icon><ChevronDown className="size-4" /></Select.Icon>
              </Select.Trigger>
              <Select.Portal>
                <Select.Content position="popper" sideOffset={6} className="select-content z-50 min-w-[var(--radix-select-trigger-width)] overflow-hidden rounded-xl border border-[var(--border)] bg-[var(--surface)] p-1 shadow-xl">
                  <Select.Viewport>
                    <SelectItem value="all">Toți clienții</SelectItem>
                    {clients.map((client) => <SelectItem key={client.id} value={client.id}>{client.name}</SelectItem>)}
                  </Select.Viewport>
                </Select.Content>
              </Select.Portal>
            </Select.Root>
            <Button variant="secondary" size="icon" onClick={toggleTheme} aria-label={theme === 'light' ? 'Activează tema întunecată' : 'Activează tema luminoasă'}>
              {theme === 'light' ? <Moon className="size-[18px]" /> : <Sun className="size-[18px]" />}
            </Button>
            <div className="ml-1 flex items-center gap-2 border-l border-[var(--border)] pl-3">
              <UserRound className="size-4 text-[var(--text-muted)]" aria-hidden="true" />
              <span className="max-w-52 truncate text-xs font-semibold">{auth.user?.email}</span>
              <Button variant="secondary" size="icon" onClick={() => void auth.logout()} aria-label="Deconectare"><LogOut className="size-[18px]" /></Button>
            </div>
          </div>
        </header>
        <main className="mx-auto max-w-[1480px] p-8"><Outlet /></main>
      </div>
    </div>
  )
}

function matchDetailId(pathname: string, resource: string) {
  const match = pathname.match(new RegExp(`^/${resource}/([^/]+)$`))
  return match?.[1]
}

function SelectItem({ value, children }: { value: string; children: ReactNode }) {
  return (
    <Select.Item value={value} className="relative flex cursor-pointer select-none items-center rounded-lg py-2 pl-8 pr-3 text-sm outline-none data-[highlighted]:bg-[var(--surface-subtle)]">
      <Select.ItemIndicator className="absolute left-2"><Check className="size-4 text-[var(--accent)]" /></Select.ItemIndicator>
      <Select.ItemText>{children}</Select.ItemText>
    </Select.Item>
  )
}
