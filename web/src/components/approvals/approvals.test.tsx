import { act, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { Approval } from '@/api/approvals'
import { reloadNotifyPref, setNotificationsEnabled } from '@/lib/notify'

const mocks = vi.hoisted(() => ({ GET: vi.fn(), POST: vi.fn(), PUT: vi.fn() }))
vi.mock('@/api/client', async (orig) => ({ ...(await orig<typeof import('@/api/client')>()), api: mocks }))

import { OperatorAlerts } from '@/components/shell/OperatorAlerts'
import { renderWithRouter } from '@/test/utils'
import { ApprovalsPage } from './ApprovalsPage'
import { RemoteApprovalsToggle } from './RemoteApprovalsToggle'

const ok = (data: unknown) => ({ data, response: new Response(null, { status: 200 }) })

const future = () => new Date(Date.now() + 200_000).toISOString()
const mk = (over: Partial<Approval>): Approval => ({
  id: 'appr-1',
  machine: 'devbox',
  tool: 'Bash',
  created_at: new Date(Date.now() - 120_000).toISOString(),
  expires_at: future(),
  ...over,
})

let items: Approval[]
let settings: { enabled: boolean; timeout_seconds: number }

const qc = () => new QueryClient({ defaultOptions: { queries: { retry: false } } })
const wrap = (ui: React.ReactNode, client = qc()) => <QueryClientProvider client={client}>{ui}</QueryClientProvider>

beforeEach(() => {
  vi.clearAllMocks()
  items = []
  settings = { enabled: true, timeout_seconds: 300 }
  mocks.GET.mockImplementation(async (path: string) => {
    if (path === '/api/approvals') return ok({ items })
    if (path === '/api/settings/approvals') return ok(settings)
    return ok({})
  })
  mocks.POST.mockImplementation(async (_p: string, o: { body: { decision: string }; params: { path: { id: string } } }) =>
    ok({ id: o.params.path.id, decision: o.body.decision }),
  )
  mocks.PUT.mockImplementation(async (_p: string, o: { body: { enabled: boolean } }) => ok({ ...settings, ...o.body }))
})

describe('ApprovalsPage', () => {
  it('renders Bash, Edit, truncated and generic inputs as text', async () => {
    items = [
      mk({ id: 'a1', identity: 'builder@github.com/acme/api', client: 'claude-code', tool: 'Bash', input: { command: 'rm -rf <b>x</b>' }, project_dir: '/src/api' }),
      mk({ id: 'a2', tool: 'Edit', session_id: 'sess-9', input: { file_path: '/src/a.go', old_string: 'abc', new_string: 'de' } }),
      mk({ id: 'a3', tool: 'Write', input: { file_path: '/src/b.go', content: 'x\ny\nz' } }),
      mk({ id: 'a4', tool: 'Big', input: { truncated: true, bytes: 40000, preview: '{"a":' } }),
      mk({ id: 'a5', tool: 'WebFetch', input: { url: 'https://example.com' } }),
    ]
    const { container } = render(wrap(<ApprovalsPage />))
    const cards = await screen.findAllByTestId('approval')
    expect(cards).toHaveLength(5)
    expect(cards[0]!.querySelector('pre code')).toHaveTextContent('rm -rf <b>x</b>')
    expect(container.querySelector('b b')).toBeNull()
    expect(cards[0]).toHaveTextContent('builder@github.com/acme/api')
    expect(cards[0]).toHaveTextContent('devbox:/src/api')
    expect(cards[0]).toHaveTextContent('2m ago')
    expect(cards[0]).toHaveTextContent(/3m \d\ds left|3m 2\ds left|3m/)
    expect(cards[1]).toHaveTextContent('unsigned session')
    expect(cards[1]).toHaveTextContent('sess-9')
    expect(cards[1]).toHaveTextContent('/src/a.go')
    expect(cards[1]).toHaveTextContent('replace 3 chars with 2 chars')
    expect(cards[2]).toHaveTextContent('3 lines')
    expect(cards[3]).toHaveTextContent('Input truncated (40000 bytes)')
    expect(cards[4]).toHaveTextContent('"url": "https://example.com"')
  })

  it('shows falling back to terminal once expired', async () => {
    items = [mk({ expires_at: new Date(Date.now() - 1000).toISOString() })]
    render(wrap(<ApprovalsPage />))
    expect(await screen.findByText('falling back to terminal')).toBeInTheDocument()
  })

  it('posts allow and deny decisions with the reason', async () => {
    const user = userEvent.setup()
    items = [mk({ id: 'a1', input: { command: 'ls' } }), mk({ id: 'a2', input: { command: 'pwd' } })]
    render(wrap(<ApprovalsPage />))
    const cards = await screen.findAllByTestId('approval')
    await user.click(within(cards[0]!).getByRole('button', { name: 'Allow' }))
    await waitFor(() => expect(mocks.POST).toHaveBeenCalledWith('/api/approvals/{id}', { params: { path: { id: 'a1' } }, body: { decision: 'allow' } }))
    await user.type(within(cards[1]!).getByLabelText('Reason (optional)'), 'too risky')
    await user.click(within(cards[1]!).getByRole('button', { name: 'Deny' }))
    await waitFor(() =>
      expect(mocks.POST).toHaveBeenCalledWith('/api/approvals/{id}', { params: { path: { id: 'a2' } }, body: { decision: 'deny', reason: 'too risky' } }),
    )
  })

  it('treats a 404 as no longer pending and refetches', async () => {
    const user = userEvent.setup()
    items = [mk({ id: 'a1', input: { command: 'ls' } })]
    mocks.POST.mockImplementation(async () => ({ error: { detail: 'gone' }, response: new Response(null, { status: 404 }) }))
    render(wrap(<ApprovalsPage />))
    const allow = await screen.findByRole('button', { name: 'Allow' })
    items = [] // the agent already stopped waiting; the refetch after the 404 sees it gone
    await user.click(allow)
    expect(await screen.findByText(/Nothing pending/)).toBeInTheDocument()
  })

  it('explains the empty state and can turn approvals on', async () => {
    const user = userEvent.setup()
    settings = { enabled: false, timeout_seconds: 120 }
    render(wrap(<ApprovalsPage />))
    expect(await screen.findByText(/Remote approvals are off/)).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Turn on remote approvals' }))
    await waitFor(() => expect(mocks.PUT).toHaveBeenCalledWith('/api/settings/approvals', { body: { enabled: true, timeout_seconds: 120 } }))
  })

  it('shows nothing pending when enabled', async () => {
    render(wrap(<ApprovalsPage />))
    expect(await screen.findByText(/Nothing pending/)).toBeInTheDocument()
  })
})

describe('RemoteApprovalsToggle', () => {
  it('toggles enabled via PUT and shows the pending badge', async () => {
    const user = userEvent.setup()
    settings = { enabled: false, timeout_seconds: 300 }
    items = [mk({}), mk({ id: 'appr-2' })]
    await renderWithRouter(wrap(<RemoteApprovalsToggle />))
    const sw = await screen.findByRole('switch', { name: 'Remote approvals' })
    await waitFor(() => expect(sw).toBeEnabled())
    expect(sw).toHaveAttribute('aria-checked', 'false')
    expect(await screen.findByLabelText('2 pending approvals')).toBeInTheDocument()
    await user.click(sw)
    await waitFor(() => expect(mocks.PUT).toHaveBeenCalledWith('/api/settings/approvals', { body: { enabled: true, timeout_seconds: 300 } }))
  })
})

describe('OperatorAlerts approvals', () => {
  const notes: Array<{ title: string; opts?: NotificationOptions }> = []
  beforeEach(() => {
    notes.length = 0
    localStorage.clear()
    class FakeNotification {
      static permission: NotificationPermission = 'granted'
      static requestPermission = vi.fn(async () => 'granted' as NotificationPermission)
      constructor(title: string, opts?: NotificationOptions) {
        notes.push({ title, opts })
      }
    }
    vi.stubGlobal('Notification', FakeNotification)
    reloadNotifyPref()
  })
  afterEach(() => {
    vi.unstubAllGlobals()
    document.title = ''
  })

  it('notifies when a new approval appears and puts the count in the title', async () => {
    items = [mk({ id: 'old' })]
    const client = qc()
    render(wrap(<OperatorAlerts />, client))
    await waitFor(() => expect(document.title).toBe('(1!) AIM'))
    await act(async () => void (await setNotificationsEnabled(true)))
    items = [mk({ id: 'old' }), mk({ id: 'new', identity: 'builder@x', summary: 'Run tests', tool: 'Bash' })]
    await act(async () => {
      await client.invalidateQueries({ queryKey: ['approvals'] })
    })
    await waitFor(() => expect(notes).toHaveLength(1))
    expect(notes[0]).toMatchObject({ title: 'Approval needed', opts: { body: 'builder@x: Run tests', tag: 'new' } })
    await waitFor(() => expect(document.title).toBe('(2!) AIM'))
  })
})
