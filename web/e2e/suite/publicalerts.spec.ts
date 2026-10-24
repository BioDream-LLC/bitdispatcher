import { expect, test } from '@playwright/test'
import { adminAPI, signIn } from './helpers'

// Public alerts in the browser: a supervisor alerts the public around an incident, checks the CAP
// message, sends it, sees it in force and on the public feed, and cancels it; the settings page.

const at = { lat: 37.2712, lon: -79.9418, address: '215 Church Ave SW' }

test('a supervisor alerts the public around an incident, then cancels it', async ({ page, request }) => {
  const api = await adminAPI(request)
  expect((await api.put('/admin/paging', { gateways: [{ id: 'hook', name: 'Test hook', kind: 'webhook', url: 'http://127.0.0.1:9/' }], publicUrl: '' })).ok()).toBeTruthy()
  expect((await api.put('/admin/alerting', { sender: 'alerts@example.gov', senderName: 'E2E County ECC', jurisdiction: 'E2E County', geocodes: ['051770'],
    feed: true, twoPerson: false, timeZone: 'America/New_York', receivers: [] })).ok()).toBeTruthy()
  expect((await api.put('/admin/alertlists', { id: '', name: 'E2E residents', gateways: ['hook'], people: [{ name: 'Resident', email: 'resident@example.org', lat: at.lat, lon: at.lon }] })).ok()).toBeTruthy()
  const inc = await (await api.post('/incidents', { type: 'HAZ', location: at })).json()
  const errors = await signIn(page)
  page.on('dialog', (d) => d.accept())
  await page.getByRole('list', { name: 'Open incidents' }).getByRole('button', { name: new RegExp(inc.number) }).first().click()
  const panel = page.getByRole('complementary', { name: `Incident ${inc.number}` })
  await panel.getByRole('region', { name: 'Public alerts for this incident' }).getByRole('button', { name: 'Alert the public' }).click()
  const dlg = page.getByRole('dialog', { name: 'Public alerts' })
  await expect(dlg.getByLabel('Kind of alert')).toHaveValue('HMW')
  await dlg.getByLabel('Kind of alert').selectOption('SPW')
  await expect(dlg.getByLabel('Headline')).toHaveValue('Shelter in place')
  await dlg.getByLabel('Headline').fill('Shelter in place near Church Ave')
  await dlg.getByLabel('What is happening').fill('A chlorine leak at 215 Church Ave SW.')
  await expect(dlg.getByRole('radio', { name: 'Around the incident' })).toHaveAttribute('aria-checked', 'true')
  // Too long for a phone: the check says so and Send stays off.
  await dlg.getByLabel('Phone alert (Wireless Emergency Alerts)').fill('x'.repeat(95))
  await dlg.getByRole('button', { name: 'Check' }).click()
  await expect(dlg.getByText(/limit is 90/)).toBeVisible()
  await expect(dlg.getByRole('button', { name: 'Send', exact: true })).toBeDisabled()
  await dlg.getByLabel('Phone alert (Wireless Emergency Alerts)').fill('Chlorine leak on Church Ave: go inside, close windows, turn off fans.')
  await dlg.getByRole('button', { name: 'Check' }).click()
  await expect(dlg.getByText(/Ready: a valid CAP message for the IPAWS profile. It goes to 1 person/)).toBeVisible()
  await dlg.getByRole('button', { name: 'Send', exact: true }).click()
  const confirm = page.getByRole('dialog', { name: 'Send a real public alert?' })
  await expect(confirm).toContainText('Chlorine leak on Church Ave')
  await confirm.getByRole('button', { name: 'Send to the public' }).click()
  await expect(page.getByText(/is going out/)).toBeVisible()
  // The dialog now lists it, sent.
  await expect(dlg.getByRole('row', { name: /Shelter in place near Church Ave/ })).toContainText('issued')
  await dlg.getByRole('button', { name: 'Close', exact: true }).last().click()
  // In force: on the status bar, the incident, and the public feed.
  await expect(page.getByRole('button', { name: /Public alerts: 1 in force/ })).toBeVisible()
  await expect(panel.getByRole('region', { name: 'Public alerts for this incident' })).toContainText('Shelter in place warning')
  await expect(panel.getByRole('region', { name: 'Timeline' }).getByText(/Public alert AL\d+ sent \(Actual Alert\)/)).toBeVisible()
  const feed = await (await request.get('api/public/alerts.atom')).text()
  expect(feed).toContain('Shelter in place near Church Ave')
  const ipaws = await (await request.get('api/public/alerts.xml')).text()
  expect(ipaws).toContain('<value>SPW</value>')
  expect(ipaws).toContain('<value>051770</value>')
  // Cancel it from the list: the all-clear replaces it.
  await page.getByRole('button', { name: /Public alerts: 1 in force/ }).click()
  const list = page.getByRole('dialog', { name: 'Public alerts' })
  await expect(list.getByRole('row', { name: /Shelter in place near Church Ave/ })).toContainText('issued')
  await list.getByRole('row', { name: /Shelter in place near Church Ave/ }).getByRole('button', { name: 'Cancel' }).click()
  await expect(list.getByRole('row', { name: /CANCELLED: Shelter in place near Church Ave/ })).toBeVisible()
  await expect(list.getByRole('row').filter({ hasText: /cancelled\s*Actual\s*Shelter in place near Church Ave/ })).toHaveCount(1)
  await expect.poll(async () => (await request.get('api/public/alerts.atom')).text()).toContain('CANCELLED: Shelter in place near Church Ave')
  await list.getByRole('button', { name: 'Close', exact: true }).last().click()
  expect(errors).toEqual([])
})

test('the public alerts settings page', async ({ page, request }) => {
  const api = await adminAPI(request)
  expect((await api.put('/admin/alerting', { sender: 'alerts@example.gov', senderName: 'E2E County ECC', jurisdiction: 'E2E County', geocodes: ['051770'],
    feed: true, twoPerson: true, timeZone: 'America/New_York', receivers: [] })).ok()).toBeTruthy()
  const errors = await signIn(page, 'admin', undefined, 'admin')
  await page.goto('/#/admin/publicalerts')
  await expect(page.getByRole('heading', { name: 'Public alerts' }).first()).toBeVisible()
  await expect(page.getByLabel(/^Sender/).first()).toHaveValue('alerts@example.gov')
  await expect(page.getByRole('textbox', { name: 'Public feed (Atom)' })).toHaveValue(/\/api\/public\/alerts\.atom$/)
  await expect(page.getByRole('textbox', { name: 'IPAWS-style feed' })).toHaveValue(/\/api\/public\/alerts\.xml$/)
  expect(errors).toEqual([])
})
