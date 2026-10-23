import { expect, test } from '@playwright/test'

test('the sign-in page says when the server is not answering, and stops saying it when it does', async ({ page }) => {
  let down = true
  await page.route('**/api/auth/me', (r) => (down ? r.abort('connectionrefused') : r.continue()))
  await page.goto('/')
  await expect(page.getByRole('alert').filter({ hasText: 'The bitdispatcher server is not answering.' })).toBeVisible()
  down = false
  await expect(page.getByText('The bitdispatcher server is not answering.')).toHaveCount(0, { timeout: 10_000 })
})
