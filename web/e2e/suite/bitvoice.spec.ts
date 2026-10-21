import { test, expect } from '@playwright/test'
import { adminAPI, signIn } from './helpers'

const SIM = 'https://127.0.0.1:9644'

test('a BitVoice call shows its caller-ID check, and a caller photo stays hidden until revealed and is recorded', async ({ page, request }) => {
  test.setTimeout(120_000)
  const api = await adminAPI(request)
  const fp = await (await api.post('/admin/integrations/bitvoice/fingerprint', { url: SIM })).json()
  const r = await api.put('/admin/integrations', { bitvoiceUrl: SIM, bitvoiceToken: 'bv_simulated', bitvoiceFingerprint: fp.fingerprint, eidoToken: 'bd_eido_e2e', radioSecret: 'bd_radio_e2e', aliListen: '' })
  expect(r.ok(), await r.text()).toBeTruthy()

  // A simulated call in progress whose caller ID was checked (most are).
  type C = { id: string; state: string; incident?: string; sources?: string[]; handling?: { callerId?: { verified?: string } } }
  let call: C | undefined
  await expect(async () => {
    const st = (await api.get('/state')) as { calls: C[] }
    call = st.calls.find((c) => c.state !== 'ended' && c.state !== 'ringing' && !c.incident && c.sources?.includes('bitvoice') && c.handling?.callerId?.verified)
    expect(call).toBeTruthy()
  }).toPass({ timeout: 60_000, intervals: [1000] })
  const id = call!.id

  // The caller sends a photo.
  const sent = await request.post(`${SIM}/sim/photo?call=${encodeURIComponent(id)}`)
  expect(sent.ok(), await sent.text()).toBeTruthy()
  const { sha256 } = (await sent.json()) as { sha256: string }
  // bitdispatcher reads the photo's facts from BitVoice when told a photo came.
  await expect(async () => {
    const st = (await api.get('/state')) as { calls: (C & { handling?: { evidence?: { items: { sha256: string }[] } } })[] }
    expect(st.calls.find((c) => c.id === id)?.handling?.evidence?.items.map((i) => i.sha256)).toContain(sha256)
  }).toPass({ timeout: 15_000, intervals: [500] })

  const inc = await api.post('/incidents', { type: 'SICK', call: id, location: { lat: 37.2712, lon: -79.9418, address: '215 Church Ave SW' } })
  expect(inc.ok(), await inc.text()).toBeTruthy()
  const { number } = (await inc.json()) as { number: string }

  await signIn(page)
  const panel = page.getByRole('complementary', { name: `Incident ${number}` })
  await expect(async () => {
    await page.getByRole('main').or(page.locator('body')).getByText(number, { exact: true }).first().click()
    await expect(panel).toBeVisible({ timeout: 2000 })
  }).toPass({ timeout: 20_000 })
  await expect(panel.getByText(/number (verified|NOT verified)/).first()).toBeVisible()

  const photos = panel.getByRole('region', { name: 'Caller photos and clips' })
  await expect(photos).toBeVisible({ timeout: 15_000 })
  await expect(photos.getByText(`SHA-256 ${sha256}`)).toBeVisible()
  await expect(photos.getByText(/GPS 3\d\.\d{5}/)).toBeVisible()
  // Shown as facts only: no picture until a person chooses to look.
  await expect(photos.getByRole('img')).toHaveCount(0)
  await photos.getByRole('button', { name: 'Reveal (recorded)' }).click()
  const img = photos.getByRole('img', { name: /Photo caller-photo-1\.jpg/ })
  await expect(img).toBeVisible()
  await expect.poll(() => img.evaluate((el: HTMLImageElement) => el.naturalWidth)).toBeGreaterThan(0)

  await photos.getByRole('button', { name: 'Hold as evidence' }).click()
  await expect(photos.getByText('held as evidence')).toBeVisible()

  // Each look and the hold are in the ledger, with the photo's hash.
  await expect(async () => {
    const led = JSON.stringify(await api.get('/ledger?type=call.action'))
    expect(led).toContain('evidence revealed')
    expect(led).toContain(`SHA-256 ${sha256}`)
    expect(led).toContain('evidence held')
  }).toPass({ timeout: 10_000 })
})
