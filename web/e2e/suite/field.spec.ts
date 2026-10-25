import { expect, test } from '@playwright/test'
import { adminAPI, signIn } from './helpers'

test('a field crew works an incident from the mobile terminal', async ({ page, request }) => {
  const api = await adminAPI(request)
  expect((await api.put('/admin/users/e3crew', { name: 'Engine 3 crew', role: 'field', password: 'engine-three-crew-e2e', disabled: false })).ok()).toBeTruthy()
  const inc = await (await api.post('/incidents', { type: 'FALL', location: { address: '1 Test St' } })).json()
  const r = await (await api.post('/command', { line: `D E3 #${inc.number} / e2e`, incident: '' })).json()
  expect(r.ok).toBeTruthy()
  await signIn(page, 'e3crew', 'engine-three-crew-e2e', 'field', 'E3')
  await expect(page.getByText('HOLD FOR EMERGENCY')).toBeVisible()
  await expect(page.getByText('Fall').first()).toBeVisible()
  await page.getByRole('button', { name: /En route/ }).click()
  await expect.poll(async () => (await api.get('/state')).units.find((u: { id: string }) => u.id === 'E3').status).toBe('enroute')
  await page.getByLabel('Note to dispatch').fill('Two minutes out')
  await page.getByRole('button', { name: 'Send', exact: true }).click()
  await expect(page.getByText('E3: Two minutes out')).toBeVisible()
  // A photo from the terminal is on the incident, and the dispatcher sees it.
  const png = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAIAAACQd1PeAAAADElEQVR4nGP4z8AAAAMBAQDJ/pLvAAAAAElFTkSuQmCC', 'base64')
  await page.getByLabel('Choose files').setInputFiles({ name: 'side-c.png', mimeType: 'image/png', buffer: png })
  await expect(page.getByRole('img', { name: 'side-c.png' })).toBeVisible()
  const files = (await api.get('/state')).incidents.find((i: { number: string }) => i.number === inc.number).files
  expect(files[0]).toMatchObject({ name: 'side-c.png', unit: 'E3', type: 'image/png' })
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

test('dispatch and a crew message each other, and the crew takes a waiting incident', async ({ page, request }) => {
  const api = await adminAPI(request)
  expect((await api.put('/admin/users/e4crew2', { name: 'Engine 4 crew', role: 'field', password: 'engine-four-crew-msg-e2e', disabled: false })).ok()).toBeTruthy()
  // The agency lets crews attach themselves.
  const st = await api.get('/state')
  expect((await api.put('/admin/settings', { ...st.settings, duties: { ...(st.settings.duties ?? {}), selfAttach: true } })).ok()).toBeTruthy()
  // E4 must be free for the test.
  await api.post('/command', { line: 'AV E4' })
  const errors = await signIn(page, 'e4crew2', 'engine-four-crew-msg-e2e', 'field', 'E4')
  expect((await api.post('/messages', { to: ['E4'], text: 'Hydrant out at Kirk and 1st', urgent: false })).ok()).toBeTruthy()
  const box = page.getByRole('region', { name: 'Messages' })
  await expect(box.getByText('Hydrant out at Kirk and 1st')).toBeVisible()
  await box.getByRole('button', { name: 'Copy' }).click()
  await expect.poll(async () => (await api.get('/state')).messages.some((m: { from: string; text: string; to: string[] }) => m.from === 'E4' && m.text === 'Copy' && m.to.includes('DISPATCH'))).toBeTruthy()
  // The dispatcher's message was read on the terminal.
  await expect.poll(async () => (await api.get('/state')).messages.find((m: { text: string }) => m.text === 'Hydrant out at Kirk and 1st')?.read?.E4).toBeTruthy()
  // An urgent message holds the top of the screen until Got it.
  await api.post('/messages', { to: ['ALL-FIRE'], text: 'Evacuate the structure now', urgent: true })
  await expect(page.getByRole('alert').filter({ hasText: 'Evacuate the structure now' })).toBeVisible()
  await page.getByRole('button', { name: 'Got it' }).click()
  await expect(page.getByRole('alert').filter({ hasText: 'Evacuate the structure now' })).toBeHidden()
  // A waiting incident, taken from the terminal.
  const inc = await (await api.post('/incidents', { type: 'SF', location: { address: '9 Take It Ln', lat: 37.271, lon: -79.94 } })).json()
  await page.getByRole('listitem').filter({ hasText: '9 Take It Ln' }).getByRole('button', { name: 'Take it' }).click()
  await expect.poll(async () => (await api.get('/state')).units.find((u: { id: string }) => u.id === 'E4').incident).toBe(inc.number)
  expect(errors).toEqual([])
})

test('an urgent BOLO reaches the crew on the terminal, sounds, and is acknowledged by the crew', async ({ page, request }) => {
  const api = await adminAPI(request)
  expect((await api.put('/admin/users/e5crew', { name: 'Engine 5 crew', role: 'field', password: 'engine-five-crew-e2e', disabled: false })).ok()).toBeTruthy()
  await page.addInitScript(() => {
    const w = window as unknown as { __tones: number }
    w.__tones = 0
    class Counting extends OfflineAudioContext {
      constructor() { super(1, 48000, 48000) }
      resume() { return Promise.resolve() }
      createOscillator() { w.__tones++; return super.createOscillator() }
    }
    ;(window as unknown as { AudioContext: unknown }).AudioContext = Counting
  })
  const errors = await signIn(page, 'e5crew', 'engine-five-crew-e2e', 'field', 'E5')
  const subject = 'Armed suspect seen near Kirk ' + Date.now()
  const r = await api.post('/comms', { kind: 'bolo', priority: 'urgent', subject, body: 'Approach with caution', plate: 'ZZT987', to: ['E5'] })
  expect(r.ok()).toBeTruthy()
  const id = (await r.json()).id
  const banner = page.getByRole('alert').filter({ hasText: subject })
  await expect(banner).toBeVisible({ timeout: 20_000 })
  // The page was used (sign-in), so sound is allowed: the urgent tone was made.
  await expect.poll(() => page.evaluate(() => (window as unknown as { __tones: number }).__tones)).toBeGreaterThan(0)
  await banner.getByRole('button', { name: subject }).click()
  await expect(page.getByRole('region', { name: 'Messages and BOLOs for this unit' }).getByText('Approach with caution')).toBeVisible()
  await banner.getByRole('button', { name: 'Acknowledge' }).click()
  await expect(banner).toBeHidden()
  const c = (await api.get('/comms/' + encodeURIComponent(id))).versions.at(-1)
  expect(Object.keys(c.acked ?? {})).toContain('e5crew')
  expect(Object.keys(c.opened ?? {})).toContain('e5crew')
  expect(errors).toEqual([])
})
