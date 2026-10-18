import { expect, test, type Locator, type Page } from '@playwright/test'

/** Exercise real touch events in phone projects while sharing the same flow. */
async function activate(control: Locator) {
  if (test.info().project.use.hasTouch) await control.tap()
  else await control.click()
}

/** Signs in; operators land on their calendars, the super admin on all calendars. */
async function login(page: Page, login: string, password: string, home = 'My calendars') {
  await page.goto('/account/login')
  await page.getByLabel('Login name').fill(login)
  await page.getByLabel('Password').fill(password)
  await page.getByRole('button', { name: 'Sign in' }).click()
  await expect(page.getByRole('heading', { name: home })).toBeVisible()
}

async function noHorizontalScroll(page: Page) {
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth)
  expect(overflow).toBeLessThanOrEqual(1)
}

/** Opens the menu on narrow screens, then follows a navigation link. */
async function goSection(page: Page, name: string) {
  // While a dialog is still closing the rest of the page is hidden from the
  // accessibility tree (so the menu button cannot be found) and the dialog
  // then hands focus back to its trigger; wait until it is gone.
  await expect(page.getByRole('dialog')).toHaveCount(0)
  const menu = page.getByRole('button', { name: 'Menu' })
  // The desktop navigation is in the page but hidden on phones; use the visible copy.
  const link = page.getByRole('link', { name, exact: true }).filter({ visible: true }).first()
  if (await menu.isVisible()) {
    await activate(menu)
    await expect(link).toBeVisible()
  }
  await activate(link)
}

