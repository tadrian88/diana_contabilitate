import { BrowserRouter, Route, Routes } from 'react-router-dom'
import { DashboardPage } from '../features/dashboard/DashboardPage'
import { InvoiceDetailPage } from '../features/invoices/InvoiceDetailPage'
import { InvoiceListPage } from '../features/invoices/InvoiceListPage'
import { TaskInboxPage } from '../features/tasks/TaskInboxPage'
import { ContractsListPage } from '../features/contracts/ContractsListPage'
import { ContractDetailPage } from '../features/contracts/ContractDetailPage'
import { ContractUploadPage } from '../features/contracts/ContractUploadPage'
import { ContractDocumentReviewPage } from '../features/contracts/ContractDocumentReviewPage'
import { RulesListPage } from '../features/rules/RulesListPage'
import { RuleDetailPage } from '../features/rules/RuleDetailPage'
import { ClientsListPage } from '../features/clients/ClientsListPage'
import { CreateClientPage } from '../features/clients/CreateClientPage'
import { ClientDetailPage } from '../features/clients/ClientDetailPage'
import { AppShell } from './AppShell'
import { LoginPage } from '../features/auth/LoginPage'
import { ProtectedRoute } from '../features/auth/ProtectedRoute'

export function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route path="login" element={<LoginPage />} />
        <Route element={<ProtectedRoute />}>
        <Route element={<AppShell />}>
          <Route index element={<DashboardPage />} />
          <Route path="tasks" element={<TaskInboxPage />} />
          <Route path="invoices" element={<InvoiceListPage />} />
          <Route path="invoices/:invoiceId" element={<InvoiceDetailPage />} />
          <Route path="contracts" element={<ContractsListPage />} />
          <Route path="contracts/upload" element={<ContractUploadPage />} />
          <Route path="contracts/documents/:clientId/:documentId" element={<ContractDocumentReviewPage />} />
          <Route path="contracts/:contractId" element={<ContractDetailPage />} />
          <Route path="rules" element={<RulesListPage />} />
          <Route path="rules/:ruleId" element={<RuleDetailPage />} />
          <Route path="clients" element={<ClientsListPage />} />
          <Route path="clients/new" element={<CreateClientPage />} />
          <Route path="clients/:clientId" element={<ClientDetailPage />} />
        </Route>
        </Route>
      </Routes>
    </BrowserRouter>
  )
}
