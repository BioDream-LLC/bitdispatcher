import { expect, test } from '@playwright/test'
import { signIn } from './helpers'

// The admin menu, clicked rather than opened by address: relative links under the admin pages once
// piled up as /admin/setup/stations/setup/... and the page never drew. Pages sit in folding groups;
// the current page's group is always open.
test('the admin menu opens each page from any other, through its groups and the filter', async ({ page }) => {
  const errors = await signIn(page, 'admin', undefined, 'admin')
  await page.goto('/#/admin')
  await expect(page).toHaveURL(/#\/admin\/setup$/)
  const menu = page.getByRole('navigation', { name: 'Administration' })
  for (const [group, label, path] of [
    ['Units & resources', 'Stations', 'stations'],
    ['Units & resources', 'Units', 'units'],
    ['Call handling', 'Response plans', 'plans'],
    ['Units & resources', 'Drones', 'drones'],
    ['Getting started', 'Setup checklist', 'setup'],
  ]) {
    const head = menu.getByRole('button', { name: new RegExp(group) })
    if ((await head.getAttribute('aria-expanded')) !== 'true') await head.click()
    await menu.getByRole('link', { name: label, exact: true }).click()
    await expect(page).toHaveURL(new RegExp(`#/admin/${path}$`))
    await expect(page.getByRole('heading', { name: label, exact: true }).first()).toBeVisible()
    await expect(head).toHaveAttribute('aria-expanded', 'true')
  }
  await menu.getByPlaceholder('Find a page').fill('ledger')
  await page.keyboard.press('Enter')
  await expect(page).toHaveURL(/#\/admin\/ledger$/)
  expect(errors).toEqual([])
})
