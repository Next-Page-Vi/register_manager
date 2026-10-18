import { expect, test, type Page } from '@playwright/test'
import { readFileSync } from 'node:fs'
import { SEED_FILE, type Seed } from './seed'
import type { components } from '../src/api/schema'
import { formControlLayoutIssues } from './form-controls'

type Schemas = components['schemas']
const fixture = () => JSON.parse(readFileSync(SEED_FILE, 'utf8')) as Seed
const failure = {
  status: 500,
  contentType: 'application/json',
  body: JSON.stringify({ code: 'unknown_error', status: 500 }),
}
const gate = () => {
  let release!: () => void
  const promise = new Promise<void>((resolve) => {
    release = resolve
  })
  return { promise, release }
}
async function spaNavigate(page: Page, path: string) {
  await page.evaluate((path) => {
    history.pushState({}, '', path)
    dispatchEvent(new PopStateEvent('popstate'))
  }, path)
}
async function refocus(page: Page) {
  await page.evaluate(() => {
    Object.defineProperty(document, 'visibilityState', { configurable: true, value: 'hidden' })
    dispatchEvent(new Event('visibilitychange'))
    Object.defineProperty(document, 'visibilityState', { configurable: true, value: 'visible' })
    dispatchEvent(new Event('visibilitychange'))
  })
}

test.beforeEach(async ({ context }) => {
  await context.addCookies(fixture().staffState.cookies)
})

test('schedule inline editors and customer filters reflow at narrow widths and double text size', { tag: '@browser' }, async ({ page }) => {
  const data = fixture()
  for (const width of [320, 360, 390]) {
    await page.setViewportSize({ width, height: 800 })
    for (const fontSize of ['112.5%', '225%']) {
      await page.goto(`/manage/${data.slug}/schedule`)
      const monday = page.getByRole('button', { name: /^Monday/ })
      await expect(monday).toBeVisible()
      await page.evaluate((size) => { document.documentElement.style.fontSize = size }, fontSize)
      await expect.poll(() => page.evaluate(() => getComputedStyle(document.documentElement).fontSize))
        .toBe(fontSize === '225%' ? '36px' : '18px')
      await monday.click()
      const editor = page.getByRole('region', { name: 'Monday', exact: true })
      await expect(editor).toBeVisible()
      await expect(page.getByRole('dialog')).toHaveCount(0)
      await expect.poll(() => editor.evaluate((el) => el.scrollWidth - el.clientWidth)).toBeLessThanOrEqual(1)
      const start = editor.locator('input[type=time]').first()
      await start.fill('10:00')
      await editor.getByRole('button', { name: 'Add a period', exact: true }).click()
      await expect(editor.locator('input[type=time]')).toHaveCount(6)
      await expect(start).toHaveValue('10:00')
      await expect.poll(() => editor.evaluate(formControlLayoutIssues)).toEqual([])
      await editor.locator('summary').click()
      await expect.poll(() => editor.evaluate((el) => el.scrollWidth - el.clientWidth)).toBeLessThanOrEqual(1)
      await expect.poll(() => monday.evaluate((el) => el.scrollWidth - el.clientWidth)).toBeLessThanOrEqual(1)
      await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth))
        .toBeLessThanOrEqual(1)
      if (width === 320 && fontSize === '225%') {
        await start.locator('../..').screenshot({ path: test.info().outputPath('schedule-period-320-double-text.png') })
      }
      await monday.click()
      await expect(editor).toHaveCount(0)
      await monday.click()
      await expect(start).toHaveValue('10:00')
      await page.goto(`/manage/${data.slug}/customers`)
      await page.getByRole('tab', { name: 'Waiting for approval', exact: true }).waitFor()
      await page.evaluate((size) => { document.documentElement.style.fontSize = size }, fontSize)
      await expect.poll(() => page.evaluate(() => getComputedStyle(document.documentElement).fontSize))
        .toBe(fontSize === '225%' ? '36px' : '18px')
      await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth))
        .toBeLessThanOrEqual(1)
      await page.getByRole('tab', { name: 'Approved', exact: true }).click()
      await expect(page.getByRole('tab', { name: 'Approved', exact: true })).toHaveAttribute('aria-selected', 'true')
    }
  }
})

