import { expect, test, type Page } from '@playwright/test'
import AxeBuilder from '@axe-core/playwright'
import { readFileSync } from 'node:fs'
import { SEED_FILE, type Seed } from './seed'
import { formControlLayoutIssues } from './form-controls'
import type { components } from '../src/api/schema'

type Schemas = components['schemas']
const fixture = () => JSON.parse(readFileSync(SEED_FILE, 'utf8')) as Seed
const failure = { status: 500, json: { code: 'unknown_error', status: 500 } }
const gate = () => {
  let release!: () => void
  const promise = new Promise<void>((resolve) => { release = resolve })
  return { promise, release }
}
async function navigate(page: Page, path: string) {
  await expect(page.locator('header').first()).toBeVisible()
  const wide = await page.evaluate(() => matchMedia('(min-width: 64rem)').matches)
  if (!wide) {
    await expect(page.locator('[data-slot="sheet-content"]')).toHaveCount(0)
    await page.getByRole('button', { name: 'Menu', exact: true }).click()
  }
  await page.locator(`a[href="${path}"]:visible`).first().click()
  await expect(page.locator('[data-slot="sheet-content"]')).toHaveCount(0)
}

test.beforeEach(async ({ context }) => { await context.addCookies(fixture().staffState.cookies) })

test('dirty policy protects navigation, preview, failed saves, pending saves and browser history', { tag: '@browser' }, async ({ page }) => {
  const data = fixture()
  const base = `/manage/${data.slug}`
  const endpoint = `/api/v1/instances/${data.instanceId}/policy`
  const initial: Schemas['Policy'] = await (await page.request.get(endpoint)).json()
  let fail = true
  const saving = gate()
  let started = false
  await page.route(`**${endpoint}`, async (route) => {
    if (route.request().method() !== 'PUT') return route.continue()
    if (fail) return route.fulfill(failure)
    started = true
    await saving.promise
    return route.fulfill({ json: { policy: route.request().postDataJSON() } })
  })
  await page.goto(`${base}/appointments`)
  await navigate(page, `${base}/policy`)
  const input = page.locator('#service_duration_minutes')
  await expect(input).toHaveValue(String(initial.service_duration_minutes))
  await input.fill('45')
  await expect(input).toHaveValue('45')
  await expect(page.getByText('Unsaved changes', { exact: true })).toBeVisible()
  await page.getByRole('button', { name: 'Preview', exact: true }).click()
  await expect(page.getByText('Unsaved changes', { exact: true })).toBeVisible()
  await navigate(page, `${base}/schedule`)
  const alert = page.getByRole('alertdialog')
  await expect(alert.getByRole('button', { name: 'Keep editing' })).toBeFocused()
  await page.keyboard.press('Escape')
  await expect(alert).toHaveCount(0)
  await expect(input).toHaveValue('45')
  await page.evaluate(() => history.back())
  await expect(alert).toBeVisible()
  await alert.getByRole('button', { name: 'Keep editing' }).click()
  await expect(page).toHaveURL(`${base}/policy`)
  await page.getByRole('button', { name: 'Save', exact: true }).click()
  await expect(page.locator('[data-save-bar] [role=alert]')).toBeVisible()
  await expect(input).toHaveValue('45')
  await expect(page.getByText('Unsaved changes', { exact: true })).toBeVisible()
  fail = false
  await page.getByRole('button', { name: 'Save', exact: true }).click()
  await expect.poll(() => started).toBe(true)
  await expect(input).toBeDisabled()
  await navigate(page, `${base}/schedule`)
  await expect(alert.getByRole('button', { name: 'Discard and leave' })).toBeDisabled()
  await alert.getByRole('button', { name: 'Keep editing' }).click()
  saving.release()
  await expect(page.getByText('Saved.', { exact: true })).toBeVisible()
  await expect(page.getByText('Unsaved changes', { exact: true })).toHaveCount(0)
  await navigate(page, `${base}/schedule`)
  await expect(alert).toHaveCount(0)
  await navigate(page, `${base}/policy`)
  await expect(input).toHaveValue(String(initial.service_duration_minutes))
  await input.fill('45')
  await navigate(page, `${base}/schedule`)
  await alert.getByRole('button', { name: 'Discard and leave' }).click()
  await expect(page).toHaveURL(`${base}/schedule`)
  await page.goBack()
  await expect(input).toHaveValue(String(initial.service_duration_minutes))
})

