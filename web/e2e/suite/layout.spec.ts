import { expect, test } from '@playwright/test'
import { signIn } from './helpers'

test('the console layout: ready-made layouts per duty, sections dragged and snapped into place, the lock', async ({ page }) => {
  const errors = await signIn(page)
  const sec = (s: string) => page.locator(`[data-section=${s}]`)
  // A supervisor starts on the full console, locked: nothing to drag.
  for (const s of ['queue', 'map', 'units', 'incident']) await expect(sec(s)).toBeVisible()
  // Every section fills its space: the map, drawn inside its box, too.
  await expect(sec('map').locator('canvas.maplibregl-canvas')).toBeVisible()
  const outer = (await sec('map').boundingBox())!.height
  await expect.poll(async () => (await sec('map').locator('canvas.maplibregl-canvas').boundingBox())!.height).toBeGreaterThan(outer - 40)
  await expect(page.getByLabel('Move Map')).toHaveCount(0)

  await page.getByRole('button', { name: 'Layout', exact: true }).click()
  const menu = page.getByRole('dialog', { name: 'Layout' })
  await expect(menu.getByRole('button', { name: /Overview/ })).toBeVisible()
  await page.screenshot({ path: '/tmp/ws-menu.png' })
  await menu.getByRole('button', { name: /Getting started/ }).click()
  await expect(sec('map')).toHaveCount(0)
  await expect(sec('queue')).toBeVisible()
  await expect(menu.getByRole('button', { name: /^Map: click to add/ })).toBeDisabled()

  // Unlocked, the map is dragged from the menu onto the incident's right edge: they share that space.
  await menu.getByRole('button', { name: 'Unlock layout', exact: true }).click()
  const tile = menu.getByRole('button', { name: /^Map: click to add/ })
  const inc = await sec('incident').boundingBox()
  await tile.hover()
  await page.mouse.down()
  await page.mouse.move(inc!.x + inc!.width - 20, inc!.y + inc!.height / 2, { steps: 8 })
  await page.mouse.move(inc!.x + inc!.width - 10, inc!.y + inc!.height / 2, { steps: 4 })
  await page.mouse.up()
  await expect(sec('map')).toBeVisible()
  const m = await sec('map').boundingBox(), i = await sec('incident').boundingBox()
  expect(m!.x).toBeGreaterThan(i!.x + i!.width - 10)

  // Dragged by its title bar onto the middle of the calls: the two trade places.
  await page.screenshot({ path: '/tmp/ws-unlocked.png' })
  const q = await sec('queue').boundingBox()
  await page.getByLabel('Move Map').hover()
  await page.mouse.down()
  await page.mouse.move(q!.x + q!.width / 2, q!.y + q!.height / 2, { steps: 10 })
  await expect(page.getByText('Trade places with Calls and incidents')).toBeVisible()
  await page.screenshot({ path: '/tmp/ws-dragging.png' })
  await page.mouse.up()
  expect((await sec('map').boundingBox())!.x).toBeLessThan((await sec('incident').boundingBox())!.x)
  expect((await sec('queue').boundingBox())!.x).toBeGreaterThan((await sec('incident').boundingBox())!.x)

  // Taken off by its X, then the screen cleared: a hint where the sections were.
  await page.getByRole('button', { name: 'Take Map off the screen' }).click()
  await expect(sec('map')).toHaveCount(0)
  await page.getByRole('button', { name: 'Layout', exact: true }).click()
  await page.getByRole('dialog', { name: 'Layout' }).getByRole('button', { name: 'Clear the screen' }).click()
  await expect(page.getByText('No sections on this screen.')).toBeVisible()

  // Locked, and kept: the next visit shows the same.
  await page.getByRole('button', { name: /Layout unlocked: click to lock it/ }).first().click()
  await page.reload()
  await expect(page.getByText('No sections on this screen.')).toBeVisible()
  await expect(page.getByRole('button', { name: /Layout locked/ })).toBeVisible()
  await page.getByRole('button', { name: 'Layout', exact: true }).click()
  await page.getByRole('dialog', { name: 'Layout' }).getByRole('button', { name: /Full console/ }).click()
  await expect(sec('units')).toBeVisible()
  expect(errors).toEqual([])
})
