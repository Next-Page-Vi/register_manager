// Test data for the layout checks, created once through the API by the
// `seed` project (seed.setup.ts) and shared by every layout worker through
// SEED_FILE. Seeding once keeps parallel workers within the login rate limit.
import { expect, type APIRequestContext, type PlaywrightWorkerArgs } from '@playwright/test'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

type PW = PlaywrightWorkerArgs['playwright']

/** Written by seed.setup.ts; e2e/.tmp is cleared when the server starts. */
export const SEED_FILE = join(dirname(fileURLToPath(import.meta.url)), '.tmp', 'seed.json')

export type Seed = {
  slug: string
  instanceId: string
  customerId: string
  appointmentId: string
  invite: string
  staffState: Awaited<ReturnType<APIRequestContext['storageState']>>
  // An operator bound to the calendar, for the pages only operators see.
  operatorState: Awaited<ReturnType<APIRequestContext['storageState']>>
  customerState: Awaited<ReturnType<APIRequestContext['storageState']>>
}

function uuid() {
  return crypto.randomUUID()
}

async function csrf(ctx: APIRequestContext): Promise<string> {
  const cookie = (await ctx.storageState()).cookies.find((c) => c.name === 'rm_csrf')
  if (cookie) return cookie.value
  await ctx.get('/api/v1/account/me')
  return (await ctx.storageState()).cookies.find((c) => c.name === 'rm_csrf')!.value
}

async function call<T = unknown>(ctx: APIRequestContext, method: string, url: string, data?: unknown): Promise<T> {
  const headers: Record<string, string> = {}
  if (method !== 'GET') headers['X-CSRF-Token'] = await csrf(ctx)
  const res = await ctx.fetch(url, { method, data, headers })
  if (!res.ok()) throw new Error(`${method} ${url}: ${res.status()} ${await res.text()}`)
  const text = await res.text()
  return (text ? JSON.parse(text) : undefined) as T
}

async function newApi(playwright: PW, baseURL: string) {
  return playwright.request.newContext({ baseURL })
}