test('schedule and settings guard drafts; logout asks before sending and forced sign-in bypasses it', async ({ page }) => {
  const base = `/manage/${fixture().slug}`
  await page.goto(`${base}/schedule`)
  await page.locator('#cycle-anchor').fill('2027-01-04')
  await navigate(page, `${base}/settings`)
  await page.getByRole('button', { name: 'Discard and leave' }).click()
  const name = page.locator('#s-name')
  await name.fill('Draft clinic')
  let logouts = 0
  await page.route('**/api/v1/account/logout', (route) => { logouts++; return route.fulfill(failure) })
  await page.getByRole('button', { name: 'Sign out', exact: true }).click()
  await expect(page.getByRole('alertdialog')).toBeVisible()
  expect(logouts).toBe(0)
  await page.getByRole('button', { name: 'Keep editing' }).click()
  await expect(name).toHaveValue('Draft clinic')
  await page.getByRole('button', { name: 'Sign out', exact: true }).click()
  await page.getByRole('button', { name: 'Discard and leave' }).click()
  await expect.poll(() => logouts).toBe(1)
  await expect(name).toHaveValue('Draft clinic')
  await navigate(page, `${base}/appointments`)
  await expect(page.getByRole('alertdialog')).toBeVisible()
  await page.getByRole('button', { name: 'Keep editing' }).click()
  await page.evaluate(() => window.dispatchEvent(new CustomEvent('rm:unauthenticated', { detail: { kind: 'account' } })))
  await expect(page).toHaveURL(/\/account\/login\?next=/)
  await expect(page.getByRole('alertdialog')).toHaveCount(0)
})

test('operator time grid has equal columns, stable selection and aligned partial rows at all sizes', { tag: '@browser' }, async ({ page }) => {
  await page.emulateMedia({ reducedMotion: 'reduce' })
  const data = fixture()
  const endpoint = `/api/v1/instances/${data.instanceId}/availability`
  const makeSlots = (day: string, count: number) => Array.from({ length: count }, (_, i) => {
    const start = new Date(`${day}T09:00:00Z`)
    start.setUTCMinutes(i * 30)
    return { starts_at: start.toISOString(), ends_at: new Date(+start + 1800000).toISOString() }
  })
  await page.route(`**${endpoint}?*`, (route) => {
    const cursor = new URL(route.request().url()).searchParams.get('cursor')
    return route.fulfill({ json: { timezone: 'Europe/London', date_range: { from: '2030-01-15', to: '2030-02-15' }, items: cursor ? makeSlots('2030-01-17', 7) : [...makeSlots('2030-01-15', 11), ...makeSlots('2030-01-16', 7)], empty_reason: null, next_cursor: cursor ? null : 'next' } })
  })
  await page.goto(`/manage/${data.slug}/appointments`)
  await page.getByRole('button', { name: 'New appointment', exact: true }).click()
  const dialog = page.getByRole('dialog')
  await dialog.getByLabel('Customer', { exact: true }).selectOption(data.customerId)
  await dialog.getByRole('button', { name: 'Recommended times', exact: true }).click()
  const options = dialog.locator('[data-slot-grid] button')
  await expect(options).toHaveCount(18)
  for (const width of [320, 390, 768, 1440]) {
    await page.setViewportSize({ width, height: 900 })
    const geometry = () => options.evaluateAll((els) => els.map((el) => {
      const b = el as HTMLButtonElement
      return { x: b.offsetLeft, y: b.offsetTop, width: b.offsetWidth, height: b.offsetHeight }
    }))
    await expect.poll(() => dialog.evaluate(() => innerWidth)).toBe(width)
    await page.evaluate(() => new Promise<void>((resolve) => requestAnimationFrame(() => requestAnimationFrame(() => resolve()))))
    const before = await geometry()
    expect(Math.max(...before.map((b) => b.width)) - Math.min(...before.map((b) => b.width))).toBeLessThanOrEqual(1)
    expect(before[0].x).toBe(before[11].x)
    await options.filter({ hasText: /^9:00$/ }).first().click()
    expect(await geometry()).toEqual(before)
    await options.filter({ hasText: /^10:00$/ }).first().click()
    expect(await geometry()).toEqual(before)
  }
  await dialog.getByRole('button', { name: 'Show later times', exact: true }).click()
  await expect(options).toHaveCount(25)
  await page.setViewportSize({ width: 320, height: 800 })
  await page.evaluate(() => { document.documentElement.style.fontSize = '225%' })
  await dialog.getByRole('button', { name: 'Choose a date', exact: true }).click()
  await expect(dialog.getByLabel('Appointment date', { exact: true })).toBeVisible()
  await expect.poll(() => dialog.evaluate(formControlLayoutIssues)).toEqual([])
  await expect.poll(() => dialog.evaluate((el) => el.scrollWidth - el.clientWidth)).toBeLessThanOrEqual(1)
  expect((await new AxeBuilder({ page }).include('[role="dialog"]').analyze()).violations).toEqual([])
  await page.screenshot({ path: test.info().outputPath('operator-grid-320-double-text.png'), fullPage: true })
})

