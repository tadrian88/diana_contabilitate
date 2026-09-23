import { expect, type Page } from '@playwright/test'

// Legacy demo contracts have no confirmed commercial snapshot. The accountant
// must record an explicit exception before their invoices can continue.
export async function approveDemoCommercialExceptions(page: Page) {
  await expect(page.getByRole('region', { name: 'Așteaptă review comercial' })).toBeVisible({ timeout: 15_000 })
  await page.getByRole('tab', { name: 'Contract' }).click()
  const reasons = page.getByRole('textbox', { name: /^Motiv excepție / })
  await expect(reasons.first()).toBeVisible()
  for (let remaining = await reasons.count(); remaining > 0; remaining--) {
    await expect(reasons.first()).toBeVisible()
    await reasons.first().fill('Excepție aprobată pentru contractul demonstrativ fără snapshot comercial confirmat.')
    const response = page.waitForResponse((item) => item.request().method() === 'POST' && item.url().includes('/commercial-validation/resolve'))
    await page.getByRole('button', { name: 'Acceptă excepția' }).first().click()
    expect((await response).status()).toBe(200)
    await expect(reasons).toHaveCount(remaining - 1)
  }
  await expect(page.getByRole('region', { name: 'Așteaptă review comercial' })).toHaveCount(0)
}
