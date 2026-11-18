import { useState } from 'react'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const mocks = vi.hoisted(() => ({ GET: vi.fn(), POST: vi.fn() }))
vi.mock('@/api/client', async (orig) => ({ ...(await orig<typeof import('@/api/client')>()), api: mocks }))

import { RecipientInput } from './RecipientInput'
import { rankCandidates, type Candidate } from './recipientCandidates'

const ok = (data: unknown) => ({ data, response: new Response(null, { status: 200 }) })
const entry = (address: string, extra: Record<string, unknown> = {}) => ({
  address,
  role: 'dev',
  description: '',
  online: [],
  subagents: [],
  ...extra,
})

const cand = (address: string, kind: Candidate['kind'], online = false): Candidate => ({ address, kind, online })

describe('rankCandidates', () => {
  it('ranks prefix before substring, online before offline, identities before subagents before scopes', () => {
    const all = [
      cand('github.com/acme/rev-repo', 'repo'),
      cand('x@reviewer@github.com/acme/api', 'subagent', true),
      cand('prereview@github.com/acme/api', 'identity', true), // substring only
      cand('reviewer@github.com/acme/api', 'identity', false),
      cand('review2@github.com/acme/api', 'identity', true),
      cand('github.com/acme/review', 'org'),
    ]
    expect(rankCandidates(all, 'review').map((c) => c.address)).toEqual([
      'review2@github.com/acme/api', // prefix, online
      'reviewer@github.com/acme/api', // prefix, offline
      'prereview@github.com/acme/api', // substring, online
      'x@reviewer@github.com/acme/api', // substring, subagent
      'github.com/acme/review', // substring, scope
    ]) // rev-repo does not contain "review"
  })

  it('prefers the name before @ and an exact match first', () => {
    const all = [cand('ops@github.com/acme/api', 'identity'), cand('ops@github.com/acme', 'identity'), cand('deploy@ops.net/a', 'identity')]
    expect(rankCandidates(all, 'ops@github.com/acme').map((c) => c.address)[0]).toBe('ops@github.com/acme')
    expect(rankCandidates(all, 'ops').map((c) => c.address)).toEqual(['ops@github.com/acme', 'ops@github.com/acme/api', 'deploy@ops.net/a'])
  })

  it('excludes chipped addresses, caps at 8, and returns nothing for an empty query', () => {
    const many = Array.from({ length: 12 }, (_, i) => cand(`a${i}@github.com/acme`, 'identity'))
    expect(rankCandidates(many, 'a')).toHaveLength(8)
    expect(rankCandidates(many, 'a', ['a0@github.com/acme']).map((c) => c.address)).not.toContain('a0@github.com/acme')
    expect(rankCandidates(many, '  ')).toEqual([])
  })
})

function Host({ initial = [] as string[] }) {
  const [v, setV] = useState(initial)
  return (
    <>
      <RecipientInput label="To" value={v} onChange={setV} />
      <output data-testid="chips">{v.join('|')}</output>
      <button>after</button>
    </>
  )
}

function mount(ui = <Host />) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(<QueryClientProvider client={qc}>{ui}</QueryClientProvider>)
}

beforeEach(() => {
  vi.clearAllMocks()
  mocks.GET.mockImplementation(async (path: string) => {
    if (path === '/api/roster')
      return ok({
        items: [
          entry('reviewer@github.com/acme/api', { role: 'reviewer', online: [{}], subagents: ['lint'] }),
          entry('release@github.com/acme', { role: 'release manager' }),
          entry('gone@github.com/acme/api', { retired_at: '2026-11-01T00:00:00Z' }),
        ],
      })
    if (path === '/api/tree')
      return ok({ orgs: [{ address: 'github.com/acme', identities: [], repos: [{ address: 'github.com/acme/api', identities: [] }] }] })
    return { error: {}, response: new Response(null, { status: 404 }) }
  })
})