test('date browsing scopes paging, retains date after confirmation and isolates late responses', async ({ page, context }) => {
  const data = fixture()
  await context.addCookies(data.customerState.cookies)
  const endpoint = `/api/v1/customer/${data.slug}/availability`
  const late = gate()
  const started = gate()
  let fail = false
  await page.route(`**${endpoint}?*`, async (route) => {
    const params = new URL(route.request().url()).searchParams
    const date = params.get('date')
    if (date === '2030-01-16') { started.release(); await late.promise }
    if (fail) return route.fulfill(failure)
    const cursor = params.get('cursor')
    const day = date ?? '2030-01-15'
    const empty = day === '2030-01-17'
    return route.fulfill({ json: {
      timezone: 'Europe/London', date_range: { from: '2030-01-15', to: '2030-01-20' },
      items: empty ? [] : [{ starts_at: `${day}T${cursor ? '11' : '09'}:00:00Z`, ends_at: `${day}T${cursor ? '11' : '09'}:30:00Z` }],
      next_cursor: cursor || empty ? null : 'next', empty_reason: empty ? 'no_slots_on_date' : null,
    } })
  })
  await page.goto(`/i/${data.slug}/booking/times`)
  await page.getByRole('button', { name: 'Choose a date', exact: true }).click()
  await expect(page.getByLabel('Appointment date', { exact: true })).toHaveValue('2030-01-15')
  await page.getByRole('button', { name: 'More times on this day' }).click()
  await expect(page.getByRole('button', { name: '11:00', exact: true })).toBeVisible()
  await page.getByRole('button', { name: '9:00', exact: true }).click()
  await expect(page).toHaveURL(/confirm\?.*date=2030-01-15/)
  await page.getByRole('link', { name: 'Choose a different time', exact: true }).click()
  await expect(page.getByLabel('Appointment date', { exact: true })).toHaveValue('2030-01-15')
  await page.getByRole('button', { name: 'Next day', exact: true }).click()
  await started.promise
  await page.getByLabel('Appointment date', { exact: true }).fill('2030-01-17')
  await expect(page.getByText('No available times on this date. Please choose another day.', { exact: true })).toBeVisible()
  late.release()
  await expect(page.getByRole('button', { name: '9:00', exact: true })).toHaveCount(0)
  await page.getByLabel('Appointment date', { exact: true }).fill('2030-01-15')
  await expect(page.getByRole('button', { name: '11:00', exact: true })).toHaveCount(0)
  fail = true
  await page.getByLabel('Appointment date', { exact: true }).fill('2030-01-18')
  await expect(page.getByRole('button', { name: 'Try again', exact: true })).toBeVisible()
  fail = false
  await page.getByRole('button', { name: 'Try again', exact: true }).click()
  await expect(page.getByRole('button', { name: '9:00', exact: true })).toBeVisible()
  expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([])
})

