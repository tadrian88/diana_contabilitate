import { render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import userEvent from '@testing-library/user-event'
import { HttpResponse, http } from 'msw'
import { MemoryRouter, Route, Routes, useNavigate } from 'react-router-dom'
import { server } from '../../test/msw/server'
import { AuthProvider, useAuth } from './AuthContext'
import { LoginPage } from './LoginPage'
import { ProtectedRoute } from './ProtectedRoute'

const endpoint = (path: string) => `http://localhost:3000/api/v1${path}`

function Dashboard() {
  const auth = useAuth()
  const navigate = useNavigate()
  return <><h1>Dashboard Diana</h1><span>{auth.user?.email}</span><button onClick={() => void auth.logout().then(() => navigate('/login'))}>Deconectare</button></>
}

function renderFlow(entry = '/') {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(<QueryClientProvider client={queryClient}><AuthProvider><MemoryRouter initialEntries={[entry]}><Routes><Route path="/login" element={<LoginPage />} /><Route element={<ProtectedRoute />}><Route path="/" element={<Dashboard />} /><Route path="/clients" element={<h1>Clienți protejați</h1>} /></Route></Routes></MemoryRouter></AuthProvider></QueryClientProvider>)
}

test('anonymous root and protected URL show login without flashing the application', async () => {
  server.use(http.get(endpoint('/auth/session'), () => HttpResponse.json({ code: 'UNAUTHENTICATED' }, { status: 401 })))
  renderFlow('/clients')
  expect(screen.queryByText('Clienți protejați')).not.toBeInTheDocument()
  expect(await screen.findByRole('heading', { name: 'Autentificare' })).toBeVisible()
})

test('invalid login stays on the generic Romanian error', async () => {
  server.use(
    http.get(endpoint('/auth/session'), () => HttpResponse.json({}, { status: 401 })),
    http.post(endpoint('/auth/login'), () => HttpResponse.json({ code: 'INVALID_CREDENTIALS' }, { status: 401 })),
  )
  renderFlow('/login')
  await screen.findByRole('heading', { name: 'Autentificare' })
  await userEvent.type(screen.getByLabelText('Email'), 'demo@accountingtechco.com')
  await userEvent.type(screen.getByLabelText('Parolă'), 'Wrong-Password-2026!')
  await userEvent.click(screen.getByRole('button', { name: 'Autentificare' }))
  expect(await screen.findByRole('alert')).toHaveTextContent('Email sau parolă incorectă.')
})

test('valid login enters dashboard and logout returns to login', async () => {
  server.use(
    http.get(endpoint('/auth/session'), () => HttpResponse.json({}, { status: 401 })),
    http.post(endpoint('/auth/login'), () => HttpResponse.json({ user: { id: 'user-1', email: 'demo@accountingtechco.com', persona: 'CONTABIL' } })),
    http.post(endpoint('/auth/logout'), () => new HttpResponse(null, { status: 204 })),
  )
  renderFlow('/login')
  await screen.findByRole('heading', { name: 'Autentificare' })
  await userEvent.type(screen.getByLabelText('Email'), 'demo@accountingtechco.com')
  await userEvent.type(screen.getByLabelText('Parolă'), 'Valid-Password-2026!')
  await userEvent.click(screen.getByRole('button', { name: 'Autentificare' }))
  expect(await screen.findByRole('heading', { name: 'Dashboard Diana' })).toBeVisible()
  expect(screen.getByText('demo@accountingtechco.com')).toBeVisible()
  await userEvent.click(screen.getByRole('button', { name: 'Deconectare' }))
  await waitFor(() => expect(screen.getByRole('heading', { name: 'Autentificare' })).toBeVisible())
})

test('refresh restores an authenticated session and a later 401 returns to login', async () => {
  server.use(http.get(endpoint('/auth/session'), () => HttpResponse.json({ user: { id: 'user-1', email: 'demo@accountingtechco.com', persona: 'CONTABIL' } })))
  renderFlow('/')
  expect(await screen.findByRole('heading', { name: 'Dashboard Diana' })).toBeVisible()
  window.dispatchEvent(new Event('diana:unauthorized'))
  expect(await screen.findByRole('heading', { name: 'Autentificare' })).toBeVisible()
})
