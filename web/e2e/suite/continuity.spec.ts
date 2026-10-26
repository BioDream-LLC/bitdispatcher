import { expect, test } from '@playwright/test'
import { type Page } from '@playwright/test'
import { adminAPI, creds, signIn } from './helpers'
import type { APIRequestContext } from '@playwright/test'

/** Acknowledges, for the administrator in each duty, the messages this file's actions send, so the
 * suites after it start without banners over the status bar. */
async function ackAll(request: APIRequestContext) {
  for (const duty of ['calltaker', 'supervisor', 'dispatcher', 'police', 'fire', 'ems', 'admin']) {
    const r = await request.post('api/auth/login', { headers: { 'X-BitDispatcher': '1' }, data: { username: 'admin', password: creds().password, console: 'E2E', duty } })
    if (!r.ok()) continue
    const inbox = (await (await request.get('api/comms?folder=inbox')).json()) as { items: { id: string }[] }
    for (const c of inbox.items ?? []) await request.post(`api/comms/${c.id}/ack`, { headers: { 'X-BitDispatcher': '1' }, data: {} })
  }
}

/** Acknowledges the urgent and high messages this sends the supervisors. */
async function ackBanners(page: Page) {
  const b = page.getByRole('alert', { name: 'Urgent communication' }).getByRole('button', { name: 'Acknowledge' })
  for (let i = 0; i < 6 && (await b.first().isVisible().catch(() => false)); i++) {
    await b.first().click()
    await page.waitForTimeout(300)
  }
}

// Continuity in the browser: scheduled map updates set in Admin › Map; the relocation checklist set in
// Admin › Continuity, then the move to the backup site declared, shown on the console and ticked; the
// continuity pack; and overflow, with the test server as its own neighbour (the same server asks, accepts,
// takes the call and gets the incident), so the whole path runs through the browser on one server.

test('scheduled map updates are set in Admin › Map', async ({ page }) => {
  const errors = await signIn(page, 'admin', undefined, 'admin')
  await page.goto('/#/admin/map')
  const panel = page.getByRole('region', { name: 'Automatic updates' })
  await panel.getByLabel('Updates').selectOption('weekly')
  await panel.getByLabel('Day').selectOption('0')
  await panel.getByLabel('Starting at').selectOption('3')
  await panel.getByRole('button', { name: 'Save schedule' }).click()
  await expect(page.getByText('Map update schedule saved')).toBeVisible()
  await expect(panel.getByText(/Next run Sun/)).toBeVisible()
  expect(errors).toEqual([])
})

test('the move to the backup site: checklist, declared, shown everywhere, ended', async ({ page, request }) => {
  const errors = await signIn(page, 'admin', undefined, 'admin')
  page.on('dialog', (d) => d.accept())
  await page.goto('/#/admin/continuity')
  await page.locator('#ca-steps').fill('Call the 911 authority to reroute 911\nSign in at the backup consoles')
  await page.locator('#ca-contacts').fill('Backup site: 540-555-0199')
  await page.getByRole('button', { name: 'Save', exact: true }).click()
  await expect(page.getByText('Continuity saved')).toBeVisible()

  await page.goto('/#/continuity')
  await page.getByLabel('Why the centre is moving').fill('E2E: smoke in the building')
  await page.getByRole('button', { name: 'We are working from the backup site' }).click()
  await expect(page.getByText(/Working from the backup site/).first()).toBeVisible()
  await ackBanners(page)
  await page.getByLabel(/1\. Call the 911 authority/).click()
  await expect(page.getByText('Step done')).toBeVisible()
  await expect(page.getByLabel(/1\. Call the 911 authority/)).toBeChecked()
  // The console shows it in the status bar.
  await page.goto('/#/console')
  await expect(page.getByRole('link', { name: 'At backup site' })).toBeVisible()
  // The pack carries the checklist, the contacts and the declaration.
  const api = await adminAPI(request)
  const pack = await (await request.get('api/continuity/pack')).text()
  expect(pack).toContain('Call the 911 authority to reroute 911')
  expect(pack).toContain('540-555-0199')
  expect(pack).toContain('E2E: smoke in the building')
  await page.goto('/#/continuity')
  await page.getByRole('button', { name: 'We are back in our building' }).click()
  await expect(page.getByRole('button', { name: 'We are working from the backup site' })).toBeVisible()
  await api.put('/admin/settings', { ...(await api.get('/state')).settings, continuity: {} })
  await ackAll(request)
  expect(errors).toEqual([])
})

