import { type ReactNode } from 'react'
import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { useRouterState } from '@tanstack/react-router'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { runtimeFor } from '@/lib/identity'
import { helpFlag, paletteFlag } from '@/lib/flag'
import { reloadNotifyPref, setNotificationsEnabled } from '@/lib/notify'
import { ThemeProvider } from '@/lib/theme'
import { composeStore } from '@/lib/compose'
import { renderWithRouter } from '@/test/utils'
import { treeFixture } from '@/test/fixtures'

const mocks = vi.hoisted(() => ({ GET: vi.fn(), POST: vi.fn() }))
vi.mock('@/api/client', async (orig) => ({ ...(await orig<typeof import('@/api/client')>()), api: mocks }))

import { CommandPalette } from './CommandPalette'
import { OperatorAlerts } from './OperatorAlerts'
import { ShortcutsDialog, useGlobalShortcuts } from './Shortcuts'
import { TopBar } from './TopBar'

const ok = (data: unknown) => ({ data, response: new Response(null, { status: 200 }) })

let unread = 0
const newest = { id: 'm1', from: 'reviewer@github.com/acme/api', subject: 'Need approval', thread_id: 't1', mailbox: 'operator' }

function Probe() {
  const loc = useRouterState({ select: (s) => s.location })
  return <output data-testid="loc">{loc.pathname + JSON.stringify(loc.search)}</output>
}

function Keys() {
  useGlobalShortcuts()
  return (
    <>
      <input aria-label="field" />
      <ShortcutsDialog />
    </>
  )
}

const qc = () => new QueryClient({ defaultOptions: { queries: { retry: false } } })
function Providers({ client, children }: { client: QueryClient; children: ReactNode }) {
  return (
    <ThemeProvider>
      <QueryClientProvider client={client}>{children}</QueryClientProvider>
    </ThemeProvider>
  )
}

beforeEach(() => {
  vi.clearAllMocks()
  unread = 0
  paletteFlag.set(false)
  helpFlag.set(false)
  composeStore.close()
  mocks.GET.mockImplementation(async (path: string) => {
    if (path === '/api/views') return ok({ operator_unread: unread, unread_everywhere: 0, today: 0 })
    if (path === '/api/tree') return ok({ orgs: treeFixture })
    if (path === '/api/roster')
      return ok({ items: [{ address: 'reviewer@github.com/acme/api', role: 'review', description: '', online: [], subagents: [] }] })
    if (path === '/api/search') return ok({ hits: [{ message_id: 'm9', thread_id: 't9', subject: 'Deploy plan', snippet: '', from: 'builder@github.com/acme/api', mailbox: 'operator', created_at: '', rank: 1 }] })
    if (path === '/api/messages') return ok({ items: [newest] })
    return { error: { detail: 'nope' }, response: new Response(null, { status: 404 }) }
  })
})

describe('CommandPalette', () => {
  const setup = () =>
    renderWithRouter(
      <Providers client={qc()}>
        <Keys />
        <CommandPalette />
        <Probe />
      </Providers>,
    )

  it('opens on Ctrl+K and Meta+K, even while typing', async () => {
    const user = userEvent.setup()
    await setup()
    expect(screen.queryByRole('dialog')).toBeNull()
    await user.keyboard('{Control>}k{/Control}')
    expect(await screen.findByRole('dialog')).toBeInTheDocument()
    await user.keyboard('{Control>}k{/Control}')
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    await user.click(screen.getByLabelText('field'))
    await user.keyboard('{Meta>}k{/Meta}')
    expect(await screen.findByRole('dialog')).toBeInTheDocument()
  })

  it('navigates to a mailbox, an identity and a view', async () => {
    const user = userEvent.setup()
    await setup()
    act(() => paletteFlag.set(true))
    await user.click(await screen.findByText('github.com/acme/api'))
    await waitFor(() => expect(screen.getByTestId('loc').textContent).toContain('"mailbox":"github.com/acme/api"'))
    act(() => paletteFlag.set(true))
    await user.click(await screen.findByText('Sent'))
    await waitFor(() => expect(screen.getByTestId('loc').textContent).toContain('"view":"sent"'))
  })

  it('searches messages (debounced, global) and opens the thread', async () => {
    const user = userEvent.setup()
    await setup()
    act(() => paletteFlag.set(true))
    await user.type(await screen.findByPlaceholderText(/Search mail/), 'deploy')
    await user.click(await screen.findByText('Deploy plan'))
    const calls = mocks.GET.mock.calls.filter((c) => c[0] === '/api/search')
    expect(calls).toHaveLength(1) // debounced: only the settled term
    expect(calls[0]![1].params.query).toEqual({ q: 'deploy', limit: 8 }) // no address scope
    await waitFor(() => expect(screen.getByTestId('loc').textContent).toContain('"thread":"t9"'))
    expect(screen.getByTestId('loc').textContent).toContain('"mailbox":"operator"')
  })

  it('runs the Compose action', async () => {
    const user = userEvent.setup()
    await setup()
    act(() => paletteFlag.set(true))
    await user.click(await screen.findByText('Compose'))
    expect(composeStore.isOpen()).toBe(true)
  })

  it('opens from the top bar search button', async () => {
    const user = userEvent.setup()
    await renderWithRouter(
      <Providers client={qc()}>
        <TopBar />
        <CommandPalette />
      </Providers>,
    )
    await user.click(screen.getByRole('button', { name: 'Search mail, mailboxes and agents' }))
    expect(await screen.findByRole('dialog')).toBeInTheDocument()
  })
})

