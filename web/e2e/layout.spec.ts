// Layout checks for every page on phones, including small and low-density
// screens (see the projects in playwright.config.ts). Data is created
// through the API so each page has realistic content; then every page is
// opened in selected language/theme combinations and checked for horizontal
// overflow, content leaving the screen, clipped text, small tap targets and small text. Every
// dialog a page can open is checked the same way, and every page and
// dialog is checked with axe for WCAG A/AA problems (contrast, labels,
// ARIA). Daily runs sample all languages and both themes; test:e2e:full
// expands the layout matrix for release checks and broad UI changes.
import AxeBuilder from '@axe-core/playwright'
import { expect, test, type Browser, type Page, type TestInfo } from '@playwright/test'
import { readFileSync } from 'node:fs'
import { SEED_FILE, type Seed } from './seed'
import { formControlLayoutIssues } from './form-controls'

const LOCALES = ['en', 'ja', 'zh-Hans', 'zh-Hant'] as const
const SCHEMES = ['light', 'dark'] as const

// Interactive controls must be at least this tall and wide (CSS pixels).
// Customer pages follow the elderly-friendly target; the back office is
// used by staff and may be slightly denser.
const MIN_TAP_CUSTOMER = 44
const MIN_TAP_STAFF = 44
const MIN_FONT_CUSTOMER = 16
const MIN_FONT_STAFF = 12

type Issue = string

/** Runs the layout checks in the page and returns human-readable issues. */
async function layoutIssues(page: Page, minTap: number, minFont: number, scope?: string): Promise<Issue[]> {
  const inputIssues = await page.locator(scope ?? 'body').evaluate(formControlLayoutIssues)
  const issues = await page.evaluate(
    ({ minTap, minFont, scope }) => {
      const issues: string[] = []
      const vw = document.documentElement.clientWidth
      const root = (scope ? document.querySelector(scope) : document.body) as HTMLElement | null
      if (!root) return [`scope ${scope} not found`]

      const describe = (el: Element) => {
        const text = (el.textContent ?? '').trim().replace(/\s+/g, ' ').slice(0, 40)
        const label = el.getAttribute('aria-label')
        const tag = el.tagName.toLowerCase()
        const cls = (el.getAttribute('class') ?? '').split(' ').slice(0, 3).join('.')
        return `<${tag}${cls ? '.' + cls : ''}> "${label ?? text}"`
      }
      const visible = (el: Element) => {
        const s = getComputedStyle(el)
        if (s.visibility === 'hidden' || s.display === 'none' || Number(s.opacity) === 0) return false
        const r = el.getBoundingClientRect()
        return r.width > 0 && r.height > 0
      }
      const srOnly = (el: Element) => {
        for (let p: Element | null = el; p; p = p.parentElement) if (p.classList.contains('sr-only')) return true
        return false
      }

      if (!scope && document.documentElement.scrollWidth > vw + 1) {
        issues.push(`page scrolls horizontally: ${document.documentElement.scrollWidth} > ${vw}`)
      }

      const all = Array.from(root.querySelectorAll('*'))
      for (const el of all) {
        if (!visible(el) || srOnly(el)) continue
        const r = el.getBoundingClientRect()
        if (r.right > vw + 1 || r.left < -1) {
          issues.push(`off screen (${Math.round(r.left)}..${Math.round(r.right)} of ${vw}): ${describe(el)}`)
        }
      }

      // These screens have vertical scrollers, but no intentional horizontal
      // scrollers. overflow-y:auto also computes overflow-x:auto, so checking
      // only page width misses content spilling inside a dialog.
      for (const el of [root, ...all]) {
        if (!(el instanceof HTMLElement) || !visible(el) || srOnly(el)) continue
        if (el.matches('input, textarea, select')) continue
        const s = getComputedStyle(el)
        if (['auto', 'scroll', 'hidden', 'clip'].includes(s.overflowX) && el.scrollWidth > el.clientWidth + 1) {
          issues.push(`content overflows horizontally (${el.scrollWidth} > ${el.clientWidth}): ${describe(el)}`)
        }
      }

      // SVG labels can be inside the page but still clipped by their own chart.
      for (const el of Array.from(root.querySelectorAll<SVGTextElement>('[data-slot=chart] svg.recharts-surface text'))) {
        if (!visible(el)) continue
        const chart = el.closest('svg')!.getBoundingClientRect()
        const label = el.getBoundingClientRect()
        if (label.left < chart.left - 1 || label.right > chart.right + 1 || label.top < chart.top - 1 || label.bottom > chart.bottom + 1) {
          issues.push(`clipped chart label: ${describe(el)}`)
        }
      }

      // Tap targets. Inline links inside running text are exempt.
      const controls = Array.from(root.querySelectorAll<HTMLElement>(
        'button, a[href], input:not([type=hidden]), select, textarea, [role=tab], [role=checkbox], [role=switch], [role=combobox]',
      ))
      for (const el of controls) {
        if (!visible(el) || srOnly(el)) continue
        if (el.tagName === 'A' && getComputedStyle(el).display === 'inline') continue
        if (
          (el instanceof HTMLInputElement && (el.type === 'checkbox' || el.type === 'radio')) ||
          el.getAttribute('role') === 'checkbox'
        ) {
          // Checkboxes are small; their label must make the target big.
          const label = el.closest('label')
          const r = (label ?? el).getBoundingClientRect()
          if (r.height < minTap) issues.push(`small tap target ${Math.round(r.width)}x${Math.round(r.height)}: ${describe(label ?? el)}`)
          continue
        }
        const r = el.getBoundingClientRect()
        if (r.height < minTap - 0.5 || r.width < minTap - 0.5) {
          issues.push(`small tap target ${Math.round(r.width)}x${Math.round(r.height)}: ${describe(el)}`)
        }
      }

      // Text size of visible text nodes.
      const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT)
      const seen = new Set<Element>()
      for (let n = walker.nextNode(); n; n = walker.nextNode()) {
        const el = n.parentElement
        if (!el || seen.has(el) || !n.textContent?.trim() || !visible(el) || srOnly(el)) continue
        seen.add(el)
        const size = parseFloat(getComputedStyle(el).fontSize)
        if (size < minFont - 0.1) issues.push(`small text ${size}px: ${describe(el)}`)
      }

      // Dialogs must fit the screen; long content scrolls inside.
      for (const dlg of Array.from(document.querySelectorAll<HTMLElement>('[role=dialog], [role=alertdialog]'))) {
        if (!visible(dlg)) continue
        const r = dlg.getBoundingClientRect()
        const vh = window.innerHeight
        if (r.left < -1 || r.right > vw + 1) issues.push(`dialog wider than screen: ${Math.round(r.width)} > ${vw}`)
        const oy = getComputedStyle(dlg).overflowY
        const scrolls = oy === 'auto' || oy === 'scroll'
        if ((r.top < -1 || r.bottom > vh + 1) && !scrolls) {
          issues.push(`dialog taller than screen and not scrollable: ${Math.round(r.height)} > ${vh}`)
        }
        // Content that spills out of the dialog cannot be reached.
        if (dlg.scrollHeight > dlg.clientHeight + 1 && !scrolls) {
          issues.push(`dialog content overflows and cannot scroll: ${dlg.scrollHeight} > ${dlg.clientHeight}`)
        }
      }
      return Array.from(new Set(issues))
    },
    { minTap, minFont, scope },
  )
  return [...inputIssues, ...issues]
}

