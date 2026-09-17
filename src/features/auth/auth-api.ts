import axios from 'axios'

export const apiClient = axios.create({ baseURL: '/api/v1', withCredentials: true })

function csrfToken() {
  const value = document.cookie.split('; ').find((item) => item.startsWith('diana_csrf='))?.split('=')[1]
  return value ? decodeURIComponent(value) : undefined
}

apiClient.interceptors.request.use((config) => {
  const method = config.method?.toUpperCase() ?? 'GET'
  if (!['GET', 'HEAD', 'OPTIONS'].includes(method)) {
    const token = csrfToken()
    if (token) config.headers.set('X-CSRF-Token', token)
  }
  return config
})

apiClient.interceptors.response.use(undefined, (error) => {
  if (error?.response?.status === 401 && !String(error?.config?.url ?? '').startsWith('/auth/login')) {
    window.dispatchEvent(new Event('diana:unauthorized'))
  }
  return Promise.reject(error)
})
