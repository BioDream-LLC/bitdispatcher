import { expect, test } from '@playwright/test'
import { adminAPI, signIn } from './helpers'

test('a police car is marked out and status-checked from the police screen', async ({ page, request }) => {
  const api = await adminAPI(request)
  // Start from available, whatever an earlier test left.
  await api.post('/command', { line: 'AV 2A12', incident: '' })
  const errors = await signIn(page)
  await page.goto('/#/screen/police')
  const board = page.getByRole('region', { name: 'Police status and mark-outs' })
  await expect(board).toBeVisible()
  // The police screen shows police units only.
  await expect(board.getByRole('cell', { name: /^2A12/ })).toBeVisible()
  await expect(board.getByRole('cell', { name: /^E1\b/ })).toHaveCount(0)
  const row = board.getByRole('row', { name: /2A12/ })
  await row.getByRole('button', { name: 'MO' }).click()
  const dlg = page.getByRole('dialog', { name: /Mark out 2A12/ })
  await dlg.getByRole('button', { name: /Court/ }).click()
  await dlg.getByLabel('Note').fill('Courtroom 2')
  await dlg.getByRole('button', { name: 'Mark out', exact: true }).click()
  await expect(page.getByText(/2A12 marked out: Court/)).toBeVisible()
  await expect(row.getByText('Courtroom 2')).toBeVisible()
  await row.getByRole('button', { name: 'OK' }).click()
  await expect(page.getByText('2A12 status checked: all right')).toBeVisible()
  await expect(row.getByText(/ago/).first()).toBeVisible()
  // A command that fails stays on the line with its reason.
  await page.keyboard.press('Escape')
  await page.getByLabel('Command line').fill('MO 2A12 NAP')
  await page.getByLabel('Command line').press('Enter')
  await expect(page.getByRole('status').filter({ hasText: /no mark-out NAP/ })).toBeVisible()
  await expect(page.getByLabel('Command line')).toHaveValue('MO 2A12 NAP')
  await api.post('/command', { line: 'AV 2A12', incident: '' })
  expect(errors).toEqual([])
})

test('fire houses, screens and the supervisor board', async ({ page }) => {
  const errors = await signIn(page)
  await page.goto('/#/screen/stations')
  const houses = page.getByRole('region', { name: 'Fire houses' })
  await expect(houses.getByRole('article').first()).toBeVisible()
  await expect(houses.getByRole('article', { name: /Station 1 Downtown/ })).toBeVisible()
  await expect(houses.getByRole('article', { name: /Police headquarters/ })).toHaveCount(0) // the screen starts with fire and EMS
  // Screens: add one and see it listed.
  await page.goto('/')
  await page.getByRole('button', { name: /^Screens/ }).click()
  const dlg = page.getByRole('dialog', { name: /Screens/ })
  await dlg.getByRole('button', { name: /Police status and mark-outs/ }).click()
  await expect(dlg.getByRole('listitem').filter({ hasText: 'Police status and mark-outs' })).toBeVisible()
  await page.keyboard.press('Escape')
  await page.goto('/#/board')
  await expect(page.getByText('Supervisor board')).toBeVisible()
  await expect(page.getByText('Units available')).toBeVisible()
  await expect(page.getByText('Where the work is')).toBeVisible()
  await expect(page.locator('.maplibregl-canvas')).toBeVisible()
  expect(errors).toEqual([])
})

test('screens open on their own monitors, share the selection and are offered at the next sign-in', async ({ page, context }) => {
  // Two more monitors of the same model, as Chrome's Window Management API reports them.
  const cdp = await context.newCDPSession(page)
  const { targetInfo } = await cdp.send('Target.getTargetInfo')
  await cdp.send('Browser.grantPermissions', { permissions: ['windowManagement'], origin: new URL(test.info().project.use.baseURL!).origin, browserContextId: targetInfo.browserContextId })
  for (const left of [1920, 3840]) await cdp.send('Emulation.addScreen' as never, { left, top: 0, width: 1920, height: 1080, label: 'DELL U2422H', isInternal: false } as never)
  const errors = await signIn(page, undefined, undefined, 'D5')
  await page.getByRole('button', { name: /^Screens/ }).click()
  const dlg = page.getByRole('dialog', { name: /Screens/ })
  await expect(dlg.getByRole('img', { name: '3 monitors' })).toBeVisible()
  for (const t of ['Incidents and call entry', 'Police status and mark-outs']) await dlg.getByRole('button', { name: new RegExp('^Layout of ' + t) }).click()
  // Each goes to the next free monitor; same-named monitors are numbered.
  await expect(dlg.locator('select[aria-label^="Monitor for"]').first()).toHaveValue('1920,0')
  await expect(dlg.locator('select[aria-label^="Monitor for"]').nth(1)).toHaveValue('3840,0')
  const opened = context.waitForEvent('page')
  await dlg.getByRole('button', { name: /Open all/ }).click()
  await opened
  await expect.poll(() => context.pages().length).toBe(3)
  const [calls, police] = [context.pages().find((p) => p.url().includes('/screen/calls'))!, context.pages().find((p) => p.url().includes('/screen/police'))!]
  expect(await calls.evaluate(() => [screenX, outerWidth])).toEqual([1920, 1920])
  expect(await police.evaluate(() => [screenX, outerWidth])).toEqual([3840, 1920])
  await expect(dlg.locator('[aria-label="open"]')).toHaveCount(2)
  await page.keyboard.press('Escape')
  // Choosing an incident on one screen opens it on the others.
  const row = calls.getByRole('list', { name: 'Open incidents' }).getByRole('button').first()
  const num = (await row.textContent())!.match(/\d\d-\d{6}/)![0]
  await row.click()
  await expect(page.locator(`aside[aria-label="Incident ${num}"]`)).toBeVisible()
  await calls.close()
  await police.close()
  // The next sign-in at this position offers them.
  const again = await context.newPage()
  await again.goto('/')
  await expect(again.getByRole('status').filter({ hasText: 'Position D5 has 2 screens' })).toBeVisible()
  expect(errors).toEqual([])
})
