import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mailFilterParams, mailSearchFromParams } from '@/api/filters'
import { renderWithRouter } from '@/test/utils'

const mocks = vi.hoisted(() => ({ GET: vi.fn(), POST: vi.fn(), PATCH: vi.fn(), DELETE: vi.fn() }))
vi.mock('@/api/client', async (orig) => ({ ...(await orig<typeof import('@/api/client')>()), api: mocks }))

import { SavedViews } from './SavedViews'

const ok = (data: unknown) => ({ data, response: new Response(null, { status: 200 }) })

beforeEach(() => {
  vi.clearAllMocks()
  mocks.GET.mockImplementation(async () =>
    ok({ items: [{ id: 'f1', name: 'Reviewer inbox', params: { mailbox: 'reviewer@github.com/acme/api', state: 'unread' } }] }),
  )
  mocks.PATCH.mockImplementation(async () => ok({ id: 'f1', name: 'x', params: {} }))
  mocks.DELETE.mockImplementation(async () => ({ response: new Response(null, { status: 204 }) }))
})

async function mount() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  await renderWithRouter(
    <QueryClientProvider client={qc}>
      <SavedViews />
    </QueryClientProvider>,
  )
}

describe('saved views', () => {
  it('lists filters as links that reproduce the view', async () => {
    await mount()
    const link = await screen.findByRole('link', { name: 'Reviewer inbox' })
    expect(link.getAttribute('href')).toContain('mailbox=reviewer%40github.com%2Facme%2Fapi')
    expect(link.getAttribute('href')).toContain('state=unread')
  })

  it('renames through the menu', async () => {
    const user = userEvent.setup()
    await mount()
    await user.click(await screen.findByRole('button', { name: 'Actions for Reviewer inbox' }))
    await user.click(await screen.findByRole('menuitem', { name: 'Rename' }))
    const dlg = await screen.findByRole('dialog', { name: 'Rename view' })
    await user.clear(within(dlg).getByLabelText('Name'))
    await user.type(within(dlg).getByLabelText('Name'), 'Triage')
    await user.click(within(dlg).getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(mocks.PATCH).toHaveBeenCalledWith('/api/filters', { params: { query: { id: 'f1' } }, body: { name: 'Triage' } }))
  })

  it('deletes through the menu', async () => {
    const user = userEvent.setup()
    await mount()
    await user.click(await screen.findByRole('button', { name: 'Actions for Reviewer inbox' }))
    await user.click(await screen.findByRole('menuitem', { name: 'Delete' }))
    await waitFor(() => expect(mocks.DELETE).toHaveBeenCalledWith('/api/filters', { params: { query: { id: 'f1' } } }))
  })

  it('round-trips mail search params and drops the open thread', () => {
    const s = { view: 'operator', state: 'all', muted: true, thread: 't1' } as const
    const p = mailFilterParams(s)
    expect(p).toEqual({ view: 'operator', state: 'all', muted: 'true' })
    expect(mailSearchFromParams(p)).toEqual({ view: 'operator', state: 'all', muted: true })
    expect(mailSearchFromParams({ view: 'bogus', state: 'nope' })).toEqual({})
  })
})
