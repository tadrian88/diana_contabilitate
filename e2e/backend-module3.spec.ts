import { expect, test } from './fixtures/authenticated'

test('persisted missing-contract request survives reload and remains blocked', async ({ page }) => {
  await page.goto('/tasks')

  await expect(page.getByText('TASK-CM-OPEN')).toBeVisible()
  await expect(page.getByText('TASK-CL-OPEN')).toBeVisible()
  const openRow = page.locator('tr', { hasText: 'TASK-MC-OPEN' })
  await expect(openRow).toContainText('Contract lipsă')

  const requestResponse = page.waitForResponse((response) => response.url().includes('/contract-requests') && response.request().method() === 'POST')
  await openRow.getByRole('button', { name: 'Solicită contract' }).click()
  const response = await requestResponse
  expect(response.status()).toBe(200)
  expect(await response.json()).toMatchObject({
    id: 'inv-task-missing-open',
    pipelineStatus: 'AWAITING_CONTRACT',
    task: { id: 'task-missing-open', status: 'WAITING', contractRequested: true },
  })

  await expect(page.getByRole('status')).toContainText('Contractul a fost solicitat')
  await expect(openRow).toHaveCount(0)
  await page.getByRole('tab', { name: /În așteptare/ }).click()
  const waitingRow = page.locator('tr', { hasText: 'TASK-MC-OPEN' })
  await expect(waitingRow).toContainText('În așteptare')

  await page.reload()
  await expect(page.locator('tr', { hasText: 'TASK-MC-OPEN' })).toContainText('În așteptare')

  const openTasks = await page.request.get('/api/v1/validation-tasks?status=OPEN')
  expect(openTasks.status()).toBe(200)
  expect((await openTasks.json()).some((item: { task: { id: string } }) => item.task.id === 'task-missing-open')).toBe(false)

  await waitingRow.getByRole('link', { name: 'Vezi context' }).click()
  await expect(page.getByRole('heading', { name: 'TASK-MC-OPEN' })).toBeVisible()
  await expect(page.getByText('Contract solicitat', { exact: true })).toBeVisible()
  await expect(page.getByText('Așteaptă contract', { exact: true }).first()).toBeVisible()
})
