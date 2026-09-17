import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { App } from './app/App'
import { AppProviders } from './app/providers'
import { createAppRepository } from './repositories/http/createAppRepository'
import './styles.css'

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <AppProviders repository={createAppRepository()}><App /></AppProviders>
  </StrictMode>,
)
