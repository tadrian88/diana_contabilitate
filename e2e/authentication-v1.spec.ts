import { expect, test } from '@playwright/test'

const email = 'demo.fixture@accountingtechco.test'
const password = 'Diana-E2E-Only-2026!'

test('Authentication V1: login, dashboard, client and logout', async ({ page }) => {
  await page.goto('/')
  await expect(page.getByRole('heading', { name: 'Autentificare' })).toBeVisible()
  await page.getByLabel('Email').fill(email)
  await page.getByLabel('Parolă').fill(password)
  await page.getByRole('button', { name: 'Autentificare' }).click()
  await expect(page).toHaveURL(/\/$/)
  await expect(page.getByText(email)).toBeVisible()
  await expect(page.getByText('Dashboard operațional')).toBeVisible()
  await page.getByRole('link', { name: 'Clienți' }).click()
  await expect(page).toHaveURL(/\/clients$/)
  await page.getByRole('button', { name: 'Deconectare' }).click()
  await expect(page.getByRole('heading', { name: 'Autentificare' })).toBeVisible()
  await page.goBack()
  await expect(page.getByRole('heading', { name: 'Autentificare' })).toBeVisible()
})