test('schedule date and time controls stay inside their fields', { tag: '@browser' }, async ({ page }) => {
  await page.goto(`/manage/${fixture().slug}/schedule`)
  await page.locator('#cycle-anchor').fill('2026-01-05')
  await page.locator('#day0-trigger').click()
  await page.locator('#day0-s-0').fill('13:05')
  for (const width of [320, 375, 393, 667]) {
    await page.setViewportSize({ width, height: 667 })
    await expect.poll(() => page.locator('main').evaluate(formControlLayoutIssues)).toEqual([])
  }

  await page.getByRole('button', { name: 'Add special date', exact: true }).click()
  const dialog = page.getByRole('dialog')
  await dialog.locator('#ov-date').fill('2026-12-31')
  await dialog.getByRole('checkbox', { name: 'Day off', exact: true }).uncheck()
  await dialog.getByRole('button', { name: 'Add a period', exact: true }).click()
  await expect(dialog.locator('input[type=time]')).toHaveCount(4)
  await dialog.locator('#ov-s-0').fill('13:05')
  await dialog.locator('#ov-e-0').fill('14:55')
  for (const width of [320, 375, 393, 667]) {
    await page.setViewportSize({ width, height: 667 })
    await expect.poll(() => dialog.evaluate(formControlLayoutIssues)).toEqual([])
  }
  await expect(dialog.locator('#ov-s-0')).toHaveValue('13:05')
  await page.keyboard.press('Escape')

  await page.getByRole('button', { name: 'Block a time', exact: true }).click()
  await dialog.locator('#b-start').fill('2026-12-31T13:05')
  await dialog.locator('#b-end').fill('2026-12-31T23:55')
  for (const width of [320, 375, 393, 667]) {
    await page.setViewportSize({ width, height: 667 })
    await expect.poll(() => dialog.evaluate(formControlLayoutIssues)).toEqual([])
  }
  await expect(dialog.locator('#b-start')).toHaveValue('2026-12-31T13:05')
  await expect(dialog.locator('#b-end')).toHaveValue('2026-12-31T23:55')
})

test('schedule drafts survive switching days and invalid submissions reveal the first problem', { tag: '@browser' }, async ({ page }) => {
  const data = fixture()
  await page.goto(`/manage/${data.slug}/schedule`)
  const monday = page.getByRole('button', { name: /^Monday/ })
  const tuesday = page.getByRole('button', { name: /^Tuesday/ })
  const mondayEditor = page.getByRole('region', { name: 'Monday', exact: true })
  const tuesdayEditor = page.getByRole('region', { name: 'Tuesday', exact: true })

  await monday.focus()
  await page.keyboard.press('Enter')
  await expect(monday).toHaveAttribute('aria-expanded', 'true')
  await expect(monday).toBeFocused()
  await mondayEditor.locator('input[type=time]').first().fill('10:00')
  await mondayEditor.getByLabel('Monday, period 1, end', { exact: true }).fill('')
  await tuesday.click()
  await expect(monday).toHaveAttribute('aria-expanded', 'false')
  await expect(monday).toContainText('Fill in every start and end time.')
  await expect(tuesday).toBeInViewport()
  await expect(page.locator('[id$="-panel"][role=region]:visible')).toHaveCount(1)
  await tuesdayEditor.locator('input[type=time]').first().fill('11:00')
  await monday.click()
  await expect(mondayEditor.locator('input[type=time]').first()).toHaveValue('10:00')
  await expect(mondayEditor.getByLabel('Monday, period 1, end', { exact: true })).toHaveValue('')
  await mondayEditor.locator('summary').click()
  await mondayEditor.getByRole('checkbox', { name: 'Tuesday', exact: true }).check()
  await expect(mondayEditor.getByRole('button', { name: 'Apply to selected days' })).toBeDisabled()

  for (const action of ['Preview', 'Save']) {
    await tuesday.click()
    await page.getByRole('button', { name: action, exact: true }).click()
    await expect(monday).toHaveAttribute('aria-expanded', 'true')
    await expect(mondayEditor.getByLabel('Monday, period 1, end', { exact: true })).toBeFocused()
    await expect(mondayEditor.getByLabel('Monday, period 1, end', { exact: true })).toHaveAttribute('aria-describedby', 'day0-error')
  }
  // Repeated validation works even if the invalid day is already expanded.
  await page.getByRole('button', { name: 'Preview', exact: true }).click()
  await expect(mondayEditor.getByLabel('Monday, period 1, end', { exact: true })).toBeFocused()
  await mondayEditor.getByLabel('Monday, period 1, end', { exact: true }).fill('12:00')
  await tuesday.click()
  await expect(tuesdayEditor.locator('input[type=time]').first()).toHaveValue('11:00')
  await tuesday.focus()
  await page.keyboard.press('Space')
  await expect(tuesday).toHaveAttribute('aria-expanded', 'false')
  await expect(page.getByText('Unsaved changes', { exact: true })).toBeVisible()
  const request = page.waitForRequest((request) => request.url().endsWith('/schedule/preview'))
  await page.getByRole('button', { name: 'Preview', exact: true }).click()
  const draft = (await request).postDataJSON()
  expect(draft.days[0][0]).toEqual({ start: 600, end: 720 })
  expect(draft.days[1][0]).toEqual({ start: 660, end: 720 })
})

