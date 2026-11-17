import createClient, { type Middleware } from 'openapi-fetch'
import type { paths } from './schema'

const SAFE = new Set(['GET', 'HEAD', 'OPTIONS'])

/** Adds the browser-hygiene headers the daemon requires on mutating requests. */
export const guardHeaders: Middleware = {
  onRequest({ request }) {
    if (!SAFE.has(request.method.toUpperCase())) {
      request.headers.set('Content-Type', 'application/json')
      request.headers.set('X-AIM-Request', '1')
    }
    return request
  },
}

export function createApi(fetchImpl?: typeof globalThis.fetch, baseUrl = '') {
  const client = createClient<paths>({ baseUrl, ...(fetchImpl ? { fetch: fetchImpl } : {}) })
  client.use(guardHeaders)
  return client
}

export const api = createApi()

export class ApiError extends Error {
  readonly status: number
  constructor(status: number, message: string) {
    super(message)
    this.status = status
  }
}

/** Unwraps an openapi-fetch result, throwing ApiError on failure. */
export function unwrap<T>(res: { data?: T; error?: unknown; response: Response }): T {
  if (res.data !== undefined) return res.data
  const e = res.error as { detail?: string; title?: string } | undefined
  throw new ApiError(res.response.status, e?.detail ?? e?.title ?? res.response.statusText)
}
