// Screenshots of the dispatch map at several zooms, for review: node e2e/map.mjs <url> <user> <password> <out-dir>
// It signs in, searches the map for each place, sets the zoom with the keyboard and takes a shot.
import { chromium } from '@playwright/test'
const [url, user, pass, out, theme = 'night'] = process.argv.slice(2)
const browser = await chromium.launch()
const ctx = await browser.newContext({ ignoreHTTPSErrors: true, viewport: { width: 1920, height: 1080 } })
await ctx.addInitScript((t) => localStorage.setItem('bd.mapTheme', t), theme)
const page = await ctx.newPage()
const errors = []
page.on('console', (m) => m.type() === 'error' && errors.push(m.text()))
page.on('pageerror', (e) => errors.push(String(e)))
await page.goto(url)
await page.fill('#u', user)
await page.fill('#p', pass)
await page.fill('#c', 'D1')
await page.click('button:has-text("Sign in")')
await page.waitForSelector('text=Units', { timeout: 10000 })
await page.waitForTimeout(2500)
const search = page.getByLabel('Search the map')
const shots = [
  ['overview', 'Roanoke', 2],
  ['city', 'Campbell Avenue Southwest', 2.5],
  ['street', '18 Kirk Avenue', 0],
  ['hospital', 'Carilion Roanoke Memorial Hospital', 0.5],
  ['salem', 'Salem', 0],
]
for (const [name, q, out_] of shots) {
  await search.fill(q)
  await page.waitForTimeout(700)
  await search.press('Enter')
  await page.waitForTimeout(1500)
  if (out_ > 0) {
    // Zoom out with the keyboard on the map canvas.
    await page.locator('canvas.maplibregl-canvas').focus()
    for (let i = 0; i < Math.round(out_ * 2); i++) await page.keyboard.press('-').then(() => page.waitForTimeout(250))
  }
  await page.waitForTimeout(3500)
  await page.screenshot({ path: `${out}/map-${theme}-${name}.png` })
}
console.log(JSON.stringify({ errors: errors.slice(0, 20) }, null, 1))
await browser.close()
