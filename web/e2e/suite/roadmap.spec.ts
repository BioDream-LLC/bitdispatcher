import { expect, test, type Page } from '@playwright/test'
import { adminAPI, signIn } from './helpers'

// The roadmap features in the browser: alarms, roll calls, crews, paging, tows, the directory, the schedule,
// warrants at the address, a unit's history, and the new administration pages.

const at = { lat: 37.2712, lon: -79.9418, address: '215 Church Ave SW' }

async function openIncident(page: Page, number: string) {
  await page.getByRole('list', { name: 'Open incidents' }).getByRole('button', { name: new RegExp(number) }).first().click()
  await expect(page.getByRole('complementary', { name: `Incident ${number}` })).toBeVisible()
  return page.getByRole('complementary', { name: `Incident ${number}` })
}

test('a structure fire: 2nd alarm, roll call with a crew missing, benchmarks', async ({ page, request }) => {
  const api = await adminAPI(request)
  const inc = await (await api.post('/incidents', { type: 'SF', location: at })).json()
  const errors = await signIn(page)
  const panel = await openIncident(page, inc.number)
  await panel.getByRole('button', { name: /Upgrade to 2nd alarm/ }).click()
  await expect(panel.getByRole('region', { name: 'Alarm level' })).toContainText('2nd alarm')
  await expect(panel.getByRole('region', { name: 'Timeline' }).getByText(/Upgraded to the 2nd alarm: adds/)).toBeVisible()
  // Two units arrive: the accountability clock starts by itself.
  for (const [u, s] of [['E1', 'dispatched'], ['E2', 'dispatched']]) expect((await api.post(`/units/${u}/status`, { status: s, incident: inc.number })).ok()).toBeTruthy()
  for (const u of ['E1', 'E2']) expect((await api.post(`/units/${u}/status`, { status: 'onscene' })).ok()).toBeTruthy()
  const acc = panel.getByRole('region', { name: 'Accountability' })
  await expect(acc).toContainText(/PAR in/)
  await acc.getByRole('button', { name: 'E2', exact: true }).click()
  await acc.getByRole('button', { name: 'Report E2 missing' }).click()
  await expect(acc.getByText('NOT ACCOUNTED FOR: E2')).toBeVisible()
  await acc.getByRole('button', { name: 'E2 found' }).click()
  await expect(acc.getByText('NOT ACCOUNTED FOR')).toHaveCount(0)
  await acc.getByRole('button', { name: /Fire under control/ }).click()
  await expect(panel.getByRole('region', { name: 'Timeline' }).getByText(/Benchmark: Fire under control/)).toBeVisible()
  expect(errors).toEqual([])
})

test('crews, paging, a tow and the call-out list on the incident', async ({ page, request }) => {
  const api = await adminAPI(request)
  expect((await api.put('/admin/members', { id: 'E2E-1', name: 'Pat Medic', quals: ['paramedic'], volunteer: true, email: 'pat@example.com' })).ok()).toBeTruthy()
  expect((await api.put('/admin/members', { id: 'E2E-2', name: 'Lee Driver', quals: ['emt', 'driver'], volunteer: true })).ok()).toBeTruthy()
  // A gateway that answers nothing: the page is recorded and its delivery fails, which the incident says.
  expect((await api.put('/admin/paging', { gateways: [{ id: 'hook', name: 'Test hook', kind: 'webhook', url: 'http://127.0.0.1:9/' }], publicUrl: '' })).ok()).toBeTruthy()
  expect((await api.put('/admin/pagegroups', { name: 'E2E volunteers', members: ['E2E-1', 'E2E-2'], gateways: ['hook'], off: true })).ok()).toBeTruthy()
  expect((await api.put('/admin/tows', { name: 'Acme Towing', phone: '540-555-0101', classes: ['light'] })).ok()).toBeTruthy()
  expect((await api.put('/directory', { name: 'Gas company emergency line', org: 'Roanoke Gas', category: 'utility', phones: [{ label: '24 h', number: '540-555-0177' }], types: ['MVA'] })).ok()).toBeTruthy()
  const inc = await (await api.post('/incidents', { type: 'MVA', location: at })).json()
  const errors = await signIn(page)
  const panel = await openIncident(page, inc.number)
  // The call-out list.
  await expect(panel.getByRole('region', { name: 'Call-out list' })).toContainText('540-555-0177')
  // A tow from the rotation.
  await panel.getByRole('region', { name: 'Tows' }).getByRole('button', { name: /Next on rotation/ }).click()
  await expect(page.getByText(/Call Acme Towing 540-555-0101/)).toBeVisible()
  await panel.getByRole('region', { name: 'Tows' }).getByRole('button', { name: 'Accepted' }).click()
  await expect(panel.getByRole('region', { name: 'Tows' }).getByText('accepted')).toBeVisible()
  // Page the volunteers.
  await panel.getByRole('region', { name: 'Responders paged' }).getByRole('button', { name: 'Page' }).click()
  const dlg = page.getByRole('dialog', { name: /Page responders/ })
  await dlg.getByRole('button', { name: /E2E volunteers/ }).click()
  await dlg.getByRole('button', { name: 'Page', exact: true }).click()
  await expect(panel.getByRole('region', { name: 'Responders paged' })).toContainText('2 paged')
  await expect(panel.getByRole('region', { name: 'Timeline' }).getByText(/deliveries failed/)).toBeVisible({ timeout: 15_000 })
  // A crew on M1 (with no incident selected, a unit opens its panel).
  await page.keyboard.press('Escape')
  await page.getByRole('button', { name: /^M1\b/ }).first().click()
  await page.getByRole('button', { name: 'Change the crew of M1' }).click()
  const crew = page.getByRole('dialog', { name: 'Crew of M1' })
  await crew.getByRole('button', { name: /Pat Medic/ }).click()
  await crew.getByRole('button', { name: 'Save' }).click()
  await expect(page.getByText(/Pat Medic/).first()).toBeVisible()
  expect(errors).toEqual([])
})

