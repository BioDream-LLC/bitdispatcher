import { expect, test, type Page } from '@playwright/test'
import { adminAPI, creds, signIn } from './helpers'

// The final round in the browser: a forced location with its place, sent at once and alerting the
// supervisor, who reviews it; a flag shown before the call is sent; an officer-safety concern that must be
// explained; a vehicle run through the simulator with its possible hit confirmed; the request queue and the
// sign-off check; a critical incident's full review; Street View that says why it is not shown; the key test.

const addr = (page: Page) => page.locator('#addr-taker')

/** Acknowledges any urgent or high communication banners (the supervisor alerts this round sends). */
async function ackBanners(page: Page) {
  const b = page.getByRole('alert', { name: 'Urgent communication' }).getByRole('button', { name: 'Acknowledge' })
  for (let i = 0; i < 6 && (await b.first().isVisible().catch(() => false)); i++) {
    await b.first().click()
    await page.waitForTimeout(300)
  }
}

test('a forced location is sent at once, labelled, and reviewed by the supervisor', async ({ page, request }) => {
  const errors = await signIn(page, 'admin', undefined, 'calltaker')
  page.on('dialog', (d) => d.accept())
  await expect(page.getByRole('tab', { name: /Call Entry/ })).toBeVisible()
  // Location first: typed as the caller gave it, forced for this incident with a reason and a map point.
  await addr(page).fill('9 E2E New Construction Way')
  await page.getByLabel('Latitude').fill('37.2712')
  await page.getByLabel('Longitude').fill('-79.9411')
  await page.locator('#pv-taker').selectOption('forced')
  await page.locator('#bl-taker').fill('Lot 12')
  await page.locator('#fl-taker').fill('2')
  await page.locator('#un-taker').fill('204')
  // Sending without the reason puts the cursor on it.
  await page.locator('#ct-taker').fill('FALL')
  await page.locator('#ct-taker').press('Enter')
  await page.keyboard.press('F4')
  await expect(page.locator('#fr-taker')).toBeFocused()
  await page.locator('#fr-taker').selectOption({ index: 1 })
  // A comment written before sending goes with the call: one Enter adds it.
  const narrative = page.getByLabel('Add to the narrative')
  await narrative.fill('Caller says the house is new, no number on it yet')
  await narrative.press('Enter')
  await page.keyboard.press('F4')
  // A possible duplicate nearby is offered first: not the same, so send again.
  const dup = page.getByText('Is it one of these?')
  await expect(dup.or(page.getByRole('button', { name: 'Close call' })).first()).toBeVisible({ timeout: 10000 })
  if (await dup.isVisible()) await page.keyboard.press('F4')
  await expect(page.getByText(/FORCED LOCATION/).first()).toBeVisible()
  await expect(page.getByRole('button', { name: 'Close call' })).toBeVisible()
  // F5 opens another call-entry screen and keeps the sent one as it is.
  const tabs = page.getByRole('navigation', { name: 'Working calls' })
  await page.keyboard.press('F5')
  await expect(tabs.getByRole('tab')).toHaveCount(2)
  await tabs.getByRole('tab', { name: /^Working call 1/ }).click()
  await expect(page.getByText(/FORCED LOCATION/).first()).toBeVisible()
  // The supervisor's queue has it, and it stays open until both questions and the follow-up are answered.
  const api = await adminAPI(request)
  const ex = await api.get('/exceptions')
  const mine = ex.items.find((x: { entered: string }) => x.entered === '9 E2E New Construction Way')
  expect(mine?.kind).toBe('forced')
  expect(mine?.open).toBe(true)
  await page.goto('/#/supervisor')
  await expect(page.getByRole('tab', { name: /Location exceptions/ })).toBeVisible()
  await expect(page.getByText('9 E2E New Construction Way').first()).toBeVisible()
  expect(errors).toEqual([])
})

test('a flag on a verified address shows before the call is sent', async ({ page, request }) => {
  const api = await adminAPI(request)
  const r = await api.put('/flags', { category: 'hazard', title: 'E2E oxygen in use', warning: 'Home oxygen in the back bedroom', instruction: 'No open flame; ventilate',
    scope: { address: '1 E2E FLAG RD', lat: 37.2701, lon: -79.9401, verified: true }, conflictOk: true })
  expect(r.ok()).toBeTruthy()
  const errors = await signIn(page, 'admin', undefined, 'calltaker')
  await addr(page).fill('1 E2E Flag Rd')
  await page.getByLabel('Latitude').fill('37.2701')
  await page.getByLabel('Longitude').fill('-79.9401')
  await page.locator('#pv-taker').selectOption('verified')
  // Shown at once, on every tab (the header), before the call is sent.
  await expect(page.getByText(/HIGH hazard: E2E oxygen in use/).first()).toBeVisible({ timeout: 10000 })
  await page.getByRole('tab', { name: /Hazards/ }).click()
  await expect(page.getByText('Responders: No open flame; ventilate').first()).toBeVisible()
  expect(errors).toEqual([])
})