test('schedule copying is explicit, copies independent drafts and supports arbitrary cycle lengths', async ({ page }) => {
  const data = fixture()
  await page.goto(`/manage/${data.slug}/schedule`)
  const monday = page.getByRole('button', { name: /^Monday/ })
  const tuesday = page.getByRole('button', { name: /^Tuesday/ })
  const mondayEditor = page.getByRole('region', { name: 'Monday', exact: true })
  const tuesdayEditor = page.getByRole('region', { name: 'Tuesday', exact: true })
  await monday.click()
  await mondayEditor.locator('input[type=time]').first().fill('10:00')
  await mondayEditor.locator('summary').click()
  const original = await tuesday.textContent()
  await mondayEditor.getByRole('checkbox', { name: 'Tuesday', exact: true }).check()
  await expect(tuesday).toHaveText(original!)
  await mondayEditor.getByRole('button', { name: 'Apply to selected days' }).click()
  await expect(mondayEditor.getByRole('status')).toContainText('Selected days updated')
  await tuesday.click()
  await expect(tuesdayEditor.locator('input[type=time]').first()).toHaveValue('10:00')
  await tuesdayEditor.locator('input[type=time]').first().fill('11:00')
  await monday.click()
  await expect(mondayEditor.locator('input[type=time]').first()).toHaveValue('10:00')

  await mondayEditor.getByRole('checkbox', { name: 'Day off', exact: true }).check()
  await mondayEditor.locator('summary').click()
  await mondayEditor.getByRole('checkbox', { name: 'Tuesday', exact: true }).check()
  await mondayEditor.getByRole('button', { name: 'Apply to selected days' }).click()
  await expect(tuesday).toContainText('Day off')
  await page.locator('#cycle-n').fill('3')
  await expect(page.getByRole('button', { name: /^Day 1/ })).toHaveAttribute('aria-expanded', 'true')
  const first = page.getByRole('region', { name: 'Day 1', exact: true })
  await first.locator('summary').click()
  await expect(first.getByRole('checkbox')).toHaveCount(3)
  await first.getByRole('checkbox', { name: 'Day 3', exact: true }).check()
  await first.getByRole('button', { name: 'Apply to selected days' }).click()
  await expect(page.getByRole('button', { name: /^Day 3/ })).toContainText('Day off')
  await page.getByRole('button', { name: /^Day 3/ }).click()
  await page.locator('#cycle-n').fill('2')
  await expect(page.locator('[id$="-panel"][role=region]:visible')).toHaveCount(0)
  await page.locator('#cycle-n').fill('3')
  await expect(page.getByRole('button', { name: /^Day 3/ })).toHaveAttribute('aria-expanded', 'false')
})

test('selecting a time keeps every option in place and narrow dialog actions use full rows', { tag: '@browser' }, async ({ page }) => {
  const data = fixture()
  await page.setViewportSize({ width: 320, height: 800 })
  await page.goto(`/manage/${data.slug}/appointments`)
  await page.getByRole('tab', { name: 'Upcoming', exact: true }).click()
  await page.getByRole('button', { name: 'Change time', exact: true }).first().click()
  const dialog = page.getByRole('dialog')
  await dialog.getByRole('button', { name: 'Recommended times', exact: true }).click()
  const options = dialog.locator('[data-slot-grid] button[aria-pressed]')
  await expect(options.first()).toBeVisible()
  const boxes = () => options.evaluateAll((elements) => elements.slice(0, 8).map((el) => {
    // Local layout coordinates exclude the opening animation and automatic
    // scrolling performed when an option receives focus.
    const button = el as HTMLButtonElement
    return { x: button.offsetLeft, y: button.offsetTop, width: button.offsetWidth, height: button.offsetHeight }
  }))
  const before = await boxes()
  await options.first().click()
  expect(await boxes()).toEqual(before)
  await options.nth(1).click()
  expect(await boxes()).toEqual(before)
  const cancel = await dialog.getByRole('button', { name: 'Cancel', exact: true }).boundingBox()
  const confirm = await dialog.getByRole('button', { name: 'Move to this time', exact: true }).boundingBox()
  expect(cancel).not.toBeNull()
  expect(confirm).not.toBeNull()
  expect(confirm!.y).toBeGreaterThanOrEqual(cancel!.y + cancel!.height)
  expect(Math.abs(cancel!.width - confirm!.width)).toBeLessThanOrEqual(1)
})

