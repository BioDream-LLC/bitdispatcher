// Takes screenshots of a running bitdispatcher, for review: node e2e/shot.mjs <url> <user> <password> <out-dir> [steps]
import { chromium } from '@playwright/test'
const [url, user, pass, out, steps = 'console'] = process.argv.slice(2)
const browser = await chromium.launch()
const ctx = await browser.newContext({ ignoreHTTPSErrors: true, viewport: { width: 1920, height: 1080 } })
const page = await ctx.newPage()
const errors = []
page.on('console', (m) => m.type() === 'error' && errors.push(m.text()))
page.on('pageerror', (e) => errors.push(String(e)))
await page.goto(url)
await page.fill('#u', user)
await page.fill('#p', pass)
await page.fill('#c', process.env.CONSOLE ?? 'D1')
await page.click('button:has-text("Sign in")')
await page.waitForSelector(process.env.READY ?? 'text=Units', { timeout: 10000 }).catch(async (e) => { await page.screenshot({ path: out + '/login-failed.png' }); throw e })
await page.waitForTimeout(2500)
for (const step of steps.split(',')) {
  if (step === 'console') await page.screenshot({ path: `${out}/console.png` })
  else if (step.startsWith('click:')) { await page.click(step.slice(6), { timeout: 5000 }).catch((e) => errors.push('CLICK ' + step + ': ' + e.message.split('\n')[0])); await page.waitForTimeout(1200) }
  else if (step.startsWith('type:')) { await page.keyboard.type(step.slice(5)); await page.waitForTimeout(300) }
  else if (step.startsWith('key:')) { await page.keyboard.press(step.slice(4)); await page.waitForTimeout(1200) }
  else if (step.startsWith('goto:')) { await page.goto(url + step.slice(5)); await page.waitForTimeout(1500) }
  else if (step.startsWith('map:')) { const [lon, lat, z] = step.slice(4).split(';').map(Number); await page.evaluate(([lon, lat, z]) => window.__bdMap?.jumpTo({ center: [lon, lat], zoom: z }), [lon, lat, z]); await page.waitForTimeout(3500) }
  else if (step.startsWith('wait:')) await page.waitForTimeout(Number(step.slice(5)))
  else if (step.startsWith('shot:')) await page.screenshot({ path: `${out}/${step.slice(5)}.png` })
}
console.log(JSON.stringify({ errors }, null, 1))
await browser.close()