test('operator recommendation browsing preserves entered times and the chosen date', async ({ page }) => {
  const data = fixture()
  const base = `/api/v1/instances/${data.instanceId}`
  const customers: Schemas['CustomersOutBody'] = await (await page.request.get(`${base}/customers?status=approved`)).json()
  const first = customers.customers[0]
  await page.route(`**${base}/customers?*`, (route) => route.fulfill({ json: { customers: [first, { ...first, id: 'second', name: 'Second customer' }] } }))
  const requests: URLSearchParams[] = []
  await page.route(`**${base}/availability?*`, (route) => {
    const params = new URL(route.request().url()).searchParams
    requests.push(params)
    const date = params.get('date') ?? '2030-01-15'
    return route.fulfill({ json: { timezone: 'Europe/London', date_range: { from: '2030-01-15', to: '2030-01-20' }, items: [{ starts_at: `${date}T09:00:00Z`, ends_at: `${date}T09:30:00Z` }], next_cursor: null, empty_reason: null } })
  })
  await page.goto(`/manage/${data.slug}/appointments`)
  await page.getByRole('button', { name: 'New appointment', exact: true }).click()
  const dialog = page.getByRole('dialog')
  const customer = dialog.getByLabel('Customer', { exact: true })
  const slot = dialog.locator('[data-slot-grid] button').first()
  await customer.selectOption(first.id)
  await dialog.getByRole('button', { name: 'Recommended times', exact: true }).click()
  await slot.click()
  await expect(slot).toHaveAttribute('aria-pressed', 'true')
  await dialog.getByRole('button', { name: 'Choose a date', exact: true }).click()
  await expect(slot).toHaveAttribute('aria-pressed', 'true')
  await dialog.getByRole('button', { name: 'Next day', exact: true }).click()
  await slot.click()
  await customer.selectOption('second')
  await expect(dialog.getByLabel('Appointment date', { exact: true })).toHaveValue('2030-01-16')
  await expect(slot).toHaveAttribute('aria-pressed', 'true')
  await expect.poll(() => requests.at(-1)?.get('customer_id')).toBe('second')
  expect(requests.at(-1)?.get('date')).toBe('2030-01-16')
  await expect(dialog.getByLabel('Start date and time')).toHaveValue('2030-01-16T09:00')
  await dialog.getByLabel('Start date and time').fill('2030-01-16T09:07')
  await dialog.getByLabel('End date and time').fill('2030-01-16T09:47')
  await expect(slot).toHaveAttribute('aria-pressed', 'false')
  await dialog.getByRole('button', { name: 'Next day', exact: true }).click()
  await expect(dialog.getByLabel('Start date and time')).toHaveValue('2030-01-16T09:07')
  await expect(dialog.getByLabel('End date and time')).toHaveValue('2030-01-16T09:47')
  await page.keyboard.press('Escape')
  await page.getByRole('tab', { name: 'Upcoming', exact: true }).click()
  await page.getByRole('button', { name: 'Change time', exact: true }).first().click()
  await dialog.getByRole('button', { name: 'Recommended times', exact: true }).click()
  await dialog.getByRole('button', { name: 'Choose a date', exact: true }).click()
  await expect.poll(() => requests.at(-1)?.has('exclude_appointment_id')).toBe(true)
  expect(requests.at(-1)?.has('customer_id')).toBe(false)
  expect(requests.at(-1)?.get('date')).toBe('2030-01-15')
})