test('other upcoming appointments expand by keyboard and stay accurate after cancellation', async ({ page, context }) => {
  const data = fixture()
  await context.addCookies(data.customerState.cookies)
  const endpoint = `/api/v1/customer/${data.slug}/appointments`
  const initial: Schemas['CustomerAppointmentsOutBody'] = await (await page.request.get(endpoint)).json()
  let upcoming = initial.upcoming
  expect(upcoming).toHaveLength(2)
  await page.route(`**${endpoint}`, (route) => route.fulfill({ json: { ...initial, upcoming } }))
  await page.route(`**${endpoint}/*/cancel`, async (route) => {
    const id = route.request().url().split('/').at(-2)
    const cancelled = upcoming.find((appointment) => appointment.id === id)!
    upcoming = upcoming.filter((appointment) => appointment.id !== id)
    await route.fulfill({ json: { ...cancelled, status: 'cancelled' } })
  })
  await page.goto(`/i/${data.slug}/booking`)
  const cancelButtons = page.getByRole('button', { name: 'Cancel this appointment', exact: true })
  await expect(cancelButtons).toHaveCount(1)
  const expand = page.getByRole('button', { name: 'Show 1 other upcoming appointment', exact: true })
  await expect(expand).toHaveAttribute('aria-expanded', 'false')
  await expand.focus()
  await expand.press('Enter')
  const collapse = page.getByRole('button', { name: 'Hide other appointments', exact: true })
  await expect(collapse).toHaveAttribute('aria-expanded', 'true')
  await expect(cancelButtons).toHaveCount(2)
  await collapse.press('Enter')
  await expect(cancelButtons).toHaveCount(1)
  await expand.click()

  await cancelButtons.nth(1).click()
  await page.getByRole('alertdialog').getByRole('button', { name: 'Yes, cancel it', exact: true }).click()
  await expect(cancelButtons).toHaveCount(1)
  await expect(collapse).toHaveCount(0)
  await expect(expand).toHaveCount(0)
  await cancelButtons.click()
  await page.getByRole('alertdialog').getByRole('button', { name: 'Yes, cancel it', exact: true }).click()
  await expect(cancelButtons).toHaveCount(0)
  await expect(page.getByRole('heading', { name: 'Earliest available times', exact: true })).toBeVisible()
})

test('schedule section navigation keeps the unsaved repeating schedule', async ({ page }) => {
  const data = fixture()
  await page.goto(`/manage/${data.slug}/schedule`)
  const anchor = page.locator('#cycle-anchor')
  await anchor.fill('2027-01-04')
  const sections = page.getByRole('navigation', { name: 'Working hours sections' })
  await sections.getByRole('link', { name: 'Special dates', exact: true }).click()
  await expect(page).toHaveURL(/#schedule-overrides$/)
  await sections.getByRole('link', { name: 'Repeating schedule', exact: true }).click()
  await expect(page).toHaveURL(/#schedule-cycle$/)
  await expect(anchor).toHaveValue('2027-01-04')
})

test('back office controls stay within a small screen with text enlarged to 200 percent', { tag: '@browser' }, async ({ page }) => {
  const data = fixture()
  await page.setViewportSize({ width: 320, height: 568 })
  for (const section of ['settings', 'schedule']) {
    await page.goto(`/manage/${data.slug}/${section}`)
    await expect(page.locator(section === 'settings' ? '#s-name' : '#cycle-anchor')).toBeVisible()
    // The normal root size is 112.5%; double the text without shrinking it
    // to make the fixed-width fields fit.
    await page.evaluate(() => { document.documentElement.style.fontSize = '225%' })
    await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth - innerWidth)).toBeLessThanOrEqual(1)
    if (section === 'settings') {
      const qr = page.getByRole('img', { name: 'Registration link', exact: true })
      await expect(qr).toBeVisible()
      const bounds = await qr.boundingBox()
      expect(bounds).not.toBeNull()
      expect(Math.abs(bounds!.width - bounds!.height)).toBeLessThanOrEqual(1)
    }
  }
})

test('clearing a page colour saves null and later custom colours still preview and save', async ({ page }) => {
  const data = fixture()
  const endpoint = `/api/v1/instances/${data.instanceId}`
  const access: Schemas['InstanceAccess'] = await (await page.request.get(`/api/v1/instances/by-slug/${data.slug}`)).json()
  const colours: (string | null)[] = []
  await page.route(`**${endpoint}`, async (route) => {
    if (route.request().method() !== 'PUT') return route.continue()
    const body = route.request().postDataJSON()
    colours.push(body.brand_color)
    await route.fulfill({ json: { ...access.instance, ...body } })
  })
  await page.goto(`/manage/${data.slug}/settings`)
  const colour = page.getByLabel('Page colour', { exact: true })
  await page.getByRole('button', { name: 'Use the standard look', exact: true }).click()
  await expect(colour).toHaveValue('')
  await page.getByRole('button', { name: 'Save', exact: true }).click()
  await expect(page.getByText('Saved.', { exact: true })).toBeVisible()
  expect(colours).toEqual([null])
  await colour.fill('#123456')
  await expect(page.locator('figure header')).toHaveCSS('background-color', 'rgb(18, 52, 86)')
  await page.getByRole('button', { name: 'Save', exact: true }).click()
  await expect(page.getByText('Saved.', { exact: true })).toBeVisible()
  expect(colours).toEqual([null, '#123456'])
})

