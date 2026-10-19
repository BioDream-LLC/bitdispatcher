import { expect, test } from '@playwright/test'
import { adminAPI, signIn } from './helpers'

test('a field crew works an incident from the mobile terminal', async ({ page, request }) => {
  const api = await adminAPI(request)
  expect((await api.put('/admin/users/e3crew', { name: 'Engine 3 crew', role: 'field', password: 'engine-three-crew-e2e', disabled: false })).ok()).toBeTruthy()
  const inc = await (await api.post('/incidents', { type: 'FALL', location: { address: '1 Test St' } })).json()
  const r = await (await api.post('/command', { line: `D E3 #${inc.number}`, incident: '' })).json()
  expect(r.ok).toBeTruthy()
  await signIn(page, 'e3crew', 'engine-three-crew-e2e', 'E3')
  await expect(page.getByText('HOLD FOR EMERGENCY')).toBeVisible()
  await expect(page.getByText('Fall').first()).toBeVisible()
  await page.getByRole('button', { name: /En route/ }).click()
  await expect.poll(async () => (await api.get('/state')).units.find((u: { id: string }) => u.id === 'E3').status).toBe('enroute')
  await page.getByLabel('Note to dispatch').fill('Two minutes out')
  await page.getByRole('button', { name: 'Send' }).click()
  await expect(page.getByText('E3: Two minutes out')).toBeVisible()
})

test('a report is checked before it can be submitted', async ({ page }) => {
  await signIn(page)
  await page.goto('/#/rms')
  await page.getByRole('button', { name: 'New' }).click()
  await expect(page.getByText('BEFORE IT IS SUBMITTED')).toBeVisible()
  await expect(page.getByText('add at least one offence')).toBeVisible()
  await expect(page.getByRole('button', { name: /Submit for approval/ })).toBeDisabled()
})

test('the advisory panel suggests, and accepting changes the incident', async ({ page, request }) => {
  const api = await adminAPI(request)
  const inc = await (await api.post('/incidents', { type: 'FA', location: { address: '9 Alarm Rd' } })).json()
  await api.post(`/incidents/${inc.number}/notes`, { text: 'Caller sees smoke from the roof' })
  await signIn(page)
  await page.getByText(inc.number).first().click()
  await expect(page.getByText(/mentions fire/)).toBeVisible({ timeout: 20_000 })
  await page.getByRole('button', { name: /Change to Structure fire/ }).click()
  await expect.poll(async () => (await api.get('/state')).incidents.find((i: { number: string }) => i.number === inc.number).type).toBe('SF')
})