test('staff enter and reschedule arbitrary cross-day times while paused, retaining errors and using calendar time zone', async ({ page, context, playwright, baseURL }) => {
  const data = fixture()
  const csrf = (await context.cookies()).find((cookie) => cookie.name === 'rm_csrf')!.value
  const headers = { 'X-CSRF-Token': csrf }
  const call = async (method: string, url: string, body?: unknown) => {
    const response = await page.request.fetch(url, { method, headers, data: body })
    expect(response.ok(), await response.text()).toBeTruthy()
    return response.json()
  }
  const slug = `manual-${crypto.randomUUID().slice(0, 18)}`
  const calendar = await call('POST', '/api/v1/platform/instances', {
    slug, name: 'Manual appointment test', timezone: 'Asia/Tokyo', default_locale: 'en', contact_kind: 'phone', contact_required: false,
  })
  const base = `/api/v1/instances/${calendar.id}`
  const operator = await playwright.request.newContext({ baseURL, storageState: data.operatorState })
  const { account } = await (await operator.get('/api/v1/account/me')).json()
  await call('PUT', `/api/v1/platform/instances/${calendar.id}/operators/${account.id}`)
  await call('PUT', `${base}/status`, { status: 'active' })
  const invite = await call('GET', `${base}/invite`)
  const customerId = crypto.randomUUID()
  await call('POST', `/api/v1/public/instances/${slug}/registrations`, { id: customerId, invite: invite.code, name: 'Manual customer' })
  const approved = await call('POST', `${base}/customers/${customerId}/actions/approve`)
  await call('PUT', `${base}/status`, { status: 'paused' })
  await call('PUT', `${base}/schedule`, { cycle_length_days: 1, anchor_date: '2026-01-01', days: [[]] })
  const policy = await call('GET', `${base}/policy`)
  await call('PUT', `${base}/policy`, { ...policy, per_customer_daily_limit: 0, max_future_bookings: 0 })
  await call('POST', `${base}/blocks`, { starts_at: '2030-01-15T00:00:00+09:00', ends_at: '2030-01-17T00:00:00+09:00' })
  const occupied = await call('POST', `${base}/appointments`, {
    id: crypto.randomUUID(), customer_id: customerId, starts_at: '2030-01-15T02:00:00+09:00', ends_at: '2030-01-15T03:00:00+09:00',
  })
  // Use a bound operator for the actual UI, with a different device time zone.
  await context.addCookies(data.operatorState.cookies)
  await page.goto(`/manage/${slug}/appointments`)
  await page.getByRole('button', { name: 'New appointment', exact: true }).click()
  const dialog = page.getByRole('dialog')
  await dialog.getByLabel('Customer', { exact: true }).selectOption(customerId)
  await expect(dialog.getByText(/Times are in Asia\/Tokyo/)).toBeVisible()
  const start = dialog.getByLabel('Start date and time', { exact: true })
  const end = dialog.getByLabel('End date and time', { exact: true })
  await start.fill('2030-01-15T01:07')
  await end.fill('2030-01-15T01:06')
  await dialog.getByRole('button', { name: 'Book', exact: true }).click()
  await expect(start).toHaveAttribute('aria-invalid', 'true')
  await expect(dialog.getByText('Enter valid start and end times, with the end after the start.')).toBeVisible()
  await end.fill('2030-01-16T01:23')
  await dialog.getByRole('button', { name: 'Recommended times', exact: true }).click()
  await expect(dialog.getByText('No recommended times. You can still enter a start and end time.')).toBeVisible()
  await dialog.getByRole('button', { name: 'Book', exact: true }).click()
  await expect(dialog.getByRole('alert')).toContainText('Sorry, this time was just taken.')
  await expect(start).toHaveValue('2030-01-15T01:07')
  await expect(end).toHaveValue('2030-01-16T01:23')
  expect((await new AxeBuilder({ page }).include('[role="dialog"]').analyze()).violations).toEqual([])
  await expect.poll(() => dialog.evaluate((el) => el.scrollWidth - el.clientWidth)).toBeLessThanOrEqual(1)
  await page.screenshot({ path: test.info().outputPath('manual-appointment.png'), fullPage: true })
  const operatorCsrf = (await operator.storageState()).cookies.find((cookie) => cookie.name === 'rm_csrf')!.value
  expect((await operator.post(`${base}/appointments/${occupied.id}/status`, { headers: { 'X-CSRF-Token': operatorCsrf }, data: { status: 'cancelled' } })).ok()).toBeTruthy()
  const createdResponse = page.waitForResponse((r) => r.url().endsWith(`${base}/appointments`) && r.request().method() === 'POST' && r.ok())
  await dialog.getByRole('button', { name: 'Book', exact: true }).click()
  const created: Schemas['ManagedAppointment'] = await (await createdResponse).json()
  expect(created.starts_at).toBe('2030-01-14T16:07:00Z')
  expect(created.ends_at).toBe('2030-01-15T16:23:00Z')
  await expect(dialog).toHaveCount(0)
  await page.getByRole('tab', { name: 'Upcoming', exact: true }).click()
  const row = page.getByRole('button', { name: 'Change time', exact: true })
  await row.click()
  await expect(start).toHaveValue('2030-01-15T01:07')
  await expect(end).toHaveValue('2030-01-16T01:23')
  await start.fill('2030-01-15T01:11')
  await end.fill('2030-01-16T01:27')
  const movedResponse = page.waitForResponse((r) => r.url().endsWith(`/appointments/${created.id}/reschedule`) && r.ok())
  await dialog.getByRole('button', { name: 'Move to this time', exact: true }).click()
  const moved: Schemas['ManagedAppointment'] = await (await movedResponse).json()
  expect(moved.starts_at).toBe('2030-01-14T16:11:00Z')
  expect(moved.ends_at).toBe('2030-01-15T16:27:00Z')
  await expect(dialog).toHaveCount(0)
  await page.goto(approved.access_path)
  await expect(page.getByText('Next appointment', { exact: true })).toBeVisible()
  await expect(page.getByText(/Wednesday, January 16, 2030 1:27/).first()).toBeVisible()
  await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth - innerWidth)).toBeLessThanOrEqual(1)
  await page.screenshot({ path: test.info().outputPath('customer-cross-day.png'), fullPage: true })
  await operator.dispose()
})