test('an untouched note is preserved; edits survive refetch and clear as null', async ({ page }) => {
  const data = fixture()
  const endpoint = `/api/v1/instances/${data.instanceId}/customers/${data.customerId}`
  const customer: Schemas['CustomerDetail'] = await (await page.request.get(endpoint)).json()
  let stored = customer.internal_note
  const sent: (string | null)[] = []
  await page.route(`**${endpoint}`, (route) => route.fulfill({ json: { ...customer, internal_note: stored } }))
  await page.route(`**${endpoint}/note`, async (route) => {
    stored = route.request().postDataJSON().note
    sent.push(stored)
    await route.fulfill({ json: { ...customer, internal_note: stored } })
  })
  await page.goto(`/manage/${data.slug}/customers/${data.customerId}`)
  const note = page.getByLabel('Internal note', { exact: true })
  await expect(note).toHaveValue(customer.internal_note!)
  await page.getByRole('button', { name: 'Save', exact: true }).click()
  await expect(page.getByText('Saved.', { exact: true })).toBeVisible()
  expect(sent).toEqual([customer.internal_note])

  await note.fill('Unsaved local edit')
  stored = 'A newer value from the server'
  const refreshed = page.waitForResponse((response) => response.url().endsWith(endpoint))
  await refocus(page)
  await refreshed
  await expect(note).toHaveValue('Unsaved local edit')
  await note.fill('')
  await page.getByRole('button', { name: 'Save', exact: true }).click()
  await expect(page.getByText('Saved.', { exact: true })).toBeVisible()
  expect(sent.at(-1)).toBeNull()
  await expect(note).toHaveValue('')
})

test('failed forms and confirmations stay open and retain their input', async ({ page }) => {
  const pending = gate()
  let requests = 0
  await page.route('**/api/v1/platform/accounts', async (route) => {
    if (route.request().method() === 'GET') return route.continue()
    requests++
    await pending.promise
    await route.fulfill(failure)
  })
  await page.goto('/platform/accounts')
  await page.getByRole('button', { name: 'New operator' }).click()
  const dialog = page.getByRole('dialog')
  await dialog.getByLabel('Login name').fill('preserved-login')
  await dialog.getByLabel('Name', { exact: true }).fill('Preserved Name')
  await dialog.getByLabel('Initial password').fill('preserved-password')
  await dialog.getByRole('button', { name: 'Create', exact: true }).click()
  await expect(dialog.getByRole('button', { name: 'Please wait…' })).toBeDisabled()
  expect(requests).toBe(1)
  pending.release()
  await expect(dialog.getByRole('alert')).toBeVisible()
  await expect(dialog.getByLabel('Login name')).toHaveValue('preserved-login')
  await dialog.getByRole('button', { name: 'Cancel', exact: true }).click()
  await expect(dialog).toHaveCount(0)

  await page.route('**/api/v1/platform/accounts/*/disabled', (route) => route.fulfill(failure))
  const operator = page.locator('[data-slot=card]').filter({ hasText: 'Dr Maximiliana' }).first()
  await operator.getByRole('button', { name: 'Disable', exact: true }).click()
  const confirm = page.getByRole('alertdialog')
  await confirm.getByRole('button', { name: 'Confirm', exact: true }).click()
  await expect(confirm.getByRole('alert')).toBeVisible()
  await expect(confirm.getByRole('button', { name: 'Confirm', exact: true })).toBeEnabled()
})

test('a late next page cannot overwrite the newly selected appointment view', async ({ page }) => {
  const data = fixture()
  const pending = gate()
  const started = gate()
  const appointment = (name: string): Schemas['ManagedAppointment'] => ({
    id: name,
    customer_id: data.customerId,
    customer_name: name,
    starts_at: '2030-01-15T10:00:00Z',
    ends_at: '2030-01-15T10:30:00Z',
    status: 'completed',
  })
  await page.route(`**/api/v1/instances/${data.instanceId}/appointments?*`, async (route) => {
    const url = new URL(route.request().url())
    const view = url.searchParams.get('view')
    const cursor = url.searchParams.get('cursor')
    expect(cursor).not.toBe('0')
    expect(cursor).not.toBe('')
    if (cursor) {
      started.release()
      await pending.promise
      return route.fulfill({ json: { items: [appointment('Late upcoming')], next_cursor: null } })
    }
    await route.fulfill({
      json: {
        items: [appointment(view === 'history' ? 'History only' : 'First page')],
        next_cursor: view === 'upcoming' ? 'next' : null,
      },
    })
  })
  await page.goto(`/manage/${data.slug}/appointments`)
  await page.getByRole('tab', { name: 'Upcoming', exact: true }).click()
  await expect(page.getByRole('link', { name: 'First page', exact: true })).toHaveCount(1)
  await page.getByRole('button', { name: 'Show more', exact: true }).click()
  await started.promise
  await page.getByRole('tab', { name: 'History', exact: true }).click()
  await expect(page.getByRole('link', { name: 'History only', exact: true })).toHaveCount(1)
  pending.release()
  await expect(page.getByRole('link', { name: 'First page', exact: true })).toHaveCount(0)
  await expect(page.getByRole('link', { name: 'Late upcoming', exact: true })).toHaveCount(0)
})