test('an officer-safety concern must say what and why; a person is found by first name alone', async ({ page }) => {
  const errors = await signIn(page, 'admin', undefined, 'calltaker')
  await page.goto('/#/taker')
  await ackBanners(page)
  await page.getByRole('tab', { name: /Persons/ }).click()
  await page.getByRole('button', { name: /Add (a )?person/ }).first().click()
  const first = page.getByLabel('First name').first()
  await first.fill('Zebulon')
  await page.getByLabel('Last name').first().fill('Testperson')
  await page.getByLabel(/Officer-safety concern/).first().check()
  await page.getByRole('button', { name: 'Save', exact: true }).first().click()
  // Not saved without the explanation: the cursor goes to the alert title.
  await expect(page.locator('#pf-safety-title')).toBeFocused()
  await page.locator('#pf-safety-title').fill('Threatened to fight officers')
  await page.locator('#pf-safety-reason').fill('Caller says he said he would fight the police if they came')
  await page.locator('#pf-safety-source').fill('caller')
  await page.locator('#pf-safety-instruction').fill('Wait for a second unit')
  await page.getByRole('button', { name: 'Save', exact: true }).first().click()
  await expect(page.getByText(/OFFICER SAFETY: Threatened to fight officers/).first()).toBeVisible()
  expect(errors).toEqual([])
})

test('a vehicle run through the simulator: a possible hit, confirmed only by the hit confirmation', async ({ page, request }) => {
  const api = await adminAPI(request)
  const st = (await api.get('/state')).settings
  if (st.ncicMode !== 'sim') expect((await api.put('/admin/settings', { ...st, ncicMode: 'sim' })).ok()).toBeTruthy()
  const inc = await (await api.post('/incidents', { type: 'FALL', location: { address: '1 E2E Plate St', lat: 37.27, lon: -79.94 } })).json()
  const v = await (await api.post(`/incidents/${inc.number}/vehicles`, { role: 'suspect', plate: 'TEST777', color: 'BLU' })).json()
  const run = await (await api.post(`/incidents/${inc.number}/records/${v.id}/run`, { requester: 'AD6' })).json()
  expect(run.hit).toBe('stolen')
  expect(run.simulated).toBe(true)
  const hc = await (await api.post(`/incidents/${inc.number}/records/${v.id}/hit-confirmation`, { returnId: run.return, requester: 'AD6' })).json()
  expect(hc.id).toBeTruthy()
  const errors = await signIn(page)
  await page.goto('/#/requests')
  await expect(page.getByText(hc.id).first()).toBeVisible()
  // Signing off with open requests is refused, with the list.
  await page.goto('/')
  await ackBanners(page)
  await page.getByRole('button', { name: 'Sign out' }).click()
  await expect(page.getByRole('dialog', { name: /Finish or hand over/ })).toBeVisible()
  expect(errors).toEqual([])
})

test('a critical incident: the supervisor opens the full review with its QA matrix', async ({ page, request }) => {
  const api = await adminAPI(request)
  const inc = await (await api.post('/incidents', { type: 'FALL', location: { address: '2 E2E Critical Ave', lat: 37.27, lon: -79.94 } })).json()
  const c = await (await api.post('/critical/flag', { incident: inc.number, on: true, reason: 'E2E: officer injured' })).json()
  expect(c.id).toBeTruthy()
  const errors = await signIn(page)
  await page.goto('/#/supervisor')
  await page.getByRole('tab', { name: /Critical/ }).click()
  await expect(page.getByText(inc.number).first()).toBeVisible()
  await page.getByRole('button', { name: /Open full review/ }).first().click()
  await expect(page.getByText(/QA matrix/i).first()).toBeVisible()
  await expect(page.getByText(/Source inventory/i).first()).toBeVisible()
  expect(errors).toEqual([])
})

test('Street View says why it is not shown and offers the way back to the call', async ({ page }) => {
  const errors = await signIn(page, 'admin', undefined, 'calltaker')
  await addr(page).fill('3 E2E View St')
  await page.getByLabel('Latitude').fill('37.2712')
  await page.getByLabel('Longitude').fill('-79.9411')
  await ackBanners(page)
  await page.getByRole('tab', { name: /Hazards/ }).click()
  await expect(page.getByText(/Street View could not be shown|not available here/).first()).toBeVisible({ timeout: 15000 })
  await page.getByRole('button', { name: 'Return to call' }).first().click()
  await expect(addr(page)).toHaveValue('3 E2E View St')
  expect(errors).toEqual([])
})

test('the key test records which keys reach bitdispatcher', async ({ page }) => {
  const errors = await signIn(page, 'admin', undefined, 'calltaker')
  await ackBanners(page)
  // Offered in the key map (Ctrl+/); started here directly.
  await expect(page.getByRole('banner', { name: 'Status' })).toBeVisible()
  await page.waitForTimeout(500)
  await page.evaluate(() => window.dispatchEvent(new Event('bd:keytest')))
  const dlg = page.getByRole('dialog', { name: 'Test the keys on this workstation' })
  await expect(dlg).toBeVisible()
  // Press whatever key it asks for, a few times; then stop.
  for (let i = 0; i < 3; i++) {
    const k = (await dlg.locator('.kbd').first().textContent())?.trim() ?? 'F2'
    await page.keyboard.press(k.replace('Shift+', 'Shift+'))
  }
  await dlg.getByRole('button', { name: /Stop and keep/ }).click()
  await expect(dlg).toBeHidden()
  expect(errors).toEqual([])
})

// The supervisor alerts this file raised are acknowledged, so the later files start with a quiet console.
test.afterAll(async ({ request }) => {
  const r = await request.post('api/auth/login', { headers: { 'X-BitDispatcher': '1' }, data: { username: 'admin', password: creds().password, console: 'E2E', duty: 'supervisor' } })
  if (!r.ok()) return
  const box = await (await request.get('api/comms?folder=inbox')).json()
  for (const c of box.items ?? []) {
    if (c.needsAck || c.ackNeeded || !c.acked) await request.post(`api/comms/${c.id}/ack`, { headers: { 'X-BitDispatcher': '1' }, data: {} })
  }
})
