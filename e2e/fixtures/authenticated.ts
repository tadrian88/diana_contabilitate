import { expect, request as playwrightRequest, test as base, type APIRequestContext } from '@playwright/test'

const email = 'demo.fixture@accountingtechco.test'
const password = 'Diana-E2E-Only-2026!'

type AuthenticatedWorkerFixtures = {
  authenticatedStorageState: Awaited<ReturnType<APIRequestContext['storageState']>>
}

export const test = base.extend<{}, AuthenticatedWorkerFixtures>({
  authenticatedStorageState: [async ({}, use, workerInfo) => {
    const baseURL = workerInfo.project.use.baseURL
    if (typeof baseURL !== 'string') throw new Error('Authenticated E2E tests require use.baseURL')

    const loginContext = await playwrightRequest.newContext({
      baseURL,
      extraHTTPHeaders: { Origin: baseURL },
    })
    const response = await loginContext.post('/api/v1/auth/login', {
      data: { email, password },
    })
    if (!response.ok()) {
      const body = await response.text()
      await loginContext.dispose()
      throw new Error(`E2E login failed (${response.status()}): ${body}`)
    }
    const storageState = await loginContext.storageState()
    await loginContext.dispose()
    await use(storageState)
  }, { scope: 'worker' }],
  storageState: async ({ authenticatedStorageState }, use) => {
    await use(authenticatedStorageState)
  },
  request: async ({ authenticatedStorageState, baseURL }, use) => {
    if (!baseURL) throw new Error('Authenticated E2E tests require use.baseURL')
    const requestContext = await playwrightRequest.newContext({
      baseURL,
      extraHTTPHeaders: { Origin: baseURL },
      storageState: authenticatedStorageState,
    })
    await use(requestContext)
    await requestContext.dispose()
  },
})

export { expect }