test('appointment edits preserve untouched instants across the daylight-saving fall-back', async ({ page }) => {
  const data = fixture()
  const base = `/api/v1/instances/${data.instanceId}`
  const appointment: Schemas['ManagedAppointment'] = {
    id: 'fall-back', customer_id: data.customerId, customer_name: 'Fall-back customer', status: 'booked',
    starts_at: '2026-10-25T00:00:00Z', ends_at: '2026-10-25T01:30:00Z',
  }
  await page.route(`**${base}/appointments?*`, (route) => route.fulfill({ json: { items: [appointment], next_cursor: null } }))
  await page.route(`**${base}/appointments/${appointment.id}/reschedule`, (route) => route.fulfill(failure))
  await page.goto(`/manage/${data.slug}/appointments`)
  await page.getByRole('button', { name: 'Change time', exact: true }).click()
  const dialog = page.getByRole('dialog')
  const start = dialog.getByLabel('Start date and time', { exact: true })
  const end = dialog.getByLabel('End date and time', { exact: true })
  await expect(start).toHaveValue('2026-10-25T01:00')
  await expect(end).toHaveValue('2026-10-25T01:30')

  const submit = async (startsAt: string, endsAt: string) => {
    const response = page.waitForResponse((r) => r.url().endsWith(`/${appointment.id}/reschedule`))
    await dialog.getByRole('button', { name: 'Move to this time', exact: true }).click()
    expect((await response).request().postDataJSON()).toMatchObject({ starts_at: startsAt, ends_at: endsAt })
    await expect(dialog.getByRole('button', { name: 'Move to this time', exact: true })).toBeEnabled()
  }
  // No edit must preserve the full 90 minutes, including the later 01:30.
  await submit(appointment.starts_at, appointment.ends_at)
  await start.fill('2026-10-25T00:45')
  await submit('2026-10-24T23:45:00.000Z', appointment.ends_at)
  await dialog.getByRole('button', { name: 'Cancel', exact: true }).click()
  await expect(dialog).toHaveCount(0)
  await page.getByRole('button', { name: 'Change time', exact: true }).click()
  await end.fill('2026-10-25T02:00')
  await submit(appointment.starts_at, '2026-10-25T02:00:00.000Z')
})

test('recommended appointments retain their instants when fall-back wall times repeat or run backwards', async ({ page }) => {
  const data = fixture()
  const base = `/api/v1/instances/${data.instanceId}`
  const slots: Schemas['Slot'][] = [
    { starts_at: '2026-10-25T00:00:00Z', ends_at: '2026-10-25T01:30:00Z' },
    { starts_at: '2026-10-25T00:30:00Z', ends_at: '2026-10-25T01:00:00Z' },
    { starts_at: '2026-10-25T00:45:00Z', ends_at: '2026-10-25T01:45:00Z' },
    { starts_at: '2026-10-25T01:00:00Z', ends_at: '2026-10-25T01:30:00Z' },
  ]
  await page.route(`**${base}/availability?*`, (route) => route.fulfill({
    json: { timezone: 'Europe/London', items: slots, next_cursor: null, empty_reason: null },
  }))
  await page.route(`**${base}/appointments`, (route) => route.fulfill(failure))
  await page.goto(`/manage/${data.slug}/appointments`)
  await page.getByRole('button', { name: 'New appointment', exact: true }).click()
  const dialog = page.getByRole('dialog')
  await dialog.getByLabel('Customer', { exact: true }).selectOption(data.customerId)
  await dialog.getByRole('button', { name: 'Recommended times', exact: true }).click()
  const options = dialog.locator('[data-slot-grid] button[aria-pressed]')
  const submit = async (slot: Schemas['Slot']) => {
    const response = page.waitForResponse((r) => r.url().endsWith(`${base}/appointments`) && r.request().method() === 'POST')
    await dialog.getByRole('button', { name: 'Book', exact: true }).click()
    expect((await response).request().postDataJSON()).toMatchObject(slot)
    await expect(dialog.getByRole('button', { name: 'Book', exact: true })).toBeEnabled()
  }
  for (const [index, slot] of slots.entries()) {
    await options.nth(index).click()
    await expect(options.nth(index)).toHaveAttribute('aria-pressed', 'true')
    await submit(slot)
  }
  // Editing the end must retain the start in the later occurrence of 01:00.
  await dialog.getByLabel('End date and time', { exact: true }).fill('2026-10-25T02:00')
  await submit({ starts_at: slots[3].starts_at, ends_at: '2026-10-25T02:00:00.000Z' })
})

