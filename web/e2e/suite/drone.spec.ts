import { expect, test } from '@playwright/test'
import { adminAPI, signIn } from './helpers'

// The drone simulator (bin/dronesim, started by setup.ts) stands in for a PX4 drone and its camera.
test('a drone: asked for, approved by the supervisor, flown to the incident with its live picture, recorded, and sent home', async ({ page, request }) => {
  test.setTimeout(150_000)
  const api = await adminAPI(request)
  const saved = await api.put('/admin/drones', { drones: [{ id: 'D9', name: 'Test drone', mavlink: 'udp-listen:127.0.0.1:14651', video: 'rtsp://127.0.0.1:8655/camera', altM: 20, minBattery: 30, approval: true, record: true }] })
  expect(saved.ok()).toBeTruthy()
  await expect.poll(async () => (await api.get('/admin/drones')).telemetry.D9?.connected, { timeout: 15_000 }).toBe(true)

  const errors = await signIn(page)
  await page.addLocatorHandler(page.getByRole('alert', { name: 'Urgent communication' }), async (b) => {
    for (const x of await b.getByRole('button', { name: 'Acknowledge' }).all()) await x.click()
  })
  const inc = await (await api.post('/incidents', { type: 'MISS', location: { address: '7 Drone Test Way', lat: 37.2738, lon: -79.9385 } })).json()
  await page.getByText('7 Drone Test Way').first().click()
  const panel = page.getByRole('complementary', { name: `Incident ${inc.number}` })
  const box = panel.getByRole('region', { name: 'Drone' })
  await box.getByRole('button', { name: 'Send a drone' }).click()
  await expect(box.getByText('waiting for the pilot')).toBeVisible()
  await box.getByRole('button', { name: 'Approve launch' }).click()
  await expect(box.getByText('over the scene')).toBeVisible({ timeout: 60_000 })
  await expect(box.getByText('● REC')).toBeVisible()

  // The live picture plays in the incident.
  await box.getByRole('button', { name: 'Live picture' }).click()
  const video = box.getByLabel('Live picture from Test drone')
  await expect.poll(() => video.evaluate((v: HTMLVideoElement) => v.readyState >= 2 && v.currentTime > 1), { timeout: 20_000 }).toBe(true)
  const lag = await video.evaluate((v: HTMLVideoElement) => v.buffered.end(v.buffered.length - 1) - v.currentTime)
  expect(lag).toBeLessThan(1.5)
  await page.screenshot({ path: '/tmp/drone-live.png' })

  await box.getByRole('button', { name: 'Return home' }).click()
  await expect.poll(async () => (await api.get('/state')).drones.find((d: { id: string }) => d.id === 'D9')?.status, { timeout: 60_000 }).toBe('docked')
  const clips = panel.getByRole('list', { name: 'Drone recordings' })
  await expect(clips.getByRole('listitem')).toHaveCount(1)
  const href = await clips.getByRole('link', { name: 'Play' }).getAttribute('href')
  const r = await request.get(href!)
  expect(r.status()).toBe(200)
  expect(r.headers()['content-type']).toBe('video/mp4')
  expect((await r.body()).subarray(4, 8).toString()).toBe('ftyp')

  await api.put('/admin/drones', { drones: [] })
  expect(errors).toEqual([])
})