/** Creates a calendar with long names, customers in every state and appointments. */
export async function seed(playwright: PW, baseURL: string, tag: string): Promise<Seed> {
  const root = await newApi(playwright, baseURL)
  await call(root, 'POST', '/api/v1/account/login', { login: 'admin', password: 'root-password' })

  const worker = await call<{ id: string }>(root, 'POST', '/api/v1/platform/accounts', {
    login: `layout-${tag}`,
    name: `Dr Maximiliana Wolfeschlegelsteinhausen ${tag}`,
    password: 'worker-password',
  })
  // A second account so the account list has several cards.
  await call(root, 'POST', '/api/v1/platform/accounts', {
    login: `admin2-${tag}-with-a-rather-long-login-name`,
    name: '山田 花子',
    password: 'admin2-password',
  })
  const slug = `layout-${tag}`
  const inst = await call<{ id: string }>(root, 'POST', '/api/v1/platform/instances', {
    slug,
    name: `Riverside Community Physiotherapy Clinic ${tag}`,
    timezone: 'Europe/London',
    default_locale: 'en',
    contact_kind: 'phone',
    contact_required: false,
  })
  const base = `/api/v1/instances/${inst.id}`
  await call(root, 'PUT', `/api/v1/platform/instances/${inst.id}/operators/${worker.id}`)
  await call(root, 'PUT', `${base}/status`, { status: 'active' })
  // A light page colour, so the header uses dark text.
  await call(root, 'PUT', base, {
    name: `Riverside Community Physiotherapy Clinic ${tag}`,
    timezone: 'Europe/London',
    default_locale: 'en',
    contact_kind: 'phone',
    contact_required: false,
    brand_color: '#f59e0b',
  })
  // Allow cancelling until the start, so the cancel button always shows.
  const policy = await call<Record<string, unknown>>(root, 'GET', `${base}/policy`)
  delete policy.$schema
  await call(root, 'PUT', `${base}/policy`, { ...policy, min_cancellation_notice_minutes: 0 })
  const invite = await call<{ code: string; register_path: string }>(root, 'GET', `${base}/invite`)

  // Customers: approved (the one we book with), pending, rejected, disabled.
  const anon = await newApi(playwright, baseURL)
  const register = async (name: string) => {
    await call(anon, 'POST', `/api/v1/public/instances/${slug}/registrations`, {
      id: uuid(),
      invite: invite.code,
      name,
      contact: '+44 20 7946 0958',
    })
  }
  await register('Alexandrina Featherstonehaugh-Cholmondeley')
  await register('佐藤 美咲')
  await register('Rejected Person')
  await register('Disabled Person')
  const { customers } = await call<{ customers: { id: string; name: string }[] }>(
    root,
    'GET',
    `${base}/customers?status=pending`,
  )
  const byName = (n: string) => customers.find((c) => c.name === n)!.id
  const approved = await call<{ access_path: string }>(
    root,
    'POST',
    `${base}/customers/${byName('Alexandrina Featherstonehaugh-Cholmondeley')}/actions/approve`,
  )
  await call(root, 'POST', `${base}/customers/${byName('Rejected Person')}/actions/reject`)
  await call(root, 'POST', `${base}/customers/${byName('Disabled Person')}/actions/approve`)
  await call(root, 'POST', `${base}/customers/${byName('Disabled Person')}/actions/disable`)
  const customerId = byName('Alexandrina Featherstonehaugh-Cholmondeley')
  await call(root, 'PUT', `${base}/customers/${customerId}/note`, {
    note: 'Prefers morning appointments. Uses a walking frame; please keep the ground-floor room free. 電話は午後のみ。',
  })

  // Special date and blocked time so the schedule page has every section.
  const now = new Date()
  const day = (n: number) => new Date(now.getTime() + n * 86400_000).toISOString().slice(0, 10)
  await call(root, 'PUT', `${base}/overrides/${day(20)}`, { periods: [{ start: 600, end: 720 }] })
  await call(root, 'PUT', `${base}/overrides/${day(21)}`, { periods: [] })
  await call(root, 'POST', `${base}/blocks`, {
    starts_at: `${day(25)}T09:00:00Z`,
    ends_at: `${day(25)}T12:00:00Z`,
    reason: 'Staff training on the new treatment equipment',
  })

  // The customer opens the personal link and books two times.
  const cust = await newApi(playwright, baseURL)
  const access = await cust.get(approved.access_path, { maxRedirects: 0 })
  expect(access.status()).toBe(303)
  const avail = await call<{ items: { starts_at: string; ends_at: string }[] }>(
    cust,
    'GET',
    `/api/v1/customer/${slug}/availability?limit=20`,
  )
  const booked = await call<{ id: string }>(cust, 'POST', `/api/v1/customer/${slug}/appointments`, {
    id: uuid(),
    starts_at: avail.items[0].starts_at,
    ends_at: avail.items[0].ends_at,
  })
  // Staff book another for the same customer a few slots later.
  await call(root, 'POST', `${base}/appointments`, {
    id: uuid(),
    customer_id: customerId,
    starts_at: avail.items[6].starts_at,
    ends_at: avail.items[6].ends_at,
  })


  const operator = await newApi(playwright, baseURL)
  await call(operator, 'POST', '/api/v1/account/login', { login: `layout-${tag}`, password: 'worker-password' })

  const result: Seed = {
    slug,
    instanceId: inst.id,
    customerId,
    appointmentId: booked.id,
    invite: invite.register_path,
    // The super admin sees every page, including platform pages.
    staffState: await root.storageState(),
    operatorState: await operator.storageState(),
    customerState: await cust.storageState(),
  }
  await Promise.all([root.dispose(), anon.dispose(), cust.dispose(), operator.dispose()])
  return result
}
