import { expect, test, type Page } from '@playwright/test'
import { adminAPI, signIn } from './helpers'

// The operational updates in the browser: working calls in tabs beside call entry, the messages and BOLO
// inbox with search, one-line F7 and F11 in the classic key profile, staffing, agencies and the new
// administration pages.

const addr = (page: Page) => page.locator('#addr-taker').or(page.getByLabel('Address', { exact: true })).first()

test('a call-taker keeps three working calls in tabs, each with its own information', async ({ page }) => {
  const errors = await signIn(page, 'admin', undefined, 'calltaker')
  page.on('dialog', (d) => (d.type() === 'prompt' ? d.accept('duplicate of another call') : d.accept()))
  const tabs = page.getByRole('navigation', { name: 'Working calls' })
  await expect(tabs.getByRole('tab')).toHaveCount(1)
  // Clean up any left from an earlier run: close all but one.
  await addr(page).fill('100 First St')
  await tabs.getByRole('button', { name: 'New working call' }).click()
  await expect(tabs.getByRole('tab')).toHaveCount(2)
  await addr(page).fill('200 Second St')
  await tabs.getByRole('button', { name: 'New working call' }).click()
  await addr(page).fill('300 Third St')
  await expect(tabs.getByRole('tab')).toHaveCount(3)
  // The agency's limit for a call-taker is three: no fourth.
  await expect(tabs.getByRole('button', { name: 'New working call' })).toBeDisabled()
  // Moving between them keeps each one's address.
  await tabs.getByRole('tab', { name: /^Working call 1/ }).click()
  await expect(addr(page)).toHaveValue('100 First St')
  await expect(tabs.getByRole('tab', { name: /^Working call 1 \(active\)/ })).toHaveAttribute('aria-selected', 'true')
  await tabs.getByRole('tab', { name: /^Working call 2/ }).click()
  await expect(addr(page)).toHaveValue('200 Second St')
  await tabs.getByRole('tab', { name: /^Working call 3/ }).click()
  await expect(addr(page)).toHaveValue('300 Third St')
  // After a reload (an interrupted session) they come back from the server.
  await page.reload()
  await expect(tabs.getByRole('tab')).toHaveCount(3)
  await tabs.getByRole('tab', { name: /^Working call 2/ }).click()
  await expect(addr(page)).toHaveValue('200 Second St')
  // Closing one with information in it asks, and asks why; the others stay as they were.
  await tabs.getByRole('tab', { name: /^Working call 2/ }).hover()
  await tabs.getByRole('button', { name: 'Close working call 2' }).click()
  await expect(tabs.getByRole('tab')).toHaveCount(2)
  await tabs.getByRole('tab', { name: /^Working call 2/ }).click()
  await expect(addr(page)).toHaveValue('300 Third St')
  for (const n of [2, 1]) {
    await tabs.getByRole('tab', { name: new RegExp(`^Working call ${n}`) }).hover()
    await tabs.getByRole('button', { name: `Close working call ${n}` }).click()
  }
  await expect(tabs.getByRole('tab')).toHaveCount(1)
  expect(errors).toEqual([])
})

test('a BOLO from the inbox, with its plate found by search', async ({ page }) => {
  const errors = await signIn(page)
  await page.getByRole('button', { name: /^Messages and BOLOs/ }).click()
  const dlg = page.getByRole('dialog', { name: 'Messages and BOLOs' })
  await dlg.getByRole('button', { name: 'Compose' }).click()
  await dlg.getByLabel('Kind').selectOption('bolo')
  await dlg.getByLabel('Subject').fill('E2E robbery suspect vehicle')
  await dlg.getByLabel('Message', { exact: true }).fill('Blue Ford Focus, plate E2E9876, two men, armed')
  await dlg.getByLabel('Plate', { exact: true }).fill('E2E9876')
  await dlg.getByRole('button', { name: 'Send', exact: true }).click()
  await dlg.getByRole('tab', { name: 'Active BOLOs' }).click()
  await expect(dlg.getByRole('option', { name: /E2E robbery suspect vehicle/ })).toBeVisible()
  await dlg.getByRole('option', { name: /E2E robbery suspect vehicle/ }).click()
  await expect(dlg.getByRole('region', { name: 'Message detail' })).toContainText('E2E9876')
  await page.keyboard.press('Escape')
  // Search finds it by plate and says why.
  await page.getByRole('button', { name: 'Search', exact: true }).click()
  const s = page.getByRole('dialog', { name: 'Search' })
  await s.getByLabel('Search for').fill('E2E9876')
  await s.getByLabel('Search for').press('Enter')
  const res = s.getByRole('region', { name: 'Vehicles results' }).or(s.locator('[aria-label="Vehicles results"]'))
  await expect(res.first()).toContainText('same plate in a BOLO')
  await expect(s.locator('[aria-label="Messages and BOLOs results"]')).toContainText('E2E robbery suspect vehicle')
  expect(errors).toEqual([])
})