test('rescheduling keeps the original interval precondition through a background refresh', async ({ page }) => {
  const data = fixture()
  const base = `/api/v1/instances/${data.instanceId}`
  const original: Schemas['ManagedAppointment'] = {
    id: 'concurrent-edit', customer_id: data.customerId, customer_name: 'Concurrent customer', status: 'booked',
    starts_at: '2030-01-15T10:00:00Z', ends_at: '2030-01-15T10:30:00Z',
  }
  let current = original
  await page.route(`**${base}/appointments?*`, (route) => route.fulfill({ json: { items: [current], next_cursor: null } }))
  await page.route(`**${base}/appointments/${original.id}/reschedule`, (route) => route.fulfill({
    status: 409, json: { status: 409, code: 'appointment_changed' },
  }))
  await page.goto(`/manage/${data.slug}/appointments`)
  await page.getByRole('button', { name: 'Change time', exact: true }).click()
  const dialog = page.getByRole('dialog')
  await dialog.getByLabel('End date and time', { exact: true }).fill('2030-01-15T12:00')
  current = { ...original, ends_at: '2030-01-15T11:00:00Z' }
  await refocus(page)
  await expect(dialog).toContainText('11:00')
  const response = page.waitForResponse((r) => r.url().endsWith(`/${original.id}/reschedule`))
  await dialog.getByRole('button', { name: 'Move to this time', exact: true }).click()
  expect((await response).request().postDataJSON()).toEqual({
    expected_starts_at: original.starts_at, expected_ends_at: original.ends_at,
    starts_at: original.starts_at, ends_at: '2030-01-15T12:00:00.000Z',
  })
  await expect(dialog.getByRole('alert')).toBeVisible()
  await expect(dialog.getByLabel('End date and time', { exact: true })).toHaveValue('2030-01-15T12:00')
  await dialog.getByRole('button', { name: 'Cancel', exact: true }).click()
  await expect(dialog).toHaveCount(0)
  await page.getByRole('button', { name: 'Change time', exact: true }).click()
  await expect(dialog.getByLabel('End date and time', { exact: true })).toHaveValue('2030-01-15T11:00')
  const retry = page.waitForResponse((r) => r.url().endsWith(`/${original.id}/reschedule`))
  await dialog.getByRole('button', { name: 'Move to this time', exact: true }).click()
  expect((await retry).request().postDataJSON()).toMatchObject({ expected_ends_at: current.ends_at })
})

test('changing customer isolates recommendation queries', async ({ page }) => {
  const data = fixture()
  const base = `/api/v1/instances/${data.instanceId}`
  const customer = {
    id: 'first',
    name: 'First customer',
    status: 'approved',
    contact: null,
    locale: null,
    internal_note: null,
    created_at: '2026-01-01T00:00:00Z',
  }
  await page.route(`**${base}/customers?*`, (route) =>
    route.fulfill({ json: { customers: [customer, { ...customer, id: 'second', name: 'Second customer' }] } }),
  )
  const pending = gate()
  const started = gate()
  await page.route(`**${base}/availability?*`, async (route) => {
    const first = new URL(route.request().url()).searchParams.get('customer_id') === 'first'
    if (first) {
      started.release()
      await pending.promise
    }
    await route.fulfill({
      json: {
        timezone: 'Europe/London',
        items: [
          {
            starts_at: first ? '2030-01-15T09:00:00Z' : '2030-01-15T15:00:00Z',
            ends_at: first ? '2030-01-15T09:30:00Z' : '2030-01-15T15:30:00Z',
          },
        ],
        next_cursor: null,
        empty_reason: null,
      },
    })
  })
  await page.goto(`/manage/${data.slug}/appointments`)
  await page.getByRole('button', { name: 'New appointment', exact: true }).click()
  const dialog = page.getByRole('dialog')
  await dialog.getByLabel('Customer', { exact: true }).selectOption('first')
  await dialog.getByRole('button', { name: 'Recommended times', exact: true }).click()
  await started.promise
  await dialog.getByLabel('Customer', { exact: true }).selectOption('second')
  await expect(dialog.getByRole('button', { name: '15:00', exact: true })).toBeVisible()
  pending.release()
  await expect(dialog.getByRole('button', { name: '09:00', exact: true })).toHaveCount(0)
})

