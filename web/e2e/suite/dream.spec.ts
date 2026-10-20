import { expect, test } from '@playwright/test'
import { adminAPI, signIn } from './helpers'

test.beforeEach(async ({ request }) => {
  const api = await adminAPI(request)
  expect((await api.put('/admin/connections', { hookToken: 'bd_hook_e2e' })).ok()).toBeTruthy()
  const fp = await (await api.post('/admin/integrations/bitvoice/fingerprint', { url: 'https://127.0.0.1:9644' })).json()
  await api.put('/admin/integrations', { bitvoiceUrl: 'https://127.0.0.1:9644', bitvoiceToken: 'bv_simulated', bitvoiceFingerprint: fp.fingerprint, eidoToken: 'bd_eido_e2e', radioSecret: 'bd_radio_e2e', aliListen: '' })
})

// talking: an emergency caller with a transcript (calls screened as non-emergency are kept in their own list).
const talking = (c: { transcript?: unknown[]; screen?: { nonEmergency?: boolean }; state?: string }) =>
  (c.transcript?.length ?? 0) > 1 && !c.screen?.nonEmergency && c.state !== 'ended'

test('a caller is transcribed, the details are pulled out, and crash data is rated', async ({ page, request }) => {
  const errors = await signIn(page)
  await expect(page.getByText('911 CALLS')).toBeVisible()
  // A vehicle's crash notification, through the hook.
  const r = await request.post('api/hooks/acn', { headers: { Authorization: 'Bearer bd_hook_e2e' },
    data: 'delta_v_mph=40\npdof=0\nrollover=1\nlatitude=37.2711\nlongitude=-79.9414\nvehicle_make=Ford\nfuel_type=electric\nnumber_of_occupants=2' })
  expect(r.ok()).toBeTruthy()
  await expect(page.getByText('CRASH DATA · HIGH').first()).toBeVisible()
  // The simulator's callers speak; one of them is transcribed within a few calls.
  const calls = page.getByRole('list', { name: '911 calls' })
  await expect(calls.getByText(/answered/i).first()).toBeVisible({ timeout: 30000 })
  await expect.poll(async () => {
    const st = await (await request.get('api/state')).json()
    return st.calls.some(talking)
  }, { timeout: 45000 }).toBeTruthy()
  const st = await (await request.get('api/state')).json()
  const c = st.calls.find(talking)
  await calls.getByText(c.callback.replace(/^\+1(\d{3})(\d{3})(\d{4})$/, '($1) $2-$3')).first().click()
  await expect(page.getByRole('region', { name: 'Transcript' }).first()).toBeVisible()
  expect(errors).toEqual([])
})

test('training: a board of its own, a scenario, a reply in Spanish and a score', async ({ browser }) => {
  const ctx = await browser.newContext({ ignoreHTTPSErrors: true })
  const page = await ctx.newPage()
  const errors: string[] = []
  page.on('pageerror', (e) => errors.push(String(e)))
  await page.goto('/')
  const { creds } = await import('./helpers')
  await page.fill('#u', 'admin')
  await page.fill('#p', creds().password)
  await page.getByLabel(/^Training/).check()
  await page.getByRole('button', { name: 'Sign in', exact: true }).click()
  await page.getByRole('button', { name: /Supervisor/ }).click()
  await expect(page.getByText('TRAINING', { exact: true })).toBeVisible()
  await page.getByRole('dialog').getByText('Cardiac arrest, Spanish-speaking caller').locator('xpath=../..').getByRole('button', { name: 'Start' }).click()
  const calls = page.getByRole('list', { name: '911 calls' })
  await expect(calls.getByText('Spanish')).toBeVisible({ timeout: 20000 })
  await calls.locator('[role=button]').first().click()
  await expect(page.getByText('not breathing').first()).toBeVisible()
  await page.getByRole('button', { name: 'Help is on the way' }).click()
  await expect(page.getByText('La ayuda va en camino').first()).toBeVisible()
  await page.getByRole('button', { name: 'Score' }).click()
  await expect(page.getByRole('dialog').getByText('Spoke to the caller in their language')).toBeVisible()
  await page.keyboard.press('Escape')
  await page.getByRole('button', { name: 'Reset' }).click()
  await expect(calls.getByText('Spanish')).toHaveCount(0, { timeout: 10000 })
  await ctx.close()
  expect(errors).toEqual([])
})

test('the supervisor sees the people, the quality review and a break', async ({ page }) => {
  const errors = await signIn(page)
  await expect(page.getByText('911 CALLS')).toBeVisible()
  await page.getByRole('button', { name: /break/i }).first().click()
  await expect(page.getByText('On break')).toBeVisible()
  await page.goto('/#/board')
  await expect(page.getByRole('region', { name: 'People' })).toBeVisible()
  await expect(page.getByRole('region', { name: 'People' }).getByText('on break')).toBeVisible()
  await page.goto('/#/admin/quality')
  await expect(page.getByRole('heading', { name: 'Quality review' })).toBeVisible()
  await page.goto('/')
  await page.getByRole('button', { name: /On break/ }).click()
  expect(errors).toEqual([])
})

test('working through an outage: the last board, an offline log, sent when back', async ({ page, context, request }) => {
  await signIn(page)
  await expect(page.getByText('911 CALLS')).toBeVisible()
  await page.waitForTimeout(11000) // the board is kept every ten seconds
  await context.setOffline(true)
  await expect(page.getByText('The server cannot be reached.')).toBeVisible({ timeout: 20000 })
  await page.getByRole('button', { name: /Offline log/ }).click()
  await page.getByRole('dialog').getByLabel('Address').fill('18 Kirk Ave SW')
  await page.getByRole('dialog').getByLabel('Narrative').fill('E2E offline entry')
  await page.getByRole('button', { name: 'Keep it' }).click()
  await page.keyboard.press('Escape')
  await context.setOffline(false)
  await expect(page.getByRole('button', { name: 'Send them now' })).toBeVisible({ timeout: 30000 })
  await page.getByRole('button', { name: 'Send them now' }).click()
  await expect.poll(async () => {
    const st = await (await request.get('api/state')).json()
    return st.incidents.some((i: { timeline: { text: string }[] }) => i.timeline.some((t) => t.text.includes('E2E offline entry (entered offline')))
  }, { timeout: 15000 }).toBeTruthy()
})
