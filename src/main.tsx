import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { App } from './app/App'
import { AppProviders } from './app/providers'
import { createAppRepository } from './repositories/http/createAppRepository'
import './styles.css'

// The mock Playwright suite has no backend by design. Keep this bypass limited
// to a development server with backend reads explicitly disabled; production
// builds and every real-backend E2E suite still use the normal session flow.
const e2eMockAuthUser = import.meta.env.DEV
  && import.meta.env.VITE_E2E_MOCK_AUTH === 'true'
  && import.meta.env.VITE_BACKEND_READS_ENABLED !== 'true'
  ? { id: 'e2e-mock-user', email: 'e2e.mock@accountingtechco.test', persona: 'CONTABIL' as const }
  : undefined

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <AppProviders repository={createAppRepository()} authUser={e2eMockAuthUser}><App /></AppProviders>
  </StrictMode>,
)
