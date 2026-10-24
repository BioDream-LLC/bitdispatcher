import { expect, test } from '@playwright/test'
import { adminAPI, signIn } from './helpers'

const SIM = 'https://127.0.0.1:9644'

test.beforeEach(async ({ request }) => {
  const api = await adminAPI(request)
  await api.put('/admin/connections', { hookToken: 'bd_hook_e2e' })
  const fp = await (await api.post('/admin/integrations/bitvoice/fingerprint', { url: SIM })).json()
  await api.put('/admin/integrations', { bitvoiceUrl: SIM, bitvoiceToken: 'bv_simulated', bitvoiceFingerprint: fp.fingerprint, eidoToken: 'bd_eido_e2e', radioSecret: 'bd_radio_e2e', aliListen: '' })
})

test('several open emergencies share one bar, the newest first', async ({ page, request }) => {
  const errors = await signIn(page)
  await expect(page.getByText('911 CALLS')).toBeVisible()
  for (const id of [811, 812, 813]) {
    await request.post(`${SIM}/sim/emergency?unit=${id}`)
    await page.waitForTimeout(400)
  }
  const bar = page.getByRole('alert').filter({ hasText: 'EMERGENCY' })
  await expect(bar.getByText(/2 more open/)).toBeVisible({ timeout: 15_000 })
  // One flashing row only, naming the newest radio.
  await expect(bar.locator('.flash')).toHaveCount(1)
  await expect(bar.locator('.flash')).toContainText('813')
  await bar.getByRole('button', { name: /2 more open/ }).click()
  const others = page.getByLabel('Other open emergencies')
  await expect(others.getByText('EMERGENCY')).toHaveCount(2)
  page.once('dialog', (d) => d.accept())
  await others.getByRole('button', { name: 'Acknowledge all 3' }).click()
  await expect(bar).toHaveCount(0, { timeout: 10_000 })

  expect(errors).toEqual([])
})
