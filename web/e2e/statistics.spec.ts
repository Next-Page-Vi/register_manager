import { expect, test, type Page } from '@playwright/test'
import { readFileSync } from 'node:fs'
import type { Schemas } from '../src/api/client'
import { seed, SEED_FILE, type Seed } from './seed'

const fixture = () => JSON.parse(readFileSync(SEED_FILE, 'utf8')) as Seed

// All four statuses must have visible bars to catch category-label placement.
const chartData: Schemas['Statistics'] = {
  basis: 'created_at', from: '2026-10-12', to: '2026-10-18', timezone: 'Europe/London', granularity: 'day',
  summary: { booked: 35, completed: 28, cancelled: 6, no_show: 3, total: 72, unique_customers: 41 },
  series: Array.from({ length: 7 }, (_, day) => ({
    from: `2026-10-${12 + day}`, to: `2026-10-${12 + day}`,
    counts: { booked: 2 + day, completed: 7 - day, cancelled: day % 3, no_show: day % 2 },
  })),
}

for (const locale of ['en', 'zh-Hans', 'zh-Hant', 'ja']) {
  for (const colorScheme of ['light', 'dark'] as const) {
    test(`chart axes and category labels align in ${locale}, ${colorScheme}`, { tag: '@browser' }, async ({ page, context }, info) => {
      const data = fixture()
      await context.addCookies(data.staffState.cookies)
      await page.addInitScript((language) => localStorage.setItem('rm_locale', language), locale)
      await page.emulateMedia({ colorScheme })
      await page.route(`**/api/v1/instances/${data.instanceId}/statistics?**`, (route) => route.fulfill({ json: chartData }))
      await page.goto(`/manage/${data.slug}/statistics?basis=created_at`)
      const charts = page.locator('[data-slot=chart]')
      await expect(charts).toHaveCount(2)

      for (const width of [1440, 320]) {
        await page.setViewportSize({ width, height: 1000 })
        // Allow responsive sizing and automatic axis measurement to settle.
        await expect.poll(() => charts.evaluateAll((elements) => elements.every((element) => {
          const surface = element.querySelector('svg.recharts-surface')!
          return Math.abs(surface.getBoundingClientRect().width - element.getBoundingClientRect().width) < 1
        }))).toBe(true)
        for (const chart of await charts.all()) {
          await expect(chart.locator('.recharts-cartesian-axis-line')).toHaveCount(2)
        }
        const geometry = await charts.evaluateAll((elements) => elements.map((element) => {
          const svg = element.querySelector('svg.recharts-surface')!.getBoundingClientRect()
          const xAxis = element.querySelector<SVGLineElement>('.recharts-xAxis .recharts-cartesian-axis-line')!
          const yAxis = element.querySelector<SVGLineElement>('.recharts-yAxis .recharts-cartesian-axis-line')!
          const labels = Array.from(element.querySelectorAll<SVGTextElement>('.recharts-cartesian-axis-tick-value'))
          const xLabels = Array.from(element.querySelectorAll<SVGTextElement>('.recharts-xAxis-tick-labels .recharts-cartesian-axis-tick-value'), (label) => label.getBoundingClientRect())
          return {
            top: svg.top, height: svg.height,
            plotTop: yAxis.y1.baseVal.value, plotBottom: xAxis.y1.baseVal.value,
            strokes: [xAxis, yAxis].map((axis) => getComputedStyle(axis).stroke),
            clipped: labels.filter((label) => {
              const rect = label.getBoundingClientRect()
              return rect.left < svg.left - 1 || rect.right > svg.right + 1 || rect.top < svg.top - 1 || rect.bottom > svg.bottom + 1
            }).map((label) => label.textContent),
            overlapping: xLabels.slice(1).some((label, index) => label.left < xLabels[index].right),
          }
        }))
        expect(geometry[0].height).toBe(geometry[1].height)
        expect(geometry[0].plotTop).toBe(geometry[1].plotTop)
        expect(geometry[0].plotBottom).toBe(geometry[1].plotBottom)
        expect(geometry[0].strokes).toEqual(geometry[1].strokes)
        if (width === 1440) expect(Math.abs(geometry[0].top - geometry[1].top)).toBeLessThanOrEqual(1)
        for (const chart of geometry) {
          expect(chart.strokes.every((stroke) => stroke !== 'none')).toBe(true)
          expect(chart.clipped).toEqual([])
          expect(chart.overlapping).toBe(false)
        }
        const categories = await charts.nth(1).evaluate((element) => {
          const axis = element.querySelector('.recharts-yAxis .recharts-cartesian-axis-line')!.getBoundingClientRect()
          const labels = Array.from(element.querySelectorAll<SVGTextElement>('.recharts-yAxis-tick-labels .recharts-cartesian-axis-tick-value'))
          const bars = Array.from(element.querySelectorAll<SVGPathElement>('.recharts-bar-rectangle path'))
          return labels.map((label, index) => ({
            gap: axis.left - label.getBoundingClientRect().right,
            centerOffset: Number(label.getAttribute('y')) - (Number(bars[index].getAttribute('y')) + Number(bars[index].getAttribute('height')) / 2),
          }))
        })
        expect(categories).toHaveLength(4)
        for (const category of categories) {
          expect(category.gap).toBeGreaterThanOrEqual(7)
          expect(Math.abs(category.centerOffset)).toBeLessThanOrEqual(1)
        }
        expect(await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)).toBeLessThanOrEqual(1)
        await page.locator('[data-slot=card]').filter({ has: charts.first() }).screenshot({ path: info.outputPath(`charts-${width}.png`) })
      }
    })
  }
}

