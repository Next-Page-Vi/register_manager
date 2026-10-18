import createClient, { type Middleware } from 'openapi-fetch'
import createQueryClient from 'openapi-react-query'
import type { components, paths } from './schema'
import { clearSessionCache } from './queries'

export type Schemas = components['schemas']

const CSRF_COOKIE = 'rm_csrf'
const CSRF_HEADER = 'X-CSRF-Token'

function readCookie(name: string): string | null {
  for (const part of document.cookie.split(';')) {
    const [key, ...value] = part.trim().split('=')
    if (key === name) return decodeURIComponent(value.join('='))
  }
  return null
}

/** Only HTTP failures have a status. */
export class ApiError extends Error {
  constructor(
    readonly status: number,
    readonly code: string,
    readonly params: Record<string, unknown> = {},
    readonly details?: unknown,
  ) {
    super(code)
  }
}

export class NetworkError extends Error {
  readonly code = 'network_error'
}

function transportError(error: unknown): Error {
  if (error instanceof DOMException && error.name === 'AbortError') return error
  return new NetworkError('Request failed', { cause: error })
}

export function errorInfo(error: unknown): { code: string; params: Record<string, unknown> } {
  if (error instanceof ApiError) return error
  return { code: error instanceof NetworkError ? 'network_error' : 'unknown_error', params: {} }
}

type Problem = { code?: string; params?: Record<string, unknown> | null; details?: unknown }
export type SessionScope = { kind: 'account' } | { kind: 'customer'; slug: string }
export const UNAUTHENTICATED_EVENT = 'rm:unauthenticated'

const transport: Middleware = {
  async onRequest({ request }) {
    if (request.method === 'GET' || request.method === 'HEAD') return request
    let token = readCookie(CSRF_COOKIE)
    if (token === null) {
      await fetch('/api/v1/account/me', { credentials: 'same-origin', signal: request.signal }).catch(
        (error: unknown) => {
          throw transportError(error)
        },
      )
      token = readCookie(CSRF_COOKIE)
    }
    if (token !== null) request.headers.set(CSRF_HEADER, token)
    return request
  },
  async onResponse({ request, response }) {
    const path = new URL(request.url).pathname
    if (response.ok) {
      if (/\/account\/(login|logout|password)$/.test(path) || /\/customer\/[^/]+\/logout$/.test(path)) {
        await clearSessionCache()
      }
      return response
    }
    const problem: Problem = await response
      .clone()
      .json()
      .catch(() => ({}))
    const error = new ApiError(response.status, problem.code ?? 'error', problem.params ?? {}, problem.details)
    if (response.status === 401) {
      const customer = path.match(/^\/api\/v1\/customer\/([^/]+)\//)
      const scope: SessionScope = customer
        ? { kind: 'customer', slug: decodeURIComponent(customer[1]) }
        : { kind: 'account' }
      window.dispatchEvent(new CustomEvent<SessionScope>(UNAUTHENTICATED_EVENT, { detail: scope }))
    }
    throw error
  },
  onError({ error }) {
    return transportError(error)
  },
}

const api = createClient<paths>({ credentials: 'same-origin' })
api.use(transport)
export const $api = createQueryClient(api)
