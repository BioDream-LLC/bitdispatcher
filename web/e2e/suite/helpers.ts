import { readFileSync } from 'node:fs'
import { expect, type Page, type APIRequestContext } from '@playwright/test'
import { state } from './setup'

export const creds = () => JSON.parse(readFileSync(state, 'utf8')) as { password: string }

const DUTY: Record<string, string> = { calltaker: 'Call-taker', police: 'Police dispatcher', fire: 'Fire dispatcher', ems: 'EMS dispatcher', dispatcher: 'Dispatcher (all disciplines)', supervisor: 'Supervisor', admin: 'Administrator', records: 'Records' }

/** Signs in through the sign-in page, choosing the duty when asked (a supervisor by default: the console with
 * the supervisor's rights), and typing the unit for a field crew. */
export async function signIn(page: Page, user = 'admin', password = creds().password, duty = 'supervisor', unit = '') {
  const errors: string[] = []
  page.on('pageerror', (e) => errors.push(String(e)))
  await page.goto('/')
  await page.fill('#u', user)
  await page.fill('#p', password)
  await page.getByRole('button', { name: 'Sign in', exact: true }).click()
  const chooser = page.getByText('What are you working?')
  const unitField = page.locator('#unit')
  await expect(chooser.or(unitField).or(page.getByText('911 CALLS')).or(page.getByText('HOLD FOR EMERGENCY')).first()).toBeVisible()
  if (await chooser.isVisible()) await page.getByRole('button', { name: new RegExp(DUTY[duty] ?? duty) }).click()
  if (unit && (await unitField.isVisible().catch(() => false))) {
    await unitField.fill(unit)
    await page.getByRole('button', { name: 'Sign in', exact: true }).click()
  }
  return errors
}

/** answerAsk answers the questions asked before units are sent: confirm the warnings, give a reason. */
export async function answerAsk(page: Page) {
  for (let i = 0; i < 2; i++) {
    const dlg = page.getByRole('dialog', { name: /Confirm before sending|Why these units/ })
    if (!(await dlg.waitFor({ state: 'visible', timeout: 2500 }).then(() => true).catch(() => false))) return
    if (await dlg.getByRole('button', { name: 'Confirmed: send' }).isVisible().catch(() => false)) await dlg.getByRole('button', { name: 'Confirmed: send' }).click()
    else await dlg.getByRole('button', { name: 'Crew asked for it' }).click()
    await page.waitForTimeout(300)
  }
}

/** An API client signed in as the administrator. */
export async function adminAPI(request: APIRequestContext) {
  const r = await request.post('api/auth/login', { headers: { 'X-BitDispatcher': '1' }, data: { username: 'admin', password: creds().password, console: 'E2E', duty: 'admin' } })
  expect(r.ok()).toBeTruthy()
  return {
    get: async (p: string) => (await request.get('api' + p)).json(),
    post: async (p: string, data: unknown = {}) => request.post('api' + p, { headers: { 'X-BitDispatcher': '1' }, data }),
    put: async (p: string, data: unknown) => request.put('api' + p, { headers: { 'X-BitDispatcher': '1' }, data }),
  }
}