test('staff set up a calendar and a customer books and cancels', { tag: '@smoke' }, async ({ page, browser }, info) => {
  const tag = info.project.name
  const worker = `worker-${tag}`
  const slug = `cal-${tag}`

  // The super admin creates an operator, then a calendar, then adds the
  // operator to the calendar.
  await login(page, 'admin', 'root-password', 'All calendars')
  // The super admin opens calendars from "All calendars" and has no "My calendars".
  await expect(page.getByRole('link', { name: 'My calendars' })).toHaveCount(0)
  await goSection(page, 'Accounts')
  await page.getByRole('button', { name: 'New operator' }).click()
  const dlg = page.getByRole('dialog')
  await dlg.getByLabel('Login name').fill(worker)
  await dlg.getByLabel('Name', { exact: true }).fill(`Dr ${tag}`)
  await dlg.getByLabel('Initial password').fill('worker-password')
  await dlg.getByRole('button', { name: 'Create' }).click()
  await expect(page.getByText(`(${worker})`)).toBeVisible()

  await goSection(page, 'All calendars')
  await page.getByRole('button', { name: 'New calendar' }).click()
  const inst = page.getByRole('dialog')
  await inst.getByLabel('Name', { exact: true }).fill(`Clinic ${tag}`)
  await inst.getByLabel('Web address name').fill(slug)
  await inst.getByLabel('Time zone').selectOption('Europe/London')
  await inst.getByRole('button', { name: 'Create' }).click()
  const card = page.locator('[data-slot=card]').filter({ has: page.getByRole('heading', { name: `Clinic ${tag}` }) })
  await expect(card.getByText('No operators yet.')).toBeVisible()
  await card.getByRole('button', { name: 'Add operator' }).click()
  const bind = page.getByRole('dialog')
  await bind.getByLabel('Operator').selectOption({ label: `Dr ${tag} (${worker})` })
  await bind.getByRole('button', { name: 'Add' }).click()
  const operatorName = card.getByText(`Dr ${tag}`, { exact: true })
  await expect(operatorName).toBeVisible()
  // A single-line name must not add vertical padding around the remove target.
  await expect.poll(() => operatorName.evaluate((el) => {
    const chip = el.parentElement!
    return chip.getBoundingClientRect().height - chip.querySelector('button')!.getBoundingClientRect().height
  })).toBeLessThanOrEqual(1)
  await noHorizontalScroll(page)

  // The staff member signs in, opens booking and reads the registration link.
  const staffCtx = await browser.newContext({ ...info.project.use, baseURL: info.project.use.baseURL })
  const staff = await staffCtx.newPage()
  await login(staff, worker, 'worker-password')
  await staff.getByRole('link', { name: new RegExp(`Clinic ${tag}`) }).click()
  await expect(staff.getByText('This calendar is paused')).toBeVisible()
  await goSection(staff, 'Settings')
  await staff.getByRole('button', { name: 'Open for booking' }).click()
  await expect(staff.getByText('Customers can register and book.')).toBeVisible()
  const registerUrl = await staff.getByLabel('Registration link').inputValue()
  expect(registerUrl).toContain(`/i/${slug}/register?invite=`)

  // The earliest time may be within the default cancellation notice at the
  // moment the test runs; allow cancelling until the start.
  await goSection(staff, 'Booking rules')
  await staff.getByLabel('Cancel at least (minutes before)').fill('0')
  await staff.getByRole('button', { name: 'Save' }).click()
  await expect(staff.getByText('Saved.')).toBeVisible()

  // A customer registers from the QR link.
  const custCtx = await browser.newContext({ ...info.project.use, baseURL: info.project.use.baseURL })
  const cust = await custCtx.newPage()
  await cust.goto(registerUrl)
  // Sending an empty form moves focus to the name field and marks it invalid.
  await cust.getByRole('button', { name: 'Send registration' }).click()
  await expect(cust.getByLabel('Your name')).toBeFocused()
  await expect(cust.getByLabel('Your name')).toHaveAttribute('aria-invalid', 'true')
  await cust.getByLabel('Your name').fill(`Alice ${tag}`)
  await cust.getByLabel(/Phone number/).fill('0123 456')
  await cust.getByRole('button', { name: 'Send registration' }).click()
  await expect(cust.getByText('Thank you, your registration was received.')).toBeVisible()

  // Staff approve and receive the one-time personal link.
  await goSection(staff, 'Customers')
  await staff.getByRole('button', { name: 'Approve' }).first().click()
  const linkDialog = staff.getByRole('dialog')
  await expect(linkDialog.getByText(`Personal link for Alice ${tag}`)).toBeVisible()
  const personalLink = await linkDialog.getByLabel('Personal link').inputValue()
  expect(personalLink).toContain(`/i/${slug}/access?t=`)
  await linkDialog.getByRole('button', { name: 'Done' }).click()

  // The customer opens the link: token leaves the URL, three steps to book.
  await cust.goto(personalLink)
  await expect(cust).toHaveURL(new RegExp(`/i/${slug}/booking$`))
  await expect(cust.getByRole('heading', { name: 'Bookings', exact: true })).toBeVisible()
  await expect(cust.getByText(`Hello, Alice ${tag}`, { exact: true })).toBeVisible()
  await noHorizontalScroll(cust)
  const firstSlot = cust.getByRole('region', { name: 'Earliest available times' }).getByRole('button').first()
  await activate(firstSlot)
  await expect(cust.getByRole('heading', { name: 'Please check your booking' })).toBeVisible()
  const confirmUrl = cust.url()
  await activate(cust.getByRole('button', { name: 'Yes, book this time' }))
  await expect(cust.getByText('Your booking is confirmed')).toBeVisible()
  const doneUrl = cust.url()

  // Refreshing the result page does not book again; going back to the same
  // confirmation and submitting again returns the same booking.
  await cust.reload()
  await expect(cust.getByText('Your booking is confirmed')).toBeVisible()
  await cust.goto(confirmUrl)
  await activate(cust.getByRole('button', { name: 'Yes, book this time' }))
  await expect(cust).toHaveURL(doneUrl)

  await cust.getByRole('link', { name: 'Back to the start page' }).click()
  await expect(cust.getByText('Next appointment')).toBeVisible()

  // Staff see exactly one appointment today or upcoming.
  await goSection(staff, 'Appointments')
  await staff.getByRole('tab', { name: 'Upcoming' }).click()
  await expect(staff.getByRole('link', { name: `Alice ${tag}` })).toHaveCount(1)

  // The customer switches to Japanese and cancels.
  await activate(cust.getByRole('button', { name: /^Language:/ }))
  await activate(cust.getByRole('dialog').getByRole('button', { name: '日本語' }))
  await expect(cust.getByText('次の予約')).toBeVisible()
  await expect(cust.locator('html')).toHaveAttribute('lang', 'ja')
  await activate(cust.getByRole('button', { name: 'この予約をキャンセルする' }))
  await activate(cust.getByRole('button', { name: 'はい、キャンセルします' }))
  await expect(cust.getByText('予約をキャンセルしました。')).toBeVisible()
  await expect(cust.getByText('これからの予約はありません。')).toBeVisible()

  // The language choice is remembered on reload.
  await cust.reload()
  await expect(cust.getByText('あなたの予約')).toBeVisible()

  // The super admin deletes the calendar after typing its name; the
  // customer's link then explains that it no longer works.
  await goSection(page, 'All calendars')
  await card.getByRole('button', { name: 'Delete calendar' }).click()
  const del = page.getByRole('alertdialog')
  await expect(del.getByText('Customers: 1')).toBeVisible()
  const remove = del.getByRole('button', { name: 'Delete for good' })
  await expect(remove).toBeDisabled()
  await del.getByLabel(/To confirm, type the calendar name/).fill(`Clinic ${tag}`)
  await remove.click()
  await expect(page.getByText(`The calendar “Clinic ${tag}” was deleted.`)).toBeVisible()
  await expect(card).toHaveCount(0)
  await cust.goto(personalLink)
  await expect(cust).toHaveURL(/link-invalid/)
  await cust.goto(`/i/${slug}/booking`)
  await expect(cust.getByText('このリンクは使えません')).toBeVisible()

  await staffCtx.close()
  await custCtx.close()
})

