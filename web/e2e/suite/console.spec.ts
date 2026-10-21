import { expect, test } from '@playwright/test'
import { adminAPI, answerAsk, signIn } from './helpers'

test('the dispatcher screen shows every panel', async ({ page }) => {
  const errors = await signIn(page)
  await expect(page.getByText('911 CALLS')).toBeVisible()
  await expect(page.getByText('INCIDENTS', { exact: false }).first()).toBeVisible()
  await expect(page.getByText('UNITS', { exact: false }).first()).toBeVisible()
  await expect(page.getByText('SITUATION')).toBeVisible()
  await expect(page.getByText('Roanoke Valley Demo 911')).toBeVisible()
  expect(errors).toEqual([])
})

test('a call to a closed incident from the keyboard', async ({ page }) => {
  await signIn(page)
  await expect(page.getByText('UNITS', { exact: false }).first()).toBeVisible()
  // Typing anywhere starts a command.
  await page.keyboard.type('NEW CHEST 210 Church Ave SW')
  await page.keyboard.press('Enter')
  await expect(page.getByText(/Created 26-\d{6}/)).toBeVisible()
  const number = (await page.getByText(/Created 26-\d{6}/).textContent())!.match(/26-\d{6}/)![0]
  // The new incident is selected for the next command: its recommendation is already picked.
  await expect(page.getByRole('complementary', { name: `Incident ${number}` }).getByText('Recommended', { exact: true })).toBeVisible()
  await page.keyboard.press('Control+Enter')
  await answerAsk(page)
  await expect(page.getByText(/Dispatched/).first()).toBeVisible()
  await page.keyboard.type('CL Handled')
  await page.keyboard.press('Enter')
  await expect(page.getByText(new RegExp(`Closed ${number}|${number} closed`))).toBeVisible()
  // The supervisor corrects the closed record without reopening it.
  const panel = page.getByRole('complementary', { name: `Incident ${number}` })
  await panel.getByRole('button', { name: 'Correct the record' }).click()
  const dlg = page.getByRole('dialog', { name: `Correct ${number}` })
  await dlg.getByLabel('Disposition').selectOption('Transported')
  await dlg.getByLabel('Why').fill('the crew transported after all')
  await dlg.getByRole('button', { name: 'Correct', exact: true }).click()
  await expect(panel.getByText(/record corrected: disposition Handled → Transported/)).toBeVisible()
  await expect(panel.getByRole('button', { name: 'Correct the record' })).toBeVisible()
  // And replays it from the ledger.
  await panel.getByRole('button', { name: 'Replay' }).click()
  const rp = page.getByRole('dialog', { name: `Replay ${number}` })
  await expect(rp.getByRole('button', { name: /Play/ })).toBeVisible()
  await rp.getByLabel('Speed').selectOption('120')
  await rp.getByRole('button', { name: /Play/ }).click()
  await expect(rp.getByRole('list', { name: 'Timeline so far' }).getByText(/record corrected/)).toBeVisible({ timeout: 15000 })
})

test('911 calls from the BitVoice simulator reach the queue', async ({ page, request }) => {
  const api = await adminAPI(request)
  const fp = await (await api.post('/admin/integrations/bitvoice/fingerprint', { url: 'https://127.0.0.1:9644' })).json()
  const r = await api.put('/admin/integrations', { bitvoiceUrl: 'https://127.0.0.1:9644', bitvoiceToken: 'bv_simulated', bitvoiceFingerprint: fp.fingerprint, eidoToken: 'bd_eido_e2e', radioSecret: 'bd_radio_e2e', aliListen: '' })
  expect(r.ok(), await r.text()).toBeTruthy()
  await signIn(page)
  // Every simulated call is from a 540-555 number (only one in three carries a "(SIMULATED)" caller name).
  await expect(page.getByRole('list', { name: '911 calls' }).getByText(/\(540\) 555-\d{4}/).first()).toBeVisible({ timeout: 30_000 })
})

