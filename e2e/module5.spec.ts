import { expect, test } from '@playwright/test'

function kpi(page: import('@playwright/test').Page, label: string) {
  return page.locator(`[data-kpi="${label}"]`)
}

test('începe ziua din Dashboard și rezolvă o excepție de contract', async ({ page }) => {
  await page.goto('/')
  await expect(kpi(page, 'Necesită atenție')).toContainText('4')
  await page.getByText('DEMO-MC-002').click()
  await expect(page.getByText('Contract recomandat')).toBeVisible()
  await page.getByRole('button', { name: 'Confirmă', exact: true }).click()
  await page.getByRole('link', { name: 'Înapoi la Dashboard' }).click()
  await expect(kpi(page, 'Necesită atenție')).toContainText('3')
  await expect(page.getByText('DEMO-MC-002')).toHaveCount(0)
})

test('scopează toate valorile Dashboard pentru Client Demo Alfa', async ({ page }) => {
  await page.goto('/')
  await expect(page.getByRole('heading', { name: 'Context pe clienți' })).toBeVisible()
  await page.getByRole('button', { name: 'Client Demo Alfa SRL' }).click()
  await expect(page.getByTestId('active-scope')).toHaveText('Client Demo Alfa SRL')
  await expect(kpi(page, 'Facturi în procesare')).toContainText('4')
  await expect(kpi(page, 'Necesită atenție')).toContainText('1')
  await expect(kpi(page, 'Pregătite pentru SAGA')).toContainText('0')
  await expect(kpi(page, 'Exportate în SAGA')).toContainText('1')
  await expect(page.getByText('DEMO-CL-004')).toHaveCount(0)
})

test('reflectă progresia automată după rezolvarea clasificării', async ({ page }) => {
  await page.goto('/')
  await page.getByText('DEMO-CL-004').click()
  await page.getByRole('button', { name: 'Acceptă propunerea' }).first().click()
  await expect(page.getByRole('button', { name: 'Acceptă propunerea' })).toHaveCount(1)
  await page.getByRole('button', { name: 'Acceptă propunerea' }).click()
  await page.getByRole('link', { name: 'Înapoi la Dashboard' }).click()
  await expect(kpi(page, 'Necesită atenție')).toContainText('3')
  await expect(kpi(page, 'Exportate în SAGA')).toContainText('3')
})

test('expune exportul SAGA eșuat fără task sau retry inventat', async ({ page }) => {
  await page.goto('/')
  await expect(page.getByText('DEMO-SF-010')).toBeVisible()
  await page.getByText('DEMO-SF-010').click()
  await expect(page.getByText('Exportul SAGA simulat a eșuat. Nu există comportament de reîncercare aprobat.')).toBeVisible()
  await expect(page.getByText('Export eșuat', { exact: true }).first()).toBeVisible()
  await expect(page.getByRole('button', { name: /retry|reîncearcă|exportă/i })).toHaveCount(0)
  await expect(page.getByText('Fără intervenție')).toBeVisible()
})