test('invalid personal link shows guidance', async ({ page }) => {
  await page.goto('/i/nowhere/access?t=bogus')
  await expect(page).toHaveURL(/link-invalid/)
  await expect(page.getByText('This link does not work')).toBeVisible()
})

test('back office requires sign in', async ({ page }) => {
  await page.goto('/platform/accounts')
  await expect(page).toHaveURL(/\/account\/login/)
  await page.getByLabel('Login name').fill('admin')
  await page.getByLabel('Password').fill('wrong-password')
  await page.getByRole('button', { name: 'Sign in' }).click()
  await expect(page.getByText('The login name or password is not correct.')).toBeVisible()
})

test('staff edit the repeating schedule, special dates and blocked times', { tag: '@smoke' }, async ({ page }, info) => {
  const slug = `sched-${info.project.name}`
  await login(page, 'admin', 'root-password', 'All calendars')
  // Create the calendar through the API; this test is about the schedule page.
  const csrf = (await page.context().cookies()).find((c) => c.name === 'rm_csrf')!.value
  const res = await page.request.post('/api/v1/platform/instances', {
    headers: { 'X-CSRF-Token': csrf },
    data: { slug, name: `Schedule ${info.project.name}`, timezone: 'Europe/London', default_locale: 'en', contact_kind: 'phone', contact_required: false },
  })
  expect(res.ok()).toBeTruthy()
  await page.goto(`/manage/${slug}/schedule`)
  await page.locator('#cycle-anchor').fill('2026-01-05')

  // Make Monday a day off: the change is unsaved until Save, and kept after
  // a reload.
  const monday = page.getByRole('button', { name: /^Monday/ })
  await expect(monday).not.toContainText('Day off')
  await monday.click()
  const day = page.getByRole('region', { name: 'Monday', exact: true })
  await day.getByRole('checkbox', { name: 'Day off' }).check()
  await monday.click()
  await expect(monday).toContainText('Day off')
  await expect(page.getByText('Unsaved changes')).toBeVisible()
  await page.getByRole('button', { name: /^Tuesday/ }).click()
  const tuesdayEditor = page.getByRole('region', { name: 'Tuesday', exact: true })
  await tuesdayEditor.locator('input[type=time]').first().fill('10:00')
  await tuesdayEditor.locator('summary').click()
  await tuesdayEditor.getByRole('checkbox', { name: 'Wednesday', exact: true }).check()
  await tuesdayEditor.getByRole('button', { name: 'Apply to selected days' }).click()
  await page.getByRole('button', { name: 'Save', exact: true }).click()
  await expect(page.getByText('Saved.')).toBeVisible()
  await expect(page.getByText('Unsaved changes')).toHaveCount(0)
  await page.reload()
  await expect(page.locator('#cycle-anchor')).toHaveValue('2026-01-05')
  await expect(page.getByRole('button', { name: /^Monday/ })).toContainText('Day off')
  for (const name of ['Tuesday', 'Wednesday']) {
    await page.getByRole('button', { name: new RegExp(`^${name}`) }).click()
    await expect(page.getByRole('region', { name, exact: true }).locator('input[type=time]').first()).toHaveValue('10:00')
  }

  // A special date is added (a day off by default) and removed again.
  const special = page.locator('[data-slot=card]').filter({ has: page.getByText('Special dates', { exact: true }) })
  await expect(special.getByText('No special dates.')).toBeVisible()
  await special.getByRole('button', { name: 'Add special date' }).click()
  const dateInput = page.getByRole('dialog').locator('input[type=date]')
  const tomorrow = new Date(`${await dateInput.inputValue()}T12:00:00Z`)
  tomorrow.setUTCDate(tomorrow.getUTCDate() + 1)
  const date = tomorrow.toISOString().slice(0, 10)
  await dateInput.fill(date)
  const savedDate = page.waitForResponse((response) =>
    response.request().method() === 'PUT' && response.url().endsWith(`/overrides/${date}`),
  )
  await page.getByRole('dialog').getByRole('button', { name: 'Save' }).click()
  expect((await savedDate).ok()).toBeTruthy()
  await expect(page.getByRole('dialog')).toHaveCount(0)
  await page.reload()
  await expect(special.getByText('Day off')).toBeVisible()
  await special.getByRole('button', { name: 'Delete' }).click()
  await page.getByRole('alertdialog').getByRole('button', { name: 'Delete' }).click()
  await expect(special.getByText('No special dates.')).toBeVisible()

  // A blocked time is added with a reason and removed again.
  const blocks = page.locator('[data-slot=card]').filter({ has: page.getByText('Blocked times', { exact: true }) })
  await expect(blocks.getByText('No blocked times.')).toBeVisible()
  await blocks.getByRole('button', { name: 'Block a time' }).click()
  const blockDialog = page.getByRole('dialog')
  await blockDialog.getByLabel('Start').fill(`${date}T10:00`)
  await blockDialog.getByLabel('End').fill(`${date}T11:00`)
  await blockDialog.getByLabel('Reason (optional)').fill('Staff meeting')
  await blockDialog.getByRole('button', { name: 'Save' }).click()
  await expect(page.getByRole('dialog')).toHaveCount(0)
  await page.reload()
  await expect(blocks.getByText('Staff meeting')).toBeVisible()
  await expect(blocks).toContainText('10:00')
  await expect(blocks).toContainText('11:00')
  await blocks.getByRole('button', { name: 'Delete' }).click()
  await page.getByRole('alertdialog').getByRole('button', { name: 'Delete' }).click()
  await expect(blocks.getByText('No blocked times.')).toBeVisible()
})