async function navigate(page: Page, path: string) {
  await page.evaluate((path) => {
    history.pushState({}, '', path)
    dispatchEvent(new PopStateEvent('popstate'))
  }, path)
}

test('statistics filters, history, data view and booking changes work together', { tag: '@browser' }, async ({ page, context, playwright, baseURL }, info) => {
  const data = await seed(playwright, baseURL!, `stats-${Date.now().toString(36)}-${info.project.name}`)
  await context.addCookies(data.operatorState.cookies)
  await page.goto(`/manage/${data.slug}/statistics`)
  await expect(page).toHaveURL(/basis=starts_at/)
  const basis = page.getByLabel('Group appointments by')
  await basis.selectOption('created_at')
  await expect(page.locator('[data-statistic=total] dd')).toHaveText('2')
  await expect(page.locator('[data-statistic=customers] dd')).toHaveText('1')
  await expect(page.locator('[data-slot=chart] svg.recharts-surface')).toHaveCount(2)
  await page.getByRole('button', { name: 'Last 7 days', exact: true }).click()
  const selected = page.url()
  await page.reload()
  await expect(basis).toHaveValue('created_at')
  await expect(page.getByRole('button', { name: 'Last 7 days', exact: true })).toHaveAttribute('aria-pressed', 'true')
  await expect(page.locator('[data-statistic=total] dd')).toHaveText('2')
  await page.screenshot({ path: info.outputPath('statistics.png'), fullPage: true })
  await page.locator('details > summary').click()
  await expect(page.locator('details')).toHaveAttribute('open', '')
  await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth)).toBeLessThanOrEqual(1)

  await page.getByLabel('From date', { exact: true }).fill('2000-01-01')
  await page.getByLabel('To date', { exact: true }).fill('2001-01-02')
  await page.getByRole('button', { name: 'Apply dates' }).click()
  await expect(page.getByText('Choose an end date on or after the start date, with no more than 366 days in total.')).toBeVisible()
  expect(page.url()).toBe(selected)
  await page.getByLabel('To date', { exact: true }).fill('2000-01-02')
  await page.getByRole('button', { name: 'Apply dates' }).click()
  await expect(page.locator('[data-statistic=total] dd')).toHaveText('0')
  await expect(page.getByText('No appointments in this period. Try another date range.')).toBeVisible()
  await expect(page.getByText('No data', { exact: true })).toBeVisible()
  await page.goBack()
  await expect(page).toHaveURL(selected)
  await expect(page.getByLabel('From date', { exact: true })).not.toHaveValue('2000-01-01')
  await expect(page.locator('[data-statistic=total] dd')).toHaveText('2')

  // Navigate inside the SPA to retain QueryClient and exercise mutation invalidation.
  await navigate(page, `/manage/${data.slug}/appointments`)
  await page.getByRole('tab', { name: 'Upcoming', exact: true }).click()
  await page.getByRole('button', { name: 'Cancel appointment', exact: true }).first().click()
  await page.getByRole('alertdialog').getByRole('button', { name: 'Cancel appointment', exact: true }).click()
  await expect(page.getByRole('alertdialog')).toHaveCount(0)
  await page.goBack()
  await expect(page.locator('[data-statistic=cancelled] dd')).toHaveText('1')
  await expect(page.locator('[data-statistic=total] dd')).toHaveText('2')
})

test('statistics errors can be retried and bad date links can be corrected', async ({ page, context }) => {
  const data = fixture()
  await context.addCookies(data.staffState.cookies)
  await page.route('**/statistics?**', async (route) => {
    if (route.request().resourceType() !== 'fetch') return route.continue()
    await route.fulfill({ status: 500, contentType: 'application/json', body: JSON.stringify({ code: 'internal_error', status: 500 }) })
  })
  await page.goto(`/manage/${data.slug}/statistics?basis=created_at`)
  await expect(page.getByRole('alert')).toBeVisible()
  await page.unroute('**/statistics?**')
  await page.getByRole('button', { name: 'Try again' }).click()
  await expect(page.locator('[data-statistic=total] dd')).toHaveText('2')
  await page.goto(`/manage/${data.slug}/statistics?from=bad&to=2026-01-01&basis=created_at`)
  await expect(page.getByRole('alert')).toBeVisible()
  await page.getByRole('button', { name: 'Last 30 days' }).click()
  await expect(page.locator('[data-statistic=total] dd')).toHaveText('2')
})

test('customer pages do not load statistics or chart code', async ({ page }) => {
  const scripts: string[] = []
  page.on('request', (request) => {
    if (request.resourceType() === 'script') scripts.push(request.url())
  })
  await page.goto(fixture().invite)
  await expect(page.getByRole('heading').first()).toBeVisible()
  await page.waitForLoadState('networkidle')
  expect(scripts.some((url) => /Statistics-|recharts|chart-/i.test(url))).toBe(false)
})