test('manual time entry rejects nonexistent local times and remains usable when recommendations fail', async ({ page }) => {
  const data = fixture()
  await page.route(`**/api/v1/instances/${data.instanceId}/availability?*`, (route) => route.fulfill(failure))
  await page.goto(`/manage/${data.slug}/appointments`)
  await page.getByRole('button', { name: 'New appointment', exact: true }).click()
  const dialog = page.getByRole('dialog')
  await dialog.getByLabel('Customer', { exact: true }).selectOption(data.customerId)
  const start = dialog.getByLabel('Start date and time', { exact: true })
  const end = dialog.getByLabel('End date and time', { exact: true })
  await start.fill('2030-03-31T01:15') // London skips 01:00–02:00.
  await end.fill('2030-03-31T02:30')
  await dialog.getByRole('button', { name: 'Book', exact: true }).click()
  await expect(start).toHaveAttribute('aria-invalid', 'true')
  await dialog.getByRole('button', { name: 'Recommended times', exact: true }).click()
  await expect(dialog.getByRole('button', { name: 'Try again', exact: true })).toBeVisible()
  await start.fill('2030-03-31T02:07')
  await expect(start).not.toHaveAttribute('aria-invalid', 'true')
  await expect(end).toHaveValue('2030-03-31T02:30')
  await expect(dialog.getByRole('button', { name: 'Book', exact: true })).toBeEnabled()
})
