import { expect, test } from '@playwright/test'
import { adminAPI, signIn } from './helpers'

// Self-service setup: the checklist, the go-live check and the support bundle.

test('the setup checklist shows what is left, and takes a confirmation', async ({ page, request }) => {
  const api = await adminAPI(request)
  await api.put('/admin/setup/training', { done: false })
  const errors = await signIn(page, 'admin', undefined, 'admin')
  await page.goto('/#/admin/setup')
  await expect(page.getByRole('heading', { name: 'Setup checklist' })).toBeVisible()
  await expect(page.getByTestId('setup-left')).toContainText(/required/)
  const training = page.getByTestId('step-training')
  await training.getByRole('button', { name: 'Mark done' }).click()
  await training.getByLabel('Note').fill('E2E: all three shifts')
  await training.getByRole('button', { name: 'Confirm' }).click()
  await expect(training.getByText(/Confirmed by admin.*E2E: all three shifts/)).toBeVisible()
  // A step bitdispatcher checks itself has no Mark done.
  await expect(page.getByTestId('step-stations').getByRole('button', { name: 'Mark done' })).toHaveCount(0)
  await page.getByTestId('step-stations').getByRole('link', { name: 'Open' }).click()
  await expect(page).toHaveURL(/#\/admin\/stations/)
  await page.goto('/#/admin/setup')
  await page.getByTestId('step-training').getByRole('button', { name: /Undo/ }).click()
  await expect(page.getByTestId('step-training').getByRole('button', { name: 'Mark done' })).toBeVisible()
  expect(errors).toEqual([])
})

test('the go-live check runs, and says what failed', async ({ page }) => {
  await signIn(page, 'admin', undefined, 'admin')
  await page.goto('/#/admin/golive')
  await page.getByRole('button', { name: 'Run the check' }).click()
  await expect(page.getByTestId('golive-summary')).toBeVisible()
  await expect(page.getByTestId('check-ledger')).toContainText('every hash and link checked')
  await expect(page.getByTestId('check-restore')).toContainText('replayed')
  await expect(page.getByTestId('check-calls')).toContainText('test calls')
  // The e2e system is the demo: its demonstration data fails the check.
  await expect(page.getByTestId('check-demo').getByLabel('Failed')).toBeVisible()
  await expect(page.getByTestId('golive-summary')).toContainText('Not ready')
})

test('the support bundle downloads, and the support contact is saved', async ({ page }) => {
  await signIn(page, 'admin', undefined, 'admin')
  await page.goto('/#/admin/support')
  await page.getByLabel('Your agency\'s support contact').fill('E2E help desk, 24 hours')
  await page.getByRole('button', { name: 'Save' }).click()
  await expect(page.getByTestId('support-shown')).toHaveText('E2E help desk, 24 hours')
  const [dl] = await Promise.all([page.waitForEvent('download'), page.getByRole('button', { name: 'Download the bundle' }).click()])
  expect(dl.suggestedFilename()).toMatch(/^bitdispatcher-support-\d{8}-\d{4}\.zip$/)
  await page.getByLabel('Your agency\'s support contact').fill('')
  await page.getByRole('button', { name: 'Save' }).click()
  await expect(page.getByTestId('support-shown')).toContainText('Community support')
})
