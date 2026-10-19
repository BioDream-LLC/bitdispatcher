import { readFileSync } from 'node:fs'
import { expect, type Page, type APIRequestContext } from '@playwright/test'
import { state } from './setup'

export const creds = () => JSON.parse(readFileSync(state, 'utf8')) as { password: string }

/** Signs in through the login screen. */
export async function signIn(page: Page, user = 'admin', password = creds().password, position = 'D1') {
  const errors: string[] = []
  page.on('pageerror', (e) => errors.push(String(e)))
  await page.goto('/')
  await page.fill('#u', user)
  await page.fill('#p', password)
  await page.fill('#c', position)
  await page.click('button:has-text("Sign in")')
  return errors
}

/** An API client signed in as the administrator. */
export async function adminAPI(request: APIRequestContext) {
  const r = await request.post('api/auth/login', { headers: { 'X-BitDispatcher': '1' }, data: { username: 'admin', password: creds().password, console: 'E2E' } })
  expect(r.ok()).toBeTruthy()
  return {
    get: async (p: string) => (await request.get('api' + p)).json(),
    post: async (p: string, data: unknown = {}) => request.post('api' + p, { headers: { 'X-BitDispatcher': '1' }, data }),
    put: async (p: string, data: unknown) => request.put('api' + p, { headers: { 'X-BitDispatcher': '1' }, data }),
  }
}