test('warrants at the address, the directory, the schedule and a unit history', async ({ page, request }) => {
  const api = await adminAPI(request)
  expect((await api.put('/papers', { kind: 'arrest', subject: { name: 'Jordan Q Example' }, location: at, caution: 'armed', charge: 'Failure to appear' })).ok()).toBeTruthy()
  const inc = await (await api.post('/incidents', { type: 'DOM', location: at })).json()
  const errors = await signIn(page)
  const panel = await openIncident(page, inc.number)
  await expect(panel.getByRole('alert').filter({ hasText: 'active paper at this address' })).toContainText('CAUTION: armed')
  await panel.getByRole('button', { name: 'Details' }).click()
  await expect(page.getByRole('dialog', { name: /Warrants and papers/ })).toContainText('Jordan Q Example')
  await page.keyboard.press('Escape')
  // The papers page.
  await page.getByRole('link', { name: 'Warrants and papers' }).click()
  await expect(page.getByRole('heading', { name: 'Warrants and papers to serve' })).toBeVisible()
  await page.getByText('Jordan Q Example').first().click()
  const pd = page.getByRole('dialog', { name: /Arrest warrant/ })
  await pd.getByLabel('Note').fill('no answer at the door')
  await pd.getByRole('button', { name: 'Record the attempt' }).click()
  await expect(pd.getByText(/no answer: no answer at the door/)).toBeVisible()
  await page.keyboard.press('Escape')
  await page.goto('/')
  await expect(page.getByText('911 CALLS')).toBeVisible()
  // The directory.
  expect((await api.put('/directory', { name: 'Medical examiner', phones: [{ number: '540-555-0170' }] })).ok()).toBeTruthy()
  await page.getByRole('button', { name: 'Contacts directory' }).click()
  const dir = page.getByRole('dialog', { name: 'Contacts directory' })
  await dir.getByLabel('Search the directory').fill('medical')
  await expect(dir.getByText('540-555-0170')).toBeVisible()
  await page.keyboard.press('Escape')
  // Schedule an event.
  await page.getByRole('button', { name: 'Scheduled calls and events' }).click()
  const sd = page.getByRole('dialog', { name: 'Scheduled calls and special events' })
  await sd.getByRole('button', { name: /Schedule a call or event/ }).click()
  await sd.getByRole('radio', { name: 'A special event' }).click()
  await sd.getByLabel('Title').fill('County fair')
  await sd.getByLabel('Where').fill('Berglund Center')
  await sd.getByRole('button', { name: 'Save' }).click()
  await expect(sd.getByText('County fair')).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(page.getByText('County fair').first()).toBeVisible()
  // A unit's day.
  await page.getByRole('button', { name: /^E1\b/ }).first().click()
  await page.getByRole('button', { name: /History/ }).click()
  const h = page.getByRole('dialog', { name: 'E1: status history' })
  await expect(h.getByText(/incidents/).first()).toBeVisible()
  await expect(h.locator('tbody tr').first()).toBeVisible()
  expect(errors).toEqual([])
})

test('the new administration pages open', async ({ page }) => {
  const errors = await signIn(page, undefined, undefined, 'admin')
  for (const [path, heading] of [['personnel', 'Personnel and crews'], ['paging', 'Paging groups'], ['tows', 'Tow rotation'], ['directory', 'Contacts directory and on-call lists'], ['geofences', /Areas on the map/]] as const) {
    await page.goto(`/#/admin/${path}`)
    await expect(page.getByRole('heading', { name: heading })).toBeVisible()
  }
  await page.goto('/#/admin/integrations')
  await expect(page.getByRole('heading', { name: 'Paging gateways' })).toBeVisible()
  await expect(page.getByRole('heading', { name: /NEMSIS/ })).toBeVisible()
  expect(errors).toEqual([])
})