describe('shortcuts help', () => {
  it('opens on ? but ignores keys typed into inputs', async () => {
    const user = userEvent.setup()
    render(<Keys />)
    await user.click(screen.getByLabelText('field'))
    await user.keyboard('?')
    expect(screen.queryByRole('dialog')).toBeNull()
    expect(screen.getByLabelText('field')).toHaveValue('?')
    await user.click(document.body)
    await user.keyboard('?')
    expect(await screen.findByRole('dialog', { name: 'Keyboard shortcuts' })).toBeInTheDocument()
  })
})

describe('OperatorAlerts', () => {
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

  const mount = () => {
    const client = qc()
    render(
      <QueryClientProvider client={client}>
        <OperatorAlerts />
      </QueryClientProvider>,
    )
    return client
  }
  const bump = async (client: QueryClient, n: number) => {
    unread = n
    await act(async () => {
      await client.invalidateQueries({ queryKey: ['views'] })
    })
  }

  it('shows the unread count in the tab title', async () => {
    const client = mount()
    await waitFor(() => expect(document.title).toBe('AIM'))
    await bump(client, 3)
    await waitFor(() => expect(document.title).toBe('(3) AIM'))
    await bump(client, 0)
    await waitFor(() => expect(document.title).toBe('AIM'))
  })

  it('notifies only on an increase and only when enabled', async () => {
    unread = 1
    const client = mount()
    await waitFor(() => expect(document.title).toBe('(1) AIM'))
    await bump(client, 2) // increase but not opted in
    await waitFor(() => expect(document.title).toBe('(2) AIM'))
    expect(notes).toHaveLength(0)

    await act(async () => void (await setNotificationsEnabled(true)))
    expect(localStorage.getItem('aim-notify')).toBe('1')
    await bump(client, 1) // decrease
    await waitFor(() => expect(document.title).toBe('(1) AIM'))
    expect(notes).toHaveLength(0)
    await bump(client, 4) // increase
    await waitFor(() => expect(notes).toHaveLength(1))
    expect(notes[0]).toMatchObject({ title: newest.from, opts: { body: 'Need approval' } })
    await bump(client, 4) // unchanged
    expect(notes).toHaveLength(1)
  })

  it('polls views in a hidden tab only when notifications are enabled', async () => {
    const client = mount()
    const opts = () => client.getQueryCache().find({ queryKey: ['views'] })!.observers[0]!.options.refetchIntervalInBackground
    await waitFor(() => expect(document.title).toBe('AIM'))
    expect(opts()).toBe(false)
    await act(async () => void (await setNotificationsEnabled(true)))
    await waitFor(() => expect(opts()).toBe(true))
    await act(async () => void (await setNotificationsEnabled(false)))
    await waitFor(() => expect(opts()).toBe(false))
  })

  it('does not enable when permission is denied', async () => {
    ;(Notification as unknown as { permission: string }).permission = 'denied'
    await expect(setNotificationsEnabled(true)).resolves.toBe(false)
    expect(localStorage.getItem('aim-notify')).toBe('0')
  })
})

describe('runtime icons', () => {
  it('maps every supported client and falls back to the raw name', () => {
    const names = ['claude-code', 'codex-cli', 'gemini-cli', 'opencode', 'grok', 'cline', 'kilo-code', 'deepseek-tui']
    for (const n of names) expect(runtimeFor(n).icon, n).toBeTruthy()
    expect(runtimeFor('mystery-agent')).toEqual({ label: 'mystery-agent' })
  })
})
