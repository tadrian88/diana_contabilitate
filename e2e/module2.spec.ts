import { expect, test } from '@playwright/test'

test('contabilul procesează coada și păstrează task-urile în așteptare', async ({ page }) => {
  await page.goto('/tasks')
  await expect(page.getByTestId('active-scope')).toHaveText('Toți clienții')

  const contractRow = page.locator('tbody tr').filter({ hasText: 'DEMO-MC-002' })
  await contractRow.getByRole('link', { name: 'Rezolvă' }).click()
  await page.getByRole('button', { name: 'Confirmă', exact: true }).click()
  await page.getByRole('link', { name: 'Înapoi la Task Inbox' }).click()
  await expect(page.getByText('DEMO-MC-002')).toHaveCount(0)

  const classificationRow = page.locator('tbody tr').filter({ hasText: 'DEMO-CL-004' })
  await classificationRow.getByRole('link', { name: 'Rezolvă' }).click()
  await page.getByRole('button', { name: 'Acceptă propunerea' }).first().click()
  await expect(page.getByRole('button', { name: 'Acceptă propunerea' })).toHaveCount(1)
  await page.getByRole('button', { name: 'Acceptă propunerea' }).click()
  await page.getByRole('link', { name: 'Înapoi la Task Inbox' }).click()
  await expect(page.getByText('DEMO-CL-004')).toHaveCount(0)

  const missingRow = page.locator('tbody tr').filter({ hasText: 'DEMO-NC-003' })
  await missingRow.getByRole('button', { name: 'Solicită contract' }).click()
  await expect(page.getByText('DEMO-NC-003')).toHaveCount(0)

  await page.getByRole('tab', { name: /În așteptare/ }).click()
  await expect(page.getByText('DEMO-NC-003')).toBeVisible()
  await expect(page.getByText('DEMO-WT-006')).toBeVisible()
})
