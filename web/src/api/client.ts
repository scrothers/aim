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
  /** "Did you mean" addresses from a 404 on an unknown recipient. */
  readonly suggestions: string[]
  constructor(status: number, message: string, suggestions: string[] = []) {
    super(message)
    this.status = status
    this.suggestions = suggestions
  }
}

interface ProblemBody {
  detail?: string
  title?: string
  errors?: Array<{ location?: string; value?: unknown }> | null
}

/** For operations that answer 204: throws ApiError on failure, returns nothing. */
export function unwrapVoid(res: { error?: unknown; response: Response }): void {
  if (res.response.ok) return
  unwrap({ error: res.error, response: res.response })
}

/** Unwraps an openapi-fetch result, throwing ApiError on failure. */
export function unwrap<T>(res: { data?: T; error?: unknown; response: Response }): T {
  if (res.data !== undefined) return res.data
  const e = res.error as ProblemBody | undefined
  const raw = e?.errors?.find((d) => d.location === 'suggestions')?.value
  const suggestions = Array.isArray(raw) ? raw.filter((s): s is string => typeof s === 'string') : []
  throw new ApiError(res.response.status, e?.detail ?? e?.title ?? res.response.statusText, suggestions)
}
