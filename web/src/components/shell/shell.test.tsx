import { type ReactNode } from 'react'
import { act, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { useRouterState } from '@tanstack/react-router'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { runtimeFor } from '@/lib/identity'
import { helpFlag, paletteFlag } from '@/lib/flag'
import { reloadNotifyPref, setNotificationsEnabled } from '@/lib/notify'
import { ThemeProvider } from '@/lib/theme'
import { composeStore } from '@/lib/compose'
import { mockViewport, renderWithRouter } from '@/test/utils'
import { treeFixture } from '@/test/fixtures'

const mocks = vi.hoisted(() => ({ GET: vi.fn(), POST: vi.fn(), PUT: vi.fn() }))
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

  it('offers Search all mail and opens the search page with the query', async () => {
    const user = userEvent.setup()
    await setup()
    act(() => paletteFlag.set(true))
    await user.type(await screen.findByPlaceholderText(/Search mail/), 'zebra')
    await user.click(await screen.findByText(/Search all mail for/))
    await waitFor(() => expect(screen.getByTestId('loc').textContent).toBe('/search{"q":"zebra"}'))
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

describe('mobile layout', () => {
  afterEach(() => {
    delete (window as { matchMedia?: unknown }).matchMedia
  })
  const phone = (matches: boolean) => {
    window.matchMedia = ((q: string) => ({
      matches,
      media: q,
      addEventListener: () => {},
      removeEventListener: () => {},
    })) as unknown as typeof window.matchMedia
  }
  const shell = async () => {
    const { AppShell } = await import('./AppShell')
    return renderWithRouter(
      <Providers client={qc()}>
        <AppShell>
          <p>page</p>
        </AppShell>
      </Providers>,
    )
  }

  it('desktop renders the nav and buddy rail inline', async () => {
    phone(false)
    await shell()
    expect(await screen.findByRole('navigation', { name: 'Mailboxes' })).toBeInTheDocument()
    expect(screen.getByRole('complementary', { name: 'Buddy list' })).toBeInTheDocument()
  })

  it('phone moves the nav and buddy list into sheets opened from the top bar', async () => {
    const user = userEvent.setup()
    phone(true)
    mocks.GET.mockImplementation(async (path: string) =>
      path === '/api/presence' ? ok({ groups: [], events: [] }) : path === '/api/tree' ? ok({ orgs: treeFixture }) : path === '/api/views' ? ok({ operator_unread: 0, unread_everywhere: 0, today: 0 }) : { error: { detail: 'nope' }, response: new Response(null, { status: 404 }) },
    )
    await shell()
    expect(screen.queryByRole('navigation', { name: 'Mailboxes' })).toBeNull()
    expect(screen.queryByRole('complementary', { name: 'Buddy list' })).toBeNull()
    await user.click(screen.getByRole('button', { name: 'Open navigation' }))
    const drawer = await screen.findByRole('dialog', { name: 'Navigation' })
    expect(within(drawer).getByRole('navigation', { name: 'Mailboxes' })).toBeInTheDocument()
    await user.keyboard('{Escape}')
    await waitFor(() => expect(screen.queryByRole('dialog', { name: 'Navigation' })).toBeNull())
    await user.click(screen.getByRole('button', { name: 'Open buddy list' }))
    expect(await screen.findByRole('dialog', { name: 'Buddy list' })).toBeInTheDocument()
  })
})

describe('TopBar', () => {
  let remote = false
  beforeEach(() => {
    remote = true
    const base = mocks.GET.getMockImplementation()!
    mocks.GET.mockImplementation(async (path: string, ...rest: unknown[]) =>
      path === '/api/settings/approvals'
        ? ok({ enabled: remote, timeout_seconds: 300 })
        : path === '/api/approvals'
          ? ok({ items: [{ id: 'a1' }] })
          : base(path, ...rest),
    )
  })
  afterEach(() => {
    delete (window as { matchMedia?: unknown }).matchMedia
  })
  const bar = () =>
    renderWithRouter(
      <Providers client={qc()}>
        <TopBar />
      </Providers>,
    )

  it('keeps the approvals switch, refresh age and operator in the bar at xl, without an approvals count', async () => {
    mockViewport(1440)
    await bar()
    expect(await screen.findByRole('switch', { name: 'Remote approvals' })).toBeInTheDocument()
    expect(screen.getByTestId('updated-ago')).toHaveClass('tabular-nums', 'w-[8.5rem]')
    expect(screen.getByText('operator')).toBeInTheDocument()
    expect(screen.queryByLabelText(/pending approval/)).toBeNull()
    expect(screen.queryByTestId('remote-on-dot')).toBeNull()
  })

  it('moves them into the More menu below xl and flags remote approvals on the More button', async () => {
    const user = userEvent.setup()
    mockViewport(1024)
    await bar()
    expect(screen.queryByRole('switch', { name: 'Remote approvals' })).toBeNull()
    expect(screen.queryByTestId('updated-ago')).toBeNull()
    expect(await screen.findByTestId('remote-on-dot')).toBeInTheDocument()
    const more = screen.getByRole('button', { name: 'More options' })
    expect(more).toHaveAccessibleDescription('Remote approvals on')
    await user.click(more)
    const menu = await screen.findByRole('menu')
    expect(within(menu).getByText('operator')).toBeInTheDocument()
    expect(within(menu).getByTestId('updated-ago')).toBeInTheDocument()
    const item = within(menu).getByRole('menuitemcheckbox', { name: 'Remote approvals' })
    expect(item).toHaveAttribute('aria-checked', 'true')
    mocks.PUT.mockResolvedValue(ok({ enabled: false, timeout_seconds: 300 }))
    await user.click(item)
    await waitFor(() => expect(mocks.PUT).toHaveBeenCalledWith('/api/settings/approvals', { body: { enabled: false, timeout_seconds: 300 } }))
  })
})

describe('LeftNav', () => {
  it('puts work views first, mailboxes in their own region and pages in the footer', async () => {
    localStorage.clear()
    unread = 4
    const base = mocks.GET.getMockImplementation()!
    mocks.GET.mockImplementation(async (path: string, ...rest: unknown[]) =>
      path === '/api/approvals' ? ok({ items: [{ id: 'a1' }, { id: 'a2' }] }) : base(path, ...rest),
    )
    const { LeftNav } = await import('./LeftNav')
    await renderWithRouter(
      <Providers client={qc()}>
        <LeftNav />
      </Providers>,
      '/approvals',
    )
    const nav = await screen.findByRole('navigation', { name: 'Mailboxes' })
    const work = within(nav).getByRole('group', { name: 'Work' })
    expect(within(work).getAllByRole('link').map((a) => a.textContent?.replace(/\d+$/, ''))).toEqual([
      'Operator inbox',
      'Unread everywhere',
      'Today',
      'Tasks',
      'Approvals',
    ])
    expect(await within(work).findByLabelText('4 unread')).toBeInTheDocument()
    expect(await within(work).findByLabelText('2 pending')).toBeInTheDocument()
    const folders = within(nav).getByRole('group', { name: 'Folders' })
    expect(within(folders).getByRole('link', { name: 'Sent' })).toBeInTheDocument()
    expect(within(folders).getByRole('link', { name: 'Archived' })).toBeInTheDocument()
    const manage = within(nav).getByRole('group', { name: 'Manage' })
    expect(within(manage).getAllByRole('link').map((a) => a.textContent)).toEqual(['Roster', 'Org chart', 'Settings'])
    // The active route is highlighted wherever its link lives.
    expect(within(work).getByRole('link', { name: /^Approvals/ })).toHaveAttribute('data-status', 'active')
    // Mailboxes scroll on their own, below the work views.
    const region = await within(nav).findByRole('region', { name: 'Mailbox list' })
    expect(within(region).getByTestId('mailbox-scroll')).toHaveClass('overflow-y-auto', 'min-h-0')
    expect(work.compareDocumentPosition(region) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    expect(region.compareDocumentPosition(manage) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  })
})