test('every administration screen opens without an error', async ({ page }) => {
  const errors = await signIn(page, undefined, undefined, 'admin')
  for (const p of ['units', 'stations', 'calltypes', 'map', 'gis', 'settings', 'integrations', 'users', 'workstations', 'plans', 'facilities', 'guidance', 'disclosure', 'sites', 'premises', 'migrate', 'advisory', 'reports', 'ledger']) {
    await page.goto(`/#/admin/${p}`)
    await expect(page.locator('h1').first()).toBeVisible()
  }
  await page.goto('/#/help?a=command')
  await expect(page.getByRole('heading', { name: 'The command line' })).toBeVisible()
  await page.goto('/#/rms')
  await expect(page.getByText('Dispatch').first()).toBeVisible()
  expect(errors).toEqual([])
})

test('a unit is sent by dragging it onto an incident, and changed by right-click', async ({ page, request }) => {
  const api = await adminAPI(request)
  const inc = await (await api.post('/incidents', { type: 'SICK', location: { address: '5 Drag St' } })).json()
  await signIn(page)
  const board = page.getByRole('region', { name: 'Unit status board' })
  const tile = board.getByRole('button', { name: /^M2/ })
  const row = page.getByRole('list', { name: 'Open incidents' }).getByRole('button', { name: new RegExp(inc.number) })
  const sent = async () => (await api.get('/state')).units.find((u: { id: string }) => u.id === 'M2').incident === inc.number
  // The list can reorder as calls arrive; measure the row again on each try.
  for (let i = 0; i < 3 && !(await sent()); i++) {
    await tile.hover()
    await page.mouse.down()
    const box = (await row.boundingBox())!
    await page.mouse.move(box.x + 40, box.y + 10, { steps: 12 })
    await page.mouse.up()
    await answerAsk(page)
    await page.waitForTimeout(500)
  }
  await expect.poll(async () => (await api.get('/state')).units.find((u: { id: string }) => u.id === 'M2').incident).toBe(inc.number)
  await board.getByRole('button', { name: /^E2/ }).click({ button: 'right' })
  await page.getByRole('menuitem', { name: /Out of service/ }).click()
  await expect.poll(async () => (await api.get('/state')).units.find((u: { id: string }) => u.id === 'E2').status).toBe('oos')
})

test('a public demonstration account: one button on the sign-in page', async ({ browser, request }) => {
  const api = await adminAPI(request)
  expect((await api.put('/admin/users/demo', { name: 'Demo dispatcher', role: 'dispatcher', password: 'try-the-e2e-demo-pw', disabled: false, demo: true })).ok()).toBeTruthy()
  const ctx = await browser.newContext()
  const page = await ctx.newPage()
  const errors: string[] = []
  page.on('pageerror', (e) => errors.push(String(e)))
  // The server does not answer the page's first question (as while it restarts): the button still comes.
  let refused = 0
  await page.route('**/api/auth/me', (route) => (refused++ < 2 ? route.abort('connectionrefused') : route.continue()))
  await page.goto('/')
  await expect(page.getByRole('button', { name: 'Sign in as the demo user' })).toBeVisible({ timeout: 15_000 })
  expect(refused).toBeGreaterThan(2)
  await page.getByRole('button', { name: 'Sign in as the demo user' }).click()
  await expect(page.getByText('911 CALLS')).toBeVisible()
  await expect(page.getByText(/Demo dispatcher · WEB-/)).toBeVisible()
  await ctx.close()
  // Taken away, the button goes.
  expect((await api.put('/admin/users/demo', { name: 'Demo dispatcher', role: 'dispatcher', password: '', disabled: false, demo: false })).ok()).toBeTruthy()
  const ctx2 = await browser.newContext()
  const p2 = await ctx2.newPage()
  await p2.goto('/')
  await expect(p2.getByLabel('Username')).toBeVisible()
  await expect(p2.getByRole('button', { name: 'Sign in as the demo user' })).toHaveCount(0)
  await ctx2.close()
  expect(errors).toEqual([])
})
