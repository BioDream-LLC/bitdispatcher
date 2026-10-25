import { expect, test } from '@playwright/test'
import { adminAPI, signIn } from './helpers'

// The October 2026 features: road closures, the phoned alarm card, the intake, location links, cameras.

test('a closed road is listed for the supervisor, and lifted', async ({ page, request }) => {
  const api = await adminAPI(request)
  const r = await api.put('/roads/closures', { reason: 'Water main break', line: [[-79.95, 37.27], [-79.94, 37.27]], detour: 'Use Elm' })
  expect(r.ok()).toBeTruthy()
  const { id } = await r.json()
  await signIn(page, 'admin', undefined, 'admin')
  await page.goto('/#/admin/roads')
  const row = page.getByRole('row', { name: /Water main break/ })
  await expect(row).toBeVisible()
  await expect(row.getByText('Use Elm')).toBeVisible()
  await row.getByRole('button', { name: 'Lift' }).click()
  await expect(page.getByText(/Ended in the last day: Water main break \(lifted by admin\)/)).toBeVisible()
  const roads = await api.get('/roads')
  expect(roads.closures.find((c: { id: string }) => c.id === id)).toBeUndefined()
})

test('a call-taker types an alarm company’s phone call on the alarm card', async ({ page }) => {
  await signIn(page)
  await page.getByRole('button', { name: /Phoned alarm/ }).click()
  const dlg = page.getByRole('dialog', { name: 'Alarm company on the phone' })
  await dlg.getByLabel('Alarm company').fill('E2E Alarm Co')
  await dlg.getByLabel('Their event number').fill('E2E-77')
  await dlg.getByLabel('Zone or sensor').fill('2 front door')
  await dlg.getByLabel('Address').fill('77 Alarm Way')
  await dlg.getByRole('button', { name: 'Put the alarm in the queue' }).click()
  await expect(page.getByRole('button', { name: /E2E Alarm Co.*BURGLARY zone 2 front door/ }).first()).toBeVisible()
})

test('the intake hands an emergency to a person, and takes a report otherwise', async ({ page }) => {
  await signIn(page, 'admin', undefined, 'admin')
  await page.goto('/#/admin/intake')
  const box = page.getByRole('region', { name: 'Try the intake' })
  await box.getByRole('button', { name: 'Start a conversation' }).click()
  await expect(box.getByText(/non-emergency line/)).toBeVisible()
  const say = async (t: string) => {
    await box.getByLabel('As the caller').fill(t)
    await box.getByRole('button', { name: 'Send' }).click()
  }
  await say('No, nobody is hurt')
  await expect(box.getByText('Briefly, what would you like to report?')).toBeVisible()
  await say('he has a gun and he is outside')
  await expect(box.getByText(/handed to a person/)).toBeVisible()
  await expect(box.getByText(/connecting you to a 911 call-taker/)).toBeVisible()
})

test('a caller shares their location by link, and the call has it', async ({ browser, request }) => {
  const api = await adminAPI(request)
  // A live call from the BitVoice simulator.
  let call = ''
  await expect.poll(async () => {
    const st = await api.get('/state')
    call = st.calls.find((c: { state: string; id: string }) => c.state !== 'ended')?.id ?? ''
    return call
  }, { timeout: 30_000 }).not.toBe('')
  const r = await api.post(`/calls/${encodeURIComponent(call)}/location-link`, { send: false })
  expect(r.ok()).toBeTruthy()
  const { url } = await r.json()
  const phone = await browser.newContext({ ignoreHTTPSErrors: true, permissions: ['geolocation'], geolocation: { latitude: 37.2788, longitude: -79.9561, accuracy: 6 }, viewport: { width: 390, height: 800 } })
  const p = await phone.newPage()
  await p.goto(url)
  await expect(p.getByRole('heading', { name: 'Show 911 where you are' })).toBeVisible()
  await p.getByRole('button', { name: 'Share my location' }).click()
  await expect(p.getByText(/Sent to 911/)).toBeVisible()
  await expect.poll(async () => {
    const st = await api.get('/state')
    return st.calls.find((c: { id: string }) => c.id === call)?.location?.method ?? ''
  }).toContain('by link')
  await p.getByRole('button', { name: 'Stop sharing' }).click()
  await expect(p.getByText('Stopped. Thank you.')).toBeVisible()
  await phone.close()
})

test('a camera near an incident asks why before it opens', async ({ page, request }) => {
  const api = await adminAPI(request)
  const st = await api.get('/state')
  expect((await api.put('/admin/settings', { ...st.settings, next: { ...(st.settings.next ?? {}), cameras: { on: true, radius: 500 } } })).ok()).toBeTruthy()
  expect((await api.put('/admin/cameras', { name: 'E2E Main cam', owner: 'City', lat: 37.2701, lon: -79.9411, heading: 180, live: 'https://vms.example.gov/cam/e2e' })).ok()).toBeTruthy()
  const inc = await (await api.post('/incidents', { type: 'FALL', location: { lat: 37.2700, lon: -79.9411, address: '1 Camera Test St' } })).json()
  await signIn(page)
  await page.getByText(inc.number).first().click()
  const near = page.getByRole('region', { name: 'Cameras near' })
  await near.getByRole('button', { name: 'E2E Main cam' }).click()
  const dlg = page.getByRole('dialog', { name: 'E2E Main cam' })
  await expect(dlg.getByText(/recorded: who, when and why/)).toBeVisible()
  await dlg.getByRole('button', { name: 'Open the camera' }).click()
  await expect(dlg.getByRole('link', { name: /Open its live view/ })).toHaveAttribute('href', 'https://vms.example.gov/cam/e2e')
  const looks = await api.get('/admin/cameras/looks')
  expect(looks.items[0]).toMatchObject({ name: 'E2E Main cam', incident: inc.number })
})
