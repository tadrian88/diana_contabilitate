import { render } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { AppProviders } from '../app/providers'
import { AppShell } from '../app/AppShell'
import { DashboardPage } from '../features/dashboard/DashboardPage'
import { InvoiceDetailPage } from '../features/invoices/InvoiceDetailPage'
import { InvoiceListPage } from '../features/invoices/InvoiceListPage'
import { TaskInboxPage } from '../features/tasks/TaskInboxPage'
import type { InvoiceRepository } from '../repositories/invoiceRepository'
import { ContractsListPage } from '../features/contracts/ContractsListPage'
import { ContractDetailPage } from '../features/contracts/ContractDetailPage'
import { RulesListPage } from '../features/rules/RulesListPage'
import { RuleDetailPage } from '../features/rules/RuleDetailPage'
import { ClientsListPage } from '../features/clients/ClientsListPage'
import { ClientDetailPage } from '../features/clients/ClientDetailPage'

export function renderApp(repository: InvoiceRepository, initialEntry = '/') {
  return render(
    <AppProviders repository={repository}>
      <MemoryRouter initialEntries={[initialEntry]}>
        <Routes>
          <Route element={<AppShell />}>
            <Route index element={<DashboardPage />} />
            <Route path="tasks" element={<TaskInboxPage />} />
            <Route path="invoices" element={<InvoiceListPage />} />
            <Route path="invoices/:invoiceId" element={<InvoiceDetailPage />} />
            <Route path="contracts" element={<ContractsListPage />} />
            <Route path="contracts/:contractId" element={<ContractDetailPage />} />
            <Route path="rules" element={<RulesListPage />} />
            <Route path="rules/:ruleId" element={<RuleDetailPage />} />
            <Route path="clients" element={<ClientsListPage />} />
            <Route path="clients/:clientId" element={<ClientDetailPage />} />
          </Route>
        </Routes>
      </MemoryRouter>
    </AppProviders>,
  )
}