test('overflow: asked, accepted, a call taken becomes an incident, ended', async ({ page, request }) => {
  const api = await adminAPI(request)
  const sites = await api.get('/admin/sites')
  // The server is its own neighbour: it reaches itself at its own address, pinned to its own certificate.
  const r = await api.put('/admin/partners/SELF', { name: 'E2E Neighbour 911', url: 'https://127.0.0.1:9643/api/cad/eido/SELF', outToken: 'e2e-self-overflow', inToken: 'e2e-self-overflow', fingerprint: sites.fingerprint })
  expect(r.status()).toBe(204)
  const o = await api.put('/admin/partners/SELF/overflow', { ask: true, help: true, autoAccept: false, dispatch: false, callTypes: [] })
  expect(o.status(), await o.text()).toBe(204)

  const errors = await signIn(page, 'admin', undefined, 'supervisor')
  await page.goto('/#/overflow')
  await page.getByLabel('Why we ask E2E Neighbour 911').fill('E2E storm: 40 calls waiting')
  await page.getByRole('button', { name: 'Ask E2E Neighbour 911 to take our calls' }).click()
  await expect(page.getByText('Asked E2E Neighbour 911 to take our calls')).toBeVisible()
  // The request arrives (here, at the same server) for the supervisor to accept.
  await expect(page.getByText(/asks us to take their calls/)).toBeVisible()
  await ackBanners(page)
  await page.getByRole('button', { name: 'Accept' }).click()
  await expect(page.getByText(/is taking our calls until/)).toBeVisible()
  await ackBanners(page)
  // Every console shows both sides of it in the status bar.
  await page.goto('/#/console')
  await expect(page.getByRole('link', { name: /Taking calls for E2E Neighbour 911/ })).toBeVisible()
  await expect(page.getByRole('link', { name: /E2E Neighbour 911 is taking our calls/ })).toBeVisible()
  await page.getByRole('link', { name: /Taking calls for E2E Neighbour 911/ }).click()

  // A call taken for the neighbour becomes an incident in its CAD, with its number.
  const form = page.getByRole('region', { name: 'Take a call for E2E Neighbour 911' })
  await form.getByLabel('Location', { exact: true }).fill('77 E2E Overflow Lane, Salem')
  await form.getByLabel('Call type', { exact: true }).selectOption({ index: 0 })
  await form.getByLabel('Callback').fill('5405550177')
  await form.getByLabel('What happened').fill('Tree on a car, driver trapped')
  await form.getByRole('button', { name: /Send to E2E Neighbour 911/ }).click()
  await expect(page.getByText(/Sent: E2E Neighbour 911 incident/)).toBeVisible()
  const calls = page.getByRole('region', { name: 'Calls we took for neighbours' })
  await expect(calls.getByText('77 E2E Overflow Lane, Salem')).toBeVisible()
  const st = await api.get('/state')
  const inc = st.incidents.find((i: { location?: { address?: string } }) => i.location?.address === '77 E2E Overflow Lane, Salem')
  expect(inc).toBeTruthy()
  expect(JSON.stringify(inc.timeline)).toContain('Taken for us by E2E Neighbour 911 (overflow)')

  // Ending it: no more calls are taken.
  await page.getByRole('button', { name: 'End' }).first().click()
  await expect(page.getByText('Overflow ended')).toBeVisible()
  await expect(form).toHaveCount(0)
  await api.post('/overflow/SELF/end', { direction: 'in' })
  await api.post('/overflow/SELF/end', { direction: 'out' })
  await api.put('/admin/partners/SELF/overflow', { ask: false, help: false })
  await api.del('/admin/partners/SELF')
  await ackAll(request)
  expect(errors).toEqual([])
})
