// Touch interactions shared by Android and iPhone projects.
import { expect, test } from '@playwright/test'
import { readFileSync } from 'node:fs'
import { SEED_FILE, type Seed } from './seed'

const fixture = () => JSON.parse(readFileSync(SEED_FILE, 'utf8')) as Seed

test.beforeEach(async ({ context }) => {
  await context.addCookies(fixture().staffState.cookies)
})

test('touch navigation and visible close buttons release the page after dialogs', { tag: '@touch' }, async ({ page }) => {
  await page.goto(`/manage/${fixture().slug}/schedule`)
  const menu = page.getByRole('button', { name: 'Menu', exact: true })
  await menu.tap()
  const dialog = page.getByRole('dialog')
  await dialog.getByRole('button', { name: 'Close', exact: true }).tap()
  await expect(dialog).toHaveCount(0)

  await menu.tap()
  await dialog.getByRole('link', { name: 'Settings', exact: true }).tap()
  await expect(page).toHaveURL(new RegExp(`/manage/${fixture().slug}/settings$`))
  await expect(dialog).toHaveCount(0)

  await page.getByRole('button', { name: 'Language: English', exact: true }).tap()
  await dialog.getByRole('button', { name: 'Close', exact: true }).tap()
  await expect(dialog).toHaveCount(0)
  await expect.poll(() => page.evaluate(() => getComputedStyle(document.body).overflow)).not.toBe('hidden')
  const link = page.getByLabel('Registration link', { exact: true })
  await link.scrollIntoViewIfNeeded()
  await expect(link).toBeInViewport()
  await link.tap()
  await expect(link).toBeFocused()
})

test('a long form can scroll and submit by touch in a short landscape viewport', { tag: '@touch' }, async ({ page }, info) => {
  await page.setViewportSize({ width: 667, height: 375 })
  await page.goto('/platform/instances')
  await page.getByRole('button', { name: 'New calendar', exact: true }).tap()
  const dialog = page.getByRole('dialog')
  const name = `Touch ${info.project.name}`
  await dialog.getByLabel('Name', { exact: true }).fill(name)
  await dialog.getByLabel('Web address name').fill(`touch-${info.project.name}`)
  await dialog.getByLabel('Time zone').selectOption('Europe/London')
  await expect.poll(() => dialog.evaluate((el) => el.scrollHeight > el.clientHeight)).toBe(true)
  const create = dialog.getByRole('button', { name: 'Create', exact: true })
  await create.scrollIntoViewIfNeeded()
  await expect(create).toBeInViewport()
  await create.tap()
  await expect(dialog).toHaveCount(0)
  await expect(page.getByRole('heading', { name: `${name} Paused`, exact: true })).toBeVisible()
})