async function settle(page: Page) {
  await page.waitForLoadState('networkidle')
  // Wait until no loading indicator remains.
  await expect(page.locator('.motion-safe\\:animate-pulse, .animate-spin')).toHaveCount(0)
  await page.evaluate(() => document.fonts.ready)
  await waitForAnimations(page)
}

/** Waits until open/close animations (dialogs sliding or zooming in) end. */
async function waitForAnimations(page: Page) {
  await page.waitForFunction(() => document.getAnimations().every((a) => a.playState !== 'running' || a.effect?.getTiming().iterations === Infinity))
}

/** WCAG 2.1 A/AA violations found by axe, optionally inside one element. */
async function axeIssues(page: Page, scope?: string): Promise<string[]> {
  let b = new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa'])
  if (scope) b = b.include(scope)
  const { violations } = await b.analyze()
  return violations.flatMap((v) =>
    v.nodes.map((n) => `${v.id}: ${n.target.join(' ')} — ${(n.failureSummary ?? '').replace(/\s+/g, ' ').trim()}`),
  )
}

async function shot(page: Page, info: TestInfo, name: string) {
  await page.screenshot({ path: info.outputPath(`${name}.png`), fullPage: true })
}

/** Checks the page, then every dialog its visible triggers open. */
async function checkPage(page: Page, info: TestInfo, name: string, kind: 'customer' | 'staff', openDialogs = true) {
  const minTap = kind === 'customer' ? MIN_TAP_CUSTOMER : MIN_TAP_STAFF
  const minFont = kind === 'customer' ? MIN_FONT_CUSTOMER : MIN_FONT_STAFF
  await settle(page)
  await shot(page, info, name)
  const issues = await layoutIssues(page, minTap, minFont)
  expect.soft(issues, `${name}: layout`).toEqual([])
  expect.soft(await axeIssues(page), `${name}: accessibility`).toEqual([])

  const dateFilter = page.locator('[data-date-filter]')
  if (await dateFilter.count()) {
    await dateFilter.getByRole('button').nth(1).click()
    await settle(page)
    await expect(dateFilter.locator('input[type=date]')).toBeVisible()
    expect.soft(await layoutIssues(page, minTap, minFont), `${name}: date filter layout`).toEqual([])
    expect.soft(await axeIssues(page), `${name}: date filter accessibility`).toEqual([])
    await shot(page, info, `${name}-date`)
    await dateFilter.getByRole('button').first().click()
    await settle(page)
  }

  if (!openDialogs) return
  const triggers = page.locator('[aria-haspopup=dialog]:visible')
  const count = await triggers.count()
  for (let i = 0; i < count; i++) {
    const trigger = triggers.nth(i)
    if (await trigger.isDisabled()) continue
    const label = ((await trigger.getAttribute('aria-label')) ?? (await trigger.innerText())).trim().slice(0, 30)
    await trigger.click()
    const dlg = page.locator('[role=dialog], [role=alertdialog]').last()
    await expect(dlg).toBeVisible()
    await waitForAnimations(page)
    if (await dlg.locator('#new-booking-customer').count()) {
      await dlg.locator('#new-booking-customer').selectOption(data().customerId)
      await settle(page)
    }
    const filter = dlg.locator('[data-date-filter]')
    if (await filter.count()) {
      await filter.getByRole('button').nth(1).click()
      await settle(page)
    }
    await shot(page, info, `${name}--dialog-${i}`)
    const dIssues = await layoutIssues(page, minTap, minFont, '[role=dialog], [role=alertdialog]')
    expect.soft(dIssues, `${name}: dialog "${label}"`).toEqual([])
    expect.soft(await axeIssues(page, '[role=dialog], [role=alertdialog]'), `${name}: dialog "${label}" accessibility`).toEqual([])
    // Special dates start as a day off; their working-hours editor only
    // exists after changing that state, so opening the dialog is not enough.
    if (await dlg.locator('#ov-date').count()) {
      await dlg.getByRole('checkbox').uncheck()
      await expect(dlg.locator('input[type=time]')).toHaveCount(2)
      await dlg.locator('input[type=time]').first().fill('13:05')
      await dlg.locator('input[type=time]').last().fill('23:55')
      await shot(page, info, `${name}--dialog-${i}-working`)
      expect.soft(await layoutIssues(page, minTap, minFont, '[role=dialog]'), `${name}: special-date working hours`).toEqual([])
      expect.soft(await axeIssues(page, '[role=dialog]'), `${name}: special-date working hours accessibility`).toEqual([])
    }
    await page.keyboard.press('Escape')
    await expect(page.locator('[role=dialog], [role=alertdialog]')).toHaveCount(0)
  }
}