test('policy drafts survive background refresh and empty numbers never submit as zero', async ({ page }) => {
  const data = fixture()
  const endpoint = `/api/v1/instances/${data.instanceId}/policy`
  let writes = 0
  await page.route(`**${endpoint}`, (route) => {
    if (route.request().method() === 'PUT') writes++
    return route.continue()
  })
  await page.goto(`/manage/${data.slug}/policy`)
  const duration = page.getByLabel('Length of one appointment (minutes)', { exact: true })
  await duration.fill('45')
  const refreshed = page.waitForResponse((response) => response.url().endsWith(endpoint))
  await refocus(page)
  await refreshed
  await expect(duration).toHaveValue('45')
  await duration.fill('')
  await page.getByRole('button', { name: 'Save', exact: true }).click()
  await expect(duration).toBeFocused()
  expect(writes).toBe(0)
})

test('signing in as another account clears the previous identity and cached permissions', async ({
  page,
  playwright,
  baseURL,
}) => {
  const data = fixture()
  const operator = await playwright.request.newContext({ baseURL, storageState: data.operatorState })
  const { account } = await (await operator.get('/api/v1/account/me')).json()
  await operator.dispose()
  await page.goto('/platform/accounts')
  await expect(page.getByRole('heading', { name: 'Accounts', exact: true })).toBeVisible()
  await spaNavigate(page, '/account/login')
  await page.route('**/api/v1/account/login', (route) => route.fulfill({ json: account }))
  await page.route('**/api/v1/account/me', (route) => route.fulfill({ json: { account } }))
  await page.getByLabel('Login name').fill(account.login)
  await page.getByLabel('Password').fill('worker-password')
  await page.getByRole('button', { name: 'Sign in', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'My calendars', exact: true })).toBeVisible()
  await spaNavigate(page, '/platform/accounts')
  await expect(page.getByRole('heading', { name: 'Accounts', exact: true })).toHaveCount(0)
  await expect(page.getByRole('alert')).toBeVisible()
})

test('a late customer 401 does not sign the back office out', async ({ page, context }) => {
  const data = fixture()
  await context.addCookies(data.customerState.cookies)
  const pending = gate()
  const started = gate()
  const finished = gate()
  await page.route(`**/api/v1/customer/${data.slug}/appointments/*/cancel`, async (route) => {
    started.release()
    await pending.promise
    await route.fulfill({ status: 401, json: { status: 401, code: 'unauthenticated' } })
    finished.release()
  })
  await page.goto(`/i/${data.slug}/booking`)
  await page.getByRole('button', { name: 'Cancel this appointment', exact: true }).first().click()
  await page.getByRole('alertdialog').getByRole('button', { name: 'Yes, cancel it', exact: true }).click()
  await started.promise
  await spaNavigate(page, '/platform/accounts')
  await expect(page.getByRole('heading', { name: 'Accounts', exact: true })).toBeVisible()
  pending.release()
  await finished.promise
  await expect(page).toHaveURL(/\/platform\/accounts$/)
  await expect(page.getByRole('heading', { name: 'Accounts', exact: true })).toBeVisible()
})

test('preview replaces an earlier save error without losing the policy draft', async ({ page }) => {
  const data = fixture()
  const endpoint = `/api/v1/instances/${data.instanceId}/policy`
  await page.route(`**${endpoint}`, (route) =>
    route.request().method() === 'PUT' ? route.fulfill(failure) : route.continue(),
  )
  await page.goto(`/manage/${data.slug}/policy`)
  const duration = page.getByLabel('Length of one appointment (minutes)', { exact: true })
  await duration.fill('45')
  await page.getByRole('button', { name: 'Save', exact: true }).click()
  await expect(page.getByRole('alert')).toBeVisible()
  const preview = page.waitForResponse((response) => response.url().endsWith(`${endpoint}/preview`))
  await page.getByRole('button', { name: 'Preview', exact: true }).click()
  await preview
  await expect(page.getByRole('alert')).toHaveCount(0)
  await expect(duration).toHaveValue('45')
})

test('a network failure while obtaining CSRF is reported without an automatic retry', async ({ page, context }) => {
  await page.goto('/account/login')
  // Wait for the lazy login module before removing cookies: its response can
  // set a new CSRF cookie, especially when WebKit loads it after page.goto.
  await page.getByLabel('Login name').fill('admin')
  await page.getByLabel('Password').fill('root-password')
  await context.clearCookies()
  await expect.poll(() => page.evaluate(() =>
    document.cookie.split(';').some((cookie) => cookie.trim().startsWith('rm_csrf=')),
  )).toBe(false)
  let requests = 0
  await page.route('**/api/v1/account/me', (route) => {
    requests++
    return route.abort('failed')
  })
  await page.getByRole('button', { name: 'Sign in', exact: true }).click()
  await expect(page.getByText('No connection. Please check your internet connection and try again.')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Sign in', exact: true })).toBeEnabled()
  expect(requests).toBe(1)
})