describe('RecipientInput type-ahead', () => {
  it('moves with arrows, accepts with Enter, exposes combobox ARIA', async () => {
    const user = userEvent.setup()
    mount()
    const input = screen.getByRole('combobox', { name: 'To recipients' })
    await user.type(input, 're')
    const opts = await screen.findAllByRole('option')
    // identities (online first), then nothing else matches "re" as prefix but "reviewer"/"release"
    expect(opts.map((o) => o.textContent)).toEqual(
      expect.arrayContaining([expect.stringContaining('reviewer'), expect.stringContaining('release')]),
    )
    expect(opts[0]).toHaveAttribute('aria-selected', 'true')
    expect(opts[0]).toHaveTextContent('reviewer')
    expect(opts[0]).toHaveTextContent('reviewer') // role label
    expect(screen.getByRole('listbox', { name: 'To suggestions' })).toBeInTheDocument()
    await user.keyboard('{ArrowDown}')
    await waitFor(() => expect(screen.getAllByRole('option')[1]).toHaveAttribute('aria-selected', 'true'))
    expect(input.getAttribute('aria-activedescendant')).toBe(screen.getAllByRole('option')[1]!.id)
    await user.keyboard('{Enter}')
    expect(screen.getByTestId('chips')).toHaveTextContent('release@github.com/acme')
    expect(input).toHaveValue('')
  })

  it('Tab accepts the highlighted suggestion instead of committing typed text', async () => {
    const user = userEvent.setup()
    mount()
    const input = screen.getByRole('combobox', { name: 'To recipients' })
    await user.type(input, 'rev')
    await screen.findAllByRole('option')
    await user.keyboard('{Tab}')
    expect(screen.getByTestId('chips')).toHaveTextContent('reviewer@github.com/acme/api')
    expect(input).toHaveFocus()
  })

  it('Escape closes the list; Enter then commits the typed text', async () => {
    const user = userEvent.setup()
    mount()
    const input = screen.getByRole('combobox', { name: 'To recipients' })
    await user.type(input, 'ops@github.com/acme/api')
    await user.type(input, '{Escape}')
    expect(screen.queryAllByRole('option')).toHaveLength(0)
    await user.keyboard('{Enter}')
    expect(screen.getByTestId('chips')).toHaveTextContent('ops@github.com/acme/api')
  })

  it('with no matches Enter and Tab still commit typed text', async () => {
    const user = userEvent.setup()
    mount()
    const input = screen.getByRole('combobox', { name: 'To recipients' })
    await user.type(input, 'zz@github.com/x/y{Enter}')
    expect(screen.getByTestId('chips')).toHaveTextContent('zz@github.com/x/y')
    await user.type(input, 'qq@github.com/x/y{Tab}')
    expect(screen.getByTestId('chips')).toHaveTextContent('zz@github.com/x/y|qq@github.com/x/y')
  })

  it('offers subagents, scopes and operator but not retired or already-chipped addresses', async () => {
    const user = userEvent.setup()
    mount(<Host initial={['reviewer@github.com/acme/api']} />)
    const input = screen.getByRole('combobox', { name: 'To recipients' })
    await user.type(input, 'acme')
    await screen.findAllByRole('option')
    const names = screen.getAllByRole('option').map((o) => o.textContent ?? '')
    expect(names.some((n) => n.includes('lint@reviewer'))).toBe(true) // subagent
    expect(names.some((n) => n.includes('github.com/acme/api') && n.includes('repo'))).toBe(true)
    expect(names.some((n) => n.includes('org'))).toBe(true)
    expect(names.some((n) => n.includes('gone@'))).toBe(false)
    expect(names.filter((n) => /^reviewer@/.test(n) || n.startsWith('reviewer'))).toHaveLength(0)
    await user.clear(input)
    await user.type(input, 'oper')
    expect(await screen.findByRole('option', { name: /operator/ })).toBeInTheDocument()
  })

  it('selects with the mouse without committing the typed text first', async () => {
    const user = userEvent.setup()
    mount()
    await user.type(screen.getByRole('combobox', { name: 'To recipients' }), 'rele')
    await user.click(await screen.findByRole('option', { name: /release/ }))
    expect(screen.getByTestId('chips').textContent).toBe('release@github.com/acme')
  })
})