let cached: Seed | undefined
/** The data created by the seed project (read lazily: it runs after collection). */
function data(): Seed {
  return (cached ??= JSON.parse(readFileSync(SEED_FILE, 'utf8')) as Seed)
}

/**
 * A browser context in the given language. The language is remembered in
 * the browser only, never saved on the shared seeded accounts, so tests in
 * different languages can run in parallel.
 */
async function newContext(
  browser: Browser,
  info: TestInfo,
  locale: string,
  scheme: string,
  storageState?: Seed['staffState'],
) {
  const ctx = await browser.newContext({ ...info.project.use, colorScheme: scheme as 'light' | 'dark', storageState })
  await ctx.addInitScript((l) => localStorage.setItem('rm_locale', l), locale)
  return ctx
}

// Back office pages, each checked in its own test so they spread over workers.
const STAFF_PAGES: [string, (d: Seed) => string][] = [
  ['platform-accounts', () => '/platform/accounts'],
  ['platform-instances', () => '/platform/instances'],
  ['platform-settings', () => '/platform/settings'],
  ['profile', () => '/account/profile'],
  ['dashboard', (d) => `/manage/${d.slug}`],
  ['statistics', (d) => `/manage/${d.slug}/statistics?basis=created_at`],
  ['appointments-upcoming', (d) => `/manage/${d.slug}/appointments`],
  ['customers-pending', (d) => `/manage/${d.slug}/customers?status=pending`],
  ['customers-approved', (d) => `/manage/${d.slug}/customers?status=approved`],
  ['customers-all', (d) => `/manage/${d.slug}/customers?status=all`],
  ['customer-detail', (d) => `/manage/${d.slug}/customers/${d.customerId}`],
  ['schedule', (d) => `/manage/${d.slug}/schedule`],
  ['policy', (d) => `/manage/${d.slug}/policy`],
  ['settings', (d) => `/manage/${d.slug}/settings`],
  ['audit', (d) => `/manage/${d.slug}/audit`],
]

