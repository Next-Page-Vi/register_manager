import { expect, test, type Locator, type Page } from '@playwright/test'
import { readFileSync } from 'node:fs'
import { SEED_FILE, type Seed } from './seed'
import { formControlLayoutIssues } from './form-controls'

const fixture = () => JSON.parse(readFileSync(SEED_FILE, 'utf8')) as Seed
async function settle(page: Page) {
  await page.waitForLoadState('networkidle')
  await expect(page.locator('.motion-safe\\:animate-pulse, .animate-spin')).toHaveCount(0)
  await page.evaluate(() => document.fonts.ready)
  await page.waitForFunction(() => document.getAnimations().every((a) => a.playState !== 'running' || a.effect?.getTiming().iterations === Infinity))
}
const rect = (locator: Locator) => locator.evaluate((el) => el.getBoundingClientRect().toJSON())
const fontSize = (locator: Locator) => locator.evaluate((el) => parseFloat(getComputedStyle(el).fontSize))

test.beforeEach(async ({ context }) => {
  await context.addCookies([...fixture().staffState.cookies, ...fixture().customerState.cookies])
  await context.addInitScript(() => localStorage.setItem('rm_locale', 'en'))
})

for (const width of [768, 1024, 1280, 1440, 1920]) {
  for (const locale of ['en', 'ja']) {
    test(`staff records and header use available space at ${width}px in ${locale}`, async ({ page }, info) => {
      await page.setViewportSize({ width, height: 900 })
      await page.goto(`/manage/${fixture().slug}/appointments`)
      await page.evaluate((language) => localStorage.setItem('rm_locale', language), locale)
      await page.reload()
      await settle(page)
      await page.getByRole('tab').nth(1).click()
      await settle(page)
      expect((await rect(page.locator('header').first())).height).toBeLessThanOrEqual(66)
      expect(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)).toBe(false)
      const card = page.locator('[data-slot=card]').first()
      const [content, actions] = await card.locator('[data-slot=card-content]').evaluate((el) => Array.from(el.children).map((child) => child.getBoundingClientRect().toJSON()))
      if (await card.evaluate((el) => el.clientWidth >= 720)) expect(actions.left).toBeGreaterThanOrEqual(content.right)
      for (const action of await card.getByRole('button').all()) {
        const bounds = await rect(action)
        expect(bounds.right).toBeLessThanOrEqual((await rect(card)).right)
        expect(bounds.height).toBeGreaterThanOrEqual(40)
      }
      await page.screenshot({ path: info.outputPath('appointments.png'), fullPage: true })
      await page.goto('/platform/accounts')
      await settle(page)
      expect((await rect(page.locator('header').first())).height).toBeLessThanOrEqual(66)
      expect(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)).toBe(false)
      await page.screenshot({ path: info.outputPath('accounts.png'), fullPage: true })
    })
  }
}

test('date navigation stays together in customer pages and booking dialogs', { tag: '@browser' }, async ({ page }) => {
  const d = fixture()
  for (const width of [320, 375, 768]) {
    await page.setViewportSize({ width, height: 667 })
    const check = async (scope: Locator) => {
      const filter = scope.locator('[data-date-filter]')
      await filter.getByRole('button', { name: 'Choose a date', exact: true }).click()
      await settle(page)
      const [previous, next] = await filter.locator('[data-date-navigation] button').evaluateAll((els) => els.map((el) => el.getBoundingClientRect().toJSON()))
      expect(Math.abs(previous.top - next.top)).toBeLessThanOrEqual(1)
      expect(next.left).toBeGreaterThanOrEqual(previous.right)
      expect(next.right).toBeLessThanOrEqual((await rect(filter)).right)
      expect(await scope.evaluate(formControlLayoutIssues)).toEqual([])
    }
    await page.goto(`/i/${d.slug}/booking/times`)
    await settle(page)
    await check(page.locator('main'))
    await page.goto(`/manage/${d.slug}/appointments`)
    await page.getByRole('button', { name: 'New appointment', exact: true }).click()
    const dialog = page.getByRole('dialog')
    await dialog.locator('#new-booking-customer').selectOption(d.customerId)
    await dialog.getByRole('button', { name: 'Recommended times', exact: true }).click()
    await settle(page)
    await check(dialog)
    await page.keyboard.press('Escape')
  }
})

