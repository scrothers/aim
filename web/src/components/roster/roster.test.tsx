import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { useState } from 'react'
import { render } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { RosterEntry } from '@/api/roster'
import { renderWithRouter } from '@/test/utils'

const mocks = vi.hoisted(() => ({ GET: vi.fn(), POST: vi.fn(), PATCH: vi.fn() }))
vi.mock('@/api/client', async (orig) => ({ ...(await orig<typeof import('@/api/client')>()), api: mocks }))

import { OrgChart } from '../orgchart/OrgChart'
import { RosterGrid } from './RosterGrid'

const ok = (data: unknown) => ({ data, response: new Response(null, { status: 200 }) })
const fail = (status: number, detail: string) => ({ error: { detail }, response: new Response(null, { status }) })

const entry = (address: string, extra: Partial<RosterEntry> = {}): RosterEntry => ({
  address,
  role: 'dev',
  description: 'line one\nline two',
  online: [],
  subagents: [],
  ...extra,
})

const live = entry('reviewer@github.com/acme/api', {
  role: 'review',
  reports_to: 'coordinator@github.com/acme',
  online: [{ session_id: 's1', machine: 'pi-03', client: 'codex-cli', signed_on_at: '2026-11-17T10:00:00Z' }],
  subagents: ['scanner'],
})
const lead = entry('coordinator@github.com/acme', { role: 'lead', reports_to: 'operator' })
const gone = entry('old@github.com/acme/api', { retired_at: '2026-11-16T10:00:00Z' })

function wrap(ui: React.ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return <QueryClientProvider client={qc}>{ui}</QueryClientProvider>
}

function Harness({ initial }: { initial?: string }) {
  const [edit, setEdit] = useState(initial)
  return <RosterGrid editing={edit} onEdit={setEdit} />
}

beforeEach(() => {
  vi.clearAllMocks()
  mocks.GET.mockImplementation(async (path: string, o?: { params?: { query?: { include_retired?: boolean } } }) => {
    if (path === '/api/roster') return ok({ items: o?.params?.query?.include_retired ? [live, lead, gone] : [live, lead] })
    return fail(404, 'nope')
  })
  mocks.PATCH.mockResolvedValue(ok({}))
  mocks.POST.mockImplementation(async (path: string, o?: { body?: { markdown?: string } }) =>
    path === "/api/render" ? ok({ html: `<p><strong>${o?.body?.markdown?.split("\n")[0]}</strong></p>` }) : ok({}),
  )
})

describe('RosterGrid', () => {
  it('renders cards grouped by org/repo with presence and subagents', async () => {
    render(wrap(<Harness />))
    const cards = await screen.findAllByTestId('roster-card')
    expect(cards).toHaveLength(2)
    expect(screen.getByRole('heading', { name: 'github.com/acme/api' })).toBeInTheDocument()
    const card = cards.find((c) => within(c).queryByText('review'))!
    expect(within(card).getByText('pi-03 · Codex')).toBeInTheDocument()
    for (const c of cards) expect(within(c).getByTestId('robot-avatar')).toBeInTheDocument()
    expect(within(card).getByText('scanner')).toBeInTheDocument()
    expect(within(card).getByText(/reports to/)).toBeInTheDocument()
    expect(within(card).getByText(/line one/)).toBeInTheDocument()
  })

  it('shows retired identities only when toggled', async () => {
    const user = userEvent.setup()
    render(wrap(<Harness />))
    await screen.findAllByTestId('roster-card')
    expect(screen.queryByText(/Retired/)).toBeNull()
    await user.click(screen.getByLabelText('Show retired'))
    await screen.findByText(/Retired/)
    expect(screen.getAllByTestId('roster-card')).toHaveLength(3)
    expect(mocks.GET.mock.calls.some((c) => c[1]?.params?.query?.include_retired === true)).toBe(true)
    expect(screen.getByRole('button', { name: 'Unretire' })).toBeInTheDocument()
  })

  it('saves the edit payload', async () => {
    const user = userEvent.setup()
    render(wrap(<Harness />))
    await screen.findAllByTestId('roster-card')
    await user.click(within(screen.getAllByTestId('roster-card')[1]!).getByRole('button', { name: /Edit/ }))
    const role = await screen.findByLabelText('Role')
    await user.clear(role)
    await user.type(role, 'security')
    const reports = screen.getByLabelText(/Reports to/)
    await user.clear(reports)
    await user.type(reports, 'operator')
    await user.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(mocks.PATCH).toHaveBeenCalled())
    expect(mocks.PATCH.mock.calls[0]![0]).toBe('/api/identities')
    expect(mocks.PATCH.mock.calls[0]![1]).toMatchObject({
      params: { query: { address: 'reviewer@github.com/acme/api' } },
      body: { role: 'security', description: 'line one\nline two', reports_to: 'operator' },
    })
  })

  it('opens the sheet from the edit search param and shows cycle errors inline', async () => {
    const user = userEvent.setup()
    mocks.PATCH.mockResolvedValue(fail(422, 'coordinator@github.com/acme would report to itself through reviewer@github.com/acme/api'))
    render(wrap(<Harness initial="coordinator@github.com/acme" />))
    const field = await screen.findByLabelText(/Reports to/)
    expect(field).toHaveValue('operator')
    await user.clear(field)
    await user.type(field, 'reviewer@github.com/acme/api')
    await user.click(screen.getByRole('button', { name: 'Save' }))
    expect(await screen.findByRole('alert')).toHaveTextContent(/report to itself/)
    expect(screen.getByLabelText('Role')).toBeInTheDocument() // sheet stays open
  })

  it('confirms before retiring', async () => {
    const user = userEvent.setup()
    render(wrap(<Harness />))
    await screen.findAllByTestId('roster-card')
    await user.click(within(screen.getAllByTestId('roster-card')[0]!).getByRole('button', { name: 'Retire' }))
    const retires = () => mocks.POST.mock.calls.filter((c) => c[0] === '/api/identities/retire')
    expect(retires()).toHaveLength(0)
    await user.click(within(await screen.findByRole('dialog')).getByRole('button', { name: 'Cancel' }))
    expect(retires()).toHaveLength(0)
    await user.click(within(screen.getAllByTestId('roster-card')[0]!).getByRole('button', { name: 'Retire' }))
    await user.click(within(await screen.findByRole('dialog')).getByRole('button', { name: 'Retire' }))
    await waitFor(() => expect(retires()).toHaveLength(1))
    expect(retires()[0]![1]).toMatchObject({ params: { query: { address: 'coordinator@github.com/acme' } } })
  })

  it('renders role descriptions as Markdown through the server render', async () => {
    mocks.GET.mockImplementation(async () => ok({ items: [entry('a@github.com/acme', { description: '**release train**\n- item' })] }))
    render(wrap(<Harness />))
    await screen.findByTestId('role-description')
    await waitFor(() => expect(screen.getByTestId('role-description').querySelector('strong')).toHaveTextContent('**release train**'))
    expect(mocks.POST).toHaveBeenCalledWith('/api/render', { body: { markdown: '**release train**\n- item' } })
  })

  it('shows flattened text until the render arrives, and plain text if it fails', async () => {
    mocks.POST.mockImplementation(async () => fail(500, 'boom'))
    mocks.GET.mockImplementation(async () => ok({ items: [entry('a@github.com/acme', { description: '**release train**' })] }))
    render(wrap(<Harness />))
    expect((await screen.findByTestId('role-description')).querySelector('strong')).toBeNull()
    await waitFor(() => expect(screen.getByTestId('role-description')).toHaveTextContent('**release train**'))
  })

  it('makes Retire the destructive action and Edit secondary', async () => {
    render(wrap(<Harness />))
    const card = (await screen.findAllByTestId('roster-card'))[0]!
    expect(within(card).getByRole('button', { name: 'Retire' }).className).toContain('bg-destructive')
    expect(within(card).getByRole('button', { name: /Edit/ }).className).toContain('bg-code')
  })
})