test('the classic key profile: F7 makes a traffic stop from one line, F11 a status, a unit alone opens the form', async ({ page, request }) => {
  const api = await adminAPI(request)
  const set = (await api.get('/state')).settings
  expect((await api.put('/admin/units/E2ECAR', { type: 'patrol' })).ok()).toBeTruthy()
  await api.post('/command', { line: 'AV E2ECAR' })
  expect((await api.put('/admin/settings', { ...set, commands: { ...(set.commands ?? {}), keys: { supervisor: 'classic' } } })).ok()).toBeTruthy()
  try {
    const errors = await signIn(page)
    const line = page.getByLabel('Command line')
    await line.click()
    await line.fill('E2ECAR.8TH/STEWART..ABC1234')
    await page.keyboard.press('F7')
    await expect(page.getByRole('status').filter({ hasText: /TS: E2ECAR at 8TH & STEWART, plate ABC1234/ })).toBeVisible()
    await line.fill('E2ECAR.ER')
    await page.keyboard.press('F11')
    await expect(page.getByRole('status').filter({ hasText: /E2ECAR En route/i })).toBeVisible()
    await line.fill('E2ECAR')
    await page.keyboard.press('F11')
    const form = page.getByRole('dialog', { name: 'Unit status: E2ECAR' })
    await expect(form).toBeVisible()
    await form.getByRole('button', { name: /^AS/ }).click()
    await expect(form).toHaveCount(0)
    const st = await api.get('/state')
    expect(st.units.find((u: { id: string }) => u.id === 'E2ECAR').status).toBe('onscene')
    // The key map for this duty.
    await page.keyboard.press('Control+/')
    await expect(page.getByRole('dialog', { name: /Keys: Classic CAD/ })).toContainText('The 911 call')
    await page.keyboard.press('Escape')
    await api.post('/command', { line: 'AV E2ECAR' })
    expect(errors).toEqual([])
  } finally {
    await api.put('/admin/settings', set)
  }
})

test('staffing, the working agency, and the new administration pages', async ({ page, request }) => {
  const api = await adminAPI(request)
  expect((await api.put('/admin/agencies/E2EPD', { name: 'E2E City Police', disciplines: ['police'] })).ok()).toBeTruthy()
  expect((await api.put('/admin/members', { id: 'E2E-501', name: 'Pat Example' })).ok()).toBeTruthy()
  const errors = await signIn(page)
  await page.getByRole('link', { name: /^Staffing/ }).click()
  await expect(page.getByRole('heading', { name: 'Staffing' })).toBeVisible()
  const roster = page.getByRole('region', { name: 'On duty and roster' }).or(page.locator('[aria-label="On duty and roster"]')).first()
  await roster.getByRole('checkbox', { name: 'Show everyone' }).check()
  const row = roster.getByRole('row', { name: /Pat Example/ })
  await row.getByRole('button', { name: 'Sign on' }).click()
  const on = page.getByRole('dialog', { name: 'Sign on Pat Example' })
  await on.getByRole('button', { name: /^Sign on/ }).last().click()
  await expect(on).toHaveCount(0)
  await expect(roster.getByRole('row', { name: /Pat Example/ }).getByRole('button', { name: 'Sign off' })).toBeVisible()
  await roster.getByRole('row', { name: /Pat Example/ }).getByRole('button', { name: 'Sign off' }).click()
  const off = page.getByRole('dialog', { name: 'Sign off Pat Example' })
  await off.getByRole('button', { name: /^Sign off/ }).last().click()
  await expect(off).toHaveCount(0)
  for (const [path, heading] of [['agencies', 'Agencies'], ['operations', 'Operations rules'], ['qualifications', 'Qualifications and teams']]) {
    await page.goto('/#/admin/' + path)
    await expect(page.getByRole('heading', { name: heading, exact: true }).first()).toBeVisible()
  }
  await expect(page.getByText('E2E City Police').first()).toBeVisible({ timeout: 5000 }).catch(async () => {
    await page.goto('/#/admin/agencies')
    await expect(page.getByText('E2E City Police').first()).toBeVisible()
  })
  expect((await api.del('/admin/agencies/E2EPD')).ok()).toBeTruthy()
  expect(errors).toEqual([])
})
