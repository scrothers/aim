import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { SearchPageParams } from '@/api/mail'
import { treeFixture } from '@/test/fixtures'
import { renderWithRouter } from '@/test/utils'

const mocks = vi.hoisted(() => ({ GET: vi.fn(), POST: vi.fn() }))
vi.mock('@/api/client', async (orig) => ({ ...(await orig<typeof import('@/api/client')>()), api: mocks }))

import { SearchPage } from './SearchPage'

const ok = (data: unknown) => ({ data, response: new Response(null, { status: 200 }) })

const hit = (id: string, extra = {}) => ({
  message_id: id,
  thread_id: `t${id}`,
  subject: `Subject ${id}`,
  snippet: 'unused',
  from: 'reviewer@github.com/acme/api',
  mailbox: 'operator',
  created_at: new Date().toISOString(),
  rank: 1,
  segments: [
    { text: 'ship the ', match: false },
    { text: 'zebrafish', match: true },
    { text: ' release', match: false },
  ],
  ...extra,
})

beforeEach(() => {
  vi.clearAllMocks()
  mocks.GET.mockImplementation(async (path: string) => {
    if (path === '/api/tree') return ok({ orgs: treeFixture })
    if (path === '/api/search') return ok({ hits: [hit('1')] })
    return ok({})
  })
})

async function mount(params: SearchPageParams, onSearch = vi.fn()) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  await renderWithRouter(
    <QueryClientProvider client={qc}>
      <SearchPage params={params} onSearch={onSearch} />
    </QueryClientProvider>,
  )
  return onSearch
}

const searchCalls = () => mocks.GET.mock.calls.filter((c) => c[0] === '/api/search')

describe('SearchPage', () => {
  it('renders matches as <mark> elements and escapes markup in text', async () => {
    mocks.GET.mockImplementation(async (path: string) =>
      path === '/api/search'
        ? ok({ hits: [hit('1', { segments: [{ text: '<img src=x onerror=alert(1)> ', match: false }, { text: '<b>bold</b>', match: true }] })] })
        : ok({ orgs: [] }),
    )
    await mount({ q: 'bold' })
    const snippet = await screen.findByTestId('hit-snippet')
    expect(snippet.querySelectorAll('mark')).toHaveLength(1)
    expect(snippet.querySelector('mark')).toHaveTextContent('<b>bold</b>')
    expect(snippet.querySelector('img')).toBeNull()
    expect(snippet.querySelector('b')).toBeNull()
    expect(snippet).toHaveTextContent('<img src=x onerror=alert(1)>')
  })

  it('shows sender, mailbox chip and links to the thread', async () => {
    await mount({ q: 'zebrafish' })
    const row = await screen.findByTestId('search-hit')
    expect(within(row).getByTestId('robot-avatar')).toBeInTheDocument()
    expect(within(row).getByTestId('hit-mailbox')).toHaveTextContent('operator')
    expect(within(row).getByText('Subject 1')).toBeInTheDocument()
    const href = within(row).getByRole('link').getAttribute('href')!
    expect(href).toContain('/mail')
    expect(href).toContain('thread=t1')
    expect(href).toContain('state=all')
  })

  it('sends scope, sender, inclusive date range and state to the API', async () => {
    await mount({ q: 'x', scope: 'operator', from: 'reviewer@github.com/acme/api', since: '2026-11-01', until: '2026-11-03', state: 'archived' })
    await screen.findByTestId('search-hit')
    const q = searchCalls()[0]![1].params.query
    expect(q).toMatchObject({ q: 'x', address: 'operator', from: 'reviewer@github.com/acme/api', state: 'archived' })
    expect(Date.parse(q.since)).toBe(new Date(2026, 10, 1).getTime())
    expect(Date.parse(q.until)).toBe(new Date(2026, 10, 4).getTime())
  })

  it('does not query without text, and submits the form fields', async () => {
    const user = userEvent.setup()
    const onSearch = await mount({ q: '' })
    expect(searchCalls()).toHaveLength(0)
    await user.type(screen.getByRole('textbox', { name: 'Search' }), 'deploy')
    await user.type(screen.getByLabelText('Mailbox'), 'operator')
    await user.selectOptions(screen.getByLabelText('State'), 'unread')
    await user.click(screen.getByRole('button', { name: 'Search' }))
    expect(onSearch).toHaveBeenCalledWith({ q: 'deploy', scope: 'operator', state: 'unread' })
  })

  it('offers mailbox autocomplete from the tree', async () => {
    await mount({ q: '' })
    await waitFor(() => expect(document.querySelectorAll('datalist option').length).toBeGreaterThan(3))
    const values = [...document.querySelectorAll('datalist option')].map((o) => o.getAttribute('value'))
    expect(values).toContain('github.com/acme/api')
    expect(values).toContain('reviewer@github.com/acme/api')
  })

  it('loads more with the cursor', async () => {
    const user = userEvent.setup()
    mocks.GET.mockImplementation(async (path: string, o?: { params?: { query?: { cursor?: string } } }) => {
      if (path !== '/api/search') return ok({ orgs: [] })
      return o?.params?.query?.cursor ? ok({ hits: [hit('2')] }) : ok({ hits: [hit('1')], next_cursor: 'c1' })
    })
    await mount({ q: 'x' })
    await user.click(await screen.findByRole('button', { name: 'Load more' }))
    await waitFor(() => expect(screen.getAllByTestId('search-hit')).toHaveLength(2))
    expect(searchCalls()[1]![1].params.query.cursor).toBe('c1')
  })

  it('saves the search as a filter', async () => {
    const user = userEvent.setup()
    mocks.POST.mockImplementation(async () => ok({ id: 'f', name: 'n', params: {} }))
    await mount({ q: 'deploy', scope: 'operator' })
    await user.click(await screen.findByRole('button', { name: 'Save search' }))
    await user.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(mocks.POST).toHaveBeenCalledWith('/api/filters', { body: { name: 'deploy', params: { q: 'deploy', scope: 'operator' } } }))
  })
})