describe('OrgChart', () => {
  it('draws cards and connectors and links to the roster edit sheet', async () => {
    mocks.GET.mockResolvedValue(
      ok({
        nodes: [
          { address: 'operator', operator: true, role: '', description: '', online: [] },
          { address: lead.address, role: 'lead', description: '', reports_to: 'operator', online: [] },
          { address: live.address, role: 'review', description: '', reports_to: lead.address, online: live.online },
        ],
        edges: [],
      }),
    )
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    await renderWithRouter(
      <QueryClientProvider client={qc}>
        <OrgChart />
      </QueryClientProvider>,
      '/org-chart',
    )
    const card = await screen.findByRole('link', { name: `Edit ${live.address}` })
    expect(card).toHaveAttribute('href', expect.stringContaining('/roster'))
    expect(card.getAttribute('href')).toContain('edit=')
    expect(within(card).getByRole('img', { name: 'online' })).toBeInTheDocument()
    expect(within(screen.getByRole('link', { name: `Edit ${lead.address}` })).getByRole('img', { name: 'offline' })).toBeInTheDocument()
    expect(screen.getByText('operator')).toBeInTheDocument()
    expect(document.querySelectorAll('path[data-edge]')).toHaveLength(2)
  })

  it('zooms with the controls and fits to the width', async () => {
    mocks.GET.mockResolvedValue(
      ok({
        nodes: [
          { address: 'operator', operator: true, role: '', description: '', online: [] },
          { address: lead.address, role: 'lead', description: '', reports_to: 'operator', online: [] },
        ],
        edges: [],
      }),
    )
    const user = userEvent.setup()
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    await renderWithRouter(
      <QueryClientProvider client={qc}>
        <OrgChart />
      </QueryClientProvider>,
      '/org-chart',
    )
    await screen.findByRole('link', { name: `Edit ${lead.address}` })
    const level = screen.getByLabelText('Zoom level')
    expect(level).toHaveTextContent('100%')
    await user.click(screen.getByRole('button', { name: 'Zoom out' }))
    expect(level).toHaveTextContent('75%')
    await user.click(screen.getByRole('button', { name: 'Zoom in' }))
    await user.click(screen.getByRole('button', { name: 'Zoom in' }))
    expect(level).toHaveTextContent('125%')
    await user.click(screen.getByRole('button', { name: 'Reset' }))
    expect(level).toHaveTextContent('100%')
    await user.click(screen.getByRole('button', { name: 'Fit width' }))
    expect(level).toHaveTextContent('100%') // jsdom has no layout: width 0 means "nothing to fit"
  })
})
