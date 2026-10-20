import { expect, test } from '@playwright/test'
import { adminAPI, signIn } from './helpers'

// A 1×1 PNG, enough for the server to take it as a picture and the browser to show it.
const PNG = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg==', 'base64')

test('a pre-plan photo is added to a premise and shown in the views of an incident there', async ({ page, request }) => {
  const api = await adminAPI(request)
  const prem = await (await api.put('/premises', { address: '77 Preplan Way', lat: 37.2801, lon: -79.9302, hazards: ['Knox box'] })).json()
  const up = await request.post(`api/premises/${prem.id}/photos?caption=Rear%20door%20and%20Knox%20box&facing=S`, { headers: { 'X-BitDispatcher': '1', 'Content-Type': 'image/png' }, data: PNG })
  expect(up.ok()).toBeTruthy()
  const inc = await (await api.post('/incidents', { type: 'SF', location: { address: '77 Preplan Way', lat: 37.2801, lon: -79.9302 } })).json()

  const errors = await signIn(page)
  await page.getByRole('list', { name: 'Open incidents' }).getByRole('button', { name: new RegExp(inc.number) }).click()
  await page.getByRole('complementary', { name: new RegExp(`Incident ${inc.number}`) }).getByRole('button', { name: 'Views' }).click()
  const dlg = page.getByRole('dialog', { name: 'Views of the place' })
  await expect(dlg.getByText('Rear door and Knox box').first()).toBeVisible()
  await expect(dlg.getByText(/from the south/).first()).toBeVisible()
  await expect.poll(() => dlg.locator('figure img').evaluate((i: HTMLImageElement) => i.complete && i.naturalWidth)).toBe(1)
  expect(errors).toEqual([])
})