test('customer text and controls retain their size across portal dialogs', { tag: '@browser' }, async ({ page }) => {
  const d = fixture()
  for (const kind of ['staff', 'customer'] as const) {
    await page.goto(kind === 'staff' ? `/manage/${d.slug}/policy` : `/i/${d.slug}/booking`)
    await settle(page)
    expect(await fontSize(page.locator('main'))).toBe(kind === 'staff' ? 16 : 18)
    await page.getByRole('button', { name: 'Language: English', exact: true }).click()
    await settle(page)
    const dialog = page.getByRole('dialog')
    await expect(dialog).toHaveAttribute('data-ui-surface', kind)
    expect(await fontSize(dialog)).toBe(kind === 'staff' ? 16 : 18)
    const button = dialog.getByRole('button', { name: 'English', exact: true })
    await expect.poll(
      async () => (await rect(button)).height,
      { timeout: 5_000 },
    ).toBeGreaterThanOrEqual(kind === 'staff' ? 44 : 54)
    await page.keyboard.press('Escape')
  }
  await page.getByRole('button', { name: 'Cancel this appointment', exact: true }).first().click()
  await expect(page.getByRole('alertdialog')).toHaveAttribute('data-ui-surface', 'customer')
  expect(await fontSize(page.getByRole('alertdialog'))).toBe(18)
  await page.getByRole('button', { name: 'No, go back', exact: true }).click()
  await page.setViewportSize({ width: 375, height: 667 })
  await page.goto(`/manage/${d.slug}/policy`)
  await page.getByRole('button', { name: 'Menu', exact: true }).click()
  await expect(page.locator('[data-slot=sheet-content]')).toHaveAttribute('data-ui-surface', 'staff')
  expect(await fontSize(page.locator('[data-slot=sheet-content]'))).toBe(16)
})

test('policy save and preview show the same field error and focus', { tag: '@browser' }, async ({ page }) => {
  await page.goto(`/manage/${fixture().slug}/policy`)
  const input = page.locator('#service_duration_minutes')
  const error = page.locator('#service_duration_minutes-error')
  await input.fill('0')
  await page.getByRole('button', { name: 'Save', exact: true }).click()
  await expect(input).toHaveAttribute('aria-invalid', 'true')
  await expect(input).toBeFocused()
  const message = await error.innerText()
  await input.fill('30')
  await expect(error).toHaveCount(0)
  await input.fill('0')
  await page.getByRole('button', { name: 'Preview', exact: true }).click()
  await expect(input).toBeFocused()
  await expect(error).toHaveText(message)
})

test('date range and cycle actions align with controls even after field errors', async ({ page }) => {
  const d = fixture()
  await page.setViewportSize({ width: 1024, height: 768 })
  await page.goto(`/manage/${d.slug}/statistics`)
  const check = async () => {
    const row = page.locator('.control-row').first()
    const positions = await row.locator('input, button').evaluateAll((els) => els.map((el) => el.getBoundingClientRect().top))
    expect(Math.max(...positions) - Math.min(...positions)).toBeLessThanOrEqual(1)
  }
  await settle(page)
  await check()
  await page.locator('#statistics-from').fill('2030-02-01')
  await page.locator('#statistics-to').fill('2030-01-01')
  await page.locator('.control-row button').click()
  await expect(page.locator('#statistics-to-error')).toBeVisible()
  await check()
  await page.goto(`/manage/${d.slug}/schedule`)
  await settle(page)
  await check()
  await page.locator('#cycle-n').fill('0')
  await page.getByRole('button', { name: 'Preview', exact: true }).click()
  await expect(page.locator('#cycle-n-error')).toBeVisible()
  await expect(page.locator('#cycle-n')).toBeFocused()
  await check()
})

test('enlarged staff text reflows and leaves narrow save bars in document flow', async ({ page }) => {
  await page.setViewportSize({ width: 375, height: 667 })
  await page.goto(`/manage/${fixture().slug}/policy`)
  await settle(page)
  await page.evaluate(() => { document.documentElement.style.fontSize = '200%' })
  await expect.poll(() => page.evaluate(() => getComputedStyle(document.documentElement).fontSize)).toBe('32px')
  expect(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)).toBe(false)
  expect(await page.locator('[data-save-bar]').evaluate((el) => getComputedStyle(el).position)).toBe('static')
  expect(await page.locator('main').evaluate(formControlLayoutIssues)).toEqual([])
})

test('reopened leave confirmation stays above its overlay during transitions', async ({ page }) => {
  const base = `/manage/${fixture().slug}`
  await page.goto(`${base}/policy`)
  await page.locator('#service_duration_minutes').fill('45')
  const dialog = page.getByRole('alertdialog')
  for (let attempt = 0; attempt < 3; attempt++) {
    await page.locator(`a[href="${base}/schedule"]:visible`).click()
    await expect(dialog).toBeVisible()
    const overlay = page.locator('[data-slot=alert-dialog-overlay]')
    expect(await dialog.evaluate((el) => Number(getComputedStyle(el).zIndex)))
      .toBeGreaterThan(await overlay.evaluate((el) => Number(getComputedStyle(el).zIndex)))
    await dialog.getByRole('button', { name: 'Keep editing', exact: true }).click()
  }
  await expect(page.locator('#service_duration_minutes')).toHaveValue('45')
})
