import { describe, expect, it, vi } from 'vitest'
import { createApi } from './client'

function setup() {
  const fetchMock = vi.fn(
    async (...args: [Request]) =>
      new Response(JSON.stringify({ n: args.length }), { status: 200, headers: { 'Content-Type': 'application/json' } }),
  )
  const api = createApi(fetchMock as unknown as typeof fetch, 'http://aim.test')
  const lastRequest = () => fetchMock.mock.calls[fetchMock.mock.calls.length - 1]![0] as Request
  return { api, lastRequest }
}

describe('api client guard headers', () => {
  it('does not add guard headers to GET requests', async () => {
    const { api, lastRequest } = setup()
    await api.GET('/api/tree')
    const req = lastRequest()
    expect(req.method).toBe('GET')
    expect(req.headers.get('X-AIM-Request')).toBeNull()
  })

  it('adds Content-Type and X-AIM-Request to mutations', async () => {
    const { api, lastRequest } = setup()
    await api.POST('/api/messages', { body: { to: ['a@github.com/o'], subject: 's', body: 'b' } as never })
    const req = lastRequest()
    expect(req.method).toBe('POST')
    expect(req.headers.get('X-AIM-Request')).toBe('1')
    expect(req.headers.get('Content-Type')).toBe('application/json')
  })

  it('adds the headers to PATCH too', async () => {
    const { api, lastRequest } = setup()
    await api.PATCH('/api/identities', { params: { query: { address: 'a@github.com/o' } }, body: {} as never })
    expect(lastRequest().headers.get('X-AIM-Request')).toBe('1')
  })
})