for (const { scheme, locale } of SCHEMES.flatMap((scheme) => LOCALES.map((locale) => ({ scheme, locale })))) {
  // Screenshot names carry the scheme only for dark, keeping light names as before.
  const sfx = scheme === 'dark' ? '-dark' : ''

  test.describe(`${locale}, ${scheme}`, () => {
    test('customer pages fit small screens', async ({ browser }, info) => {
      const d = data()
      const ctx = await newContext(browser, info, locale, scheme, d.customerState)
      const page = await ctx.newPage()
      const s = d.slug

      await page.goto(`/i/${s}/booking`)
      await expect(page.locator('html')).toHaveAttribute('lang', locale)
      await checkPage(page, info, `${locale}${sfx}-booking-home`, 'customer')

      await page.goto(`/i/${s}/booking/times`)
      await checkPage(page, info, `${locale}${sfx}-booking-times`, 'customer')

      // Open the confirmation for the first free time from the times page.
      await page.locator('main section button').first().click()
      await expect(page).toHaveURL(/\/booking\/confirm/)
      await checkPage(page, info, `${locale}${sfx}-booking-confirm`, 'customer')

      await page.goto(`/i/${s}/booking/done/${d.appointmentId}`)
      await checkPage(page, info, `${locale}${sfx}-booking-done`, 'customer')
      await ctx.close()
    })

    test('pages without a session fit small screens', async ({ browser }, info) => {
      const d = data()
      const anon = await newContext(browser, info, locale, scheme)
      const p = await anon.newPage()
      await p.goto(d.invite)
      await checkPage(p, info, `${locale}${sfx}-register`, 'customer')
      // Submit empty to show the validation message.
      await p.locator('button[type=submit]').click()
      await checkPage(p, info, `${locale}${sfx}-register-invalid`, 'customer', false)
      await p.goto(`/i/${d.slug}/register/done`)
      await checkPage(p, info, `${locale}${sfx}-register-done`, 'customer')
      await p.goto(`/i/${d.slug}/link-invalid`)
      await checkPage(p, info, `${locale}${sfx}-link-invalid`, 'customer')
      await p.goto('/no/such/page')
      await checkPage(p, info, `${locale}${sfx}-not-found`, 'customer')
      await p.goto('/account/login')
      await checkPage(p, info, `${locale}${sfx}-login`, 'staff')
      await anon.close()
    })

    for (const [name, url] of STAFF_PAGES) {
      test(`back office ${name} fits small screens`, async ({ browser }, info) => {
        const ctx = await newContext(browser, info, locale, scheme, data().staffState)
        const page = await ctx.newPage()
        await page.goto(url(data()))
        await expect(page.locator('html')).toHaveAttribute('lang', locale)
        if (name === 'appointments-upcoming') {
          await settle(page)
          await page.getByRole('tab').nth(1).click()
        }
        await checkPage(page, info, `${locale}${sfx}-${name}`, 'staff')
        if (name === 'schedule') {
          await page.locator('#day0-trigger').click()
          await page.locator('#day0-panel summary').click()
          await checkPage(page, info, `${locale}${sfx}-${name}-expanded`, 'staff', false)
        }
        if (name === 'statistics') {
          await page.locator('details > summary').click()
          await checkPage(page, info, `${locale}${sfx}-${name}-expanded`, 'staff', false)
        }
        await ctx.close()
      })
    }

    test('back office account-home fits small screens', async ({ browser }, info) => {
      // Only operators have "My calendars"; the super admin is sent to all calendars.
      const ctx = await newContext(browser, info, locale, scheme, data().operatorState)
      const page = await ctx.newPage()
      await page.goto('/account')
      await expect(page.locator('html')).toHaveAttribute('lang', locale)
      await checkPage(page, info, `${locale}${sfx}-account-home`, 'staff')
      await ctx.close()
    })

    test('back office policy preview fits small screens', async ({ browser }, info) => {
      const ctx = await newContext(browser, info, locale, scheme, data().staffState)
      const page = await ctx.newPage()
      await page.goto(`/manage/${data().slug}/policy`)
      await settle(page)
      // Previews render tables of times.
      const PREVIEW = { en: 'Preview', ja: 'プレビュー', 'zh-Hans': '预览', 'zh-Hant': '預覽' }[locale]
      await page.getByRole('button', { name: new RegExp(PREVIEW) }).first().click()
      await checkPage(page, info, `${locale}${sfx}-policy-preview`, 'staff', false)
      await ctx.close()
    })
  })
}
