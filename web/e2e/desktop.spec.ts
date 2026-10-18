// Desktop-only layout and native browser interactions.
import { expect, test } from '@playwright/test'
import { readFileSync } from 'node:fs'
import { SEED_FILE, type Seed } from './seed'

const fixture = () => JSON.parse(readFileSync(SEED_FILE, 'utf8')) as Seed

test.beforeEach(async ({ context }) => {
  await context.addCookies(fixture().staffState.cookies)
})

test('reloading a dirty form invokes native protection and cancelling keeps the draft', async ({ page }) => {
  await page.goto(`/manage/${fixture().slug}/policy`)
  await page.locator('#service_duration_minutes').fill('45')
  const dialog = page.waitForEvent('dialog')
  await page.evaluate(() => { setTimeout(() => location.reload(), 0) })
  const prompt = await dialog
  expect(prompt.type()).toBe('beforeunload')
  await prompt.dismiss()
  await expect(page.locator('#service_duration_minutes')).toHaveValue('45')
})

test('policy inputs and unlimited controls align across columns, including validation errors', async ({ page }) => {
  const data = fixture()
  for (const width of [768, 1280, 1440]) {
    await page.setViewportSize({ width, height: 900 })
    await page.goto(`/manage/${data.slug}/policy`)
    await page.locator('#service_duration_minutes').waitFor()
    const checkInputs = async () => {
      const ids = width >= 1280 ? ['service_duration_minutes', 'slot_step_minutes', 'buffer_minutes'] : ['service_duration_minutes', 'slot_step_minutes']
      const tops = await Promise.all(ids.map((id) => page.locator(`#${id}`).evaluate((el) => el.getBoundingClientRect().top)))
      expect(Math.max(...tops) - Math.min(...tops)).toBeLessThanOrEqual(1)
      const checks = page.getByRole('group', { name: 'Customer and booking limits', exact: true }).getByRole('checkbox')
      const positions = await checks.evaluateAll((els) => els.map((el) => el.getBoundingClientRect().top))
      expect(Math.abs(positions[0] - positions[1])).toBeLessThanOrEqual(1)
      expect(Math.abs(positions[2] - positions[3])).toBeLessThanOrEqual(1)
    }
    await checkInputs()
    await page.locator('#service_duration_minutes').fill('0')
    await page.getByRole('button', { name: 'Preview', exact: true }).click()
    await expect(page.locator('#service_duration_minutes')).toHaveAttribute('aria-invalid', 'true')
    await checkInputs()
  }
})

test('page and header edges stay fixed across routes and modal scroll locks', async ({ playwright, baseURL }) => {
  const browser = await playwright.chromium.launch({
    ignoreDefaultArgs: ['--hide-scrollbars'],
    args: ['--disable-features=OverlayScrollbar,FluentOverlayScrollbar'],
  })
  try {
    const context = await browser.newContext({
      baseURL, storageState: fixture().staffState,
      viewport: { width: 1600, height: 900 }, locale: 'en-GB', reducedMotion: 'reduce',
    })
    const page = await context.newPage()
    const edges = () => page.evaluate(() => ({
      main: document.querySelector('main')!.getBoundingClientRect().x,
      header: document.querySelector('header > div')!.getBoundingClientRect().x,
      profile: document.querySelector<HTMLAnchorElement>('header a[href="/account/profile"]')!.getBoundingClientRect().width,
    }))
    let reference: Awaited<ReturnType<typeof edges>> | null = null
    for (const path of [`/manage/${fixture().slug}/schedule`, '/account/profile', '/platform/settings']) {
      await page.goto(path)
      await page.waitForLoadState('networkidle')
      const current = await edges()
      if (reference === null) reference = current
      expect(current).toEqual(reference)
      await page.getByRole('button', { name: 'Language: English', exact: true }).click()
      await expect(page.getByRole('dialog')).toBeVisible()
      await expect.poll(edges).toEqual(reference)
      await page.keyboard.press('Escape')
      await expect(page.getByRole('dialog')).toHaveCount(0)
      expect(await edges()).toEqual(reference)
    }
  } finally {
    await browser.close()
  }
})
