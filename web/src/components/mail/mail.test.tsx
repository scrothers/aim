import { useState, type ReactNode } from 'react'
import { act, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { Header, MailSearch, MessageView } from '@/api/mail'
import { composeStore } from '@/lib/compose'

const mocks = vi.hoisted(() => ({ GET: vi.fn(), POST: vi.fn() }))
vi.mock('@/api/client', async (orig) => ({ ...(await orig<typeof import('@/api/client')>()), api: mocks }))

import { Composer, replyDefaults } from './Composer'
import { MailView } from './MailView'
import { ThreadView } from './ThreadView'

const ok = (data: unknown) => ({ data, response: new Response(null, { status: 200 }) })
const fail = (status: number, detail: string, suggestions?: string[]) => ({
  error: { detail, errors: suggestions ? [{ location: 'suggestions', value: suggestions }] : [] },
  response: new Response(null, { status }),
})

const hdr = (id: string, extra: Partial<Header> = {}): Header => ({
  id,
  thread_id: `t-${id}`,
  from: 'reviewer@github.com/acme/api',
  subject: `Subject ${id}`,
  created_at: new Date().toISOString(),
  to: ['operator'],
  cc: [],
  mailbox: 'operator',
  kind: 'message',
  thread_count: 1,
  ...extra,
})

const msg = (id: string, extra: Partial<MessageView> = {}): MessageView => ({
  id,
  thread_id: 't-1',
  from: 'reviewer@github.com/acme/api',
  subject: 'Deploy plan',
  body: `body ${id}`,
  body_html: `<p>html ${id}</p>`,
  created_at: `2026-11-17T10:0${id}:00Z`,
  to: ['operator'],
  cc: ['builder@github.com/acme/api'],
  from_client: 'claude-code',
  from_machine: 'devbox',
  ...extra,
})

function wrap(ui: ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(<QueryClientProvider client={qc}>{ui}</QueryClientProvider>)
}

function routes(opts: { list?: (q: Record<string, string>) => unknown; thread?: MessageView[]; roster?: string[] } = {}) {
  mocks.GET.mockImplementation(async (path: string, o?: { params?: { query?: Record<string, string> } }) => {
    if (path === '/api/messages') return ok(opts.list?.(o?.params?.query ?? {}) ?? { items: [] })
    if (path === '/api/threads/{id}') return ok({ messages: opts.thread ?? [] })
    if (path === '/api/roster') return ok({ items: (opts.roster ?? []).map((address) => ({ address, role: '', description: '', online: [], subagents: [] })) })
    return fail(404, 'nope')
  })
  mocks.POST.mockImplementation(async (path: string) => {
    if (path === '/api/render') return ok({ html: '<p><strong>rendered</strong></p>' })
    if (path === '/api/messages') return ok(msg('9'))
    return ok({})
  })
}

const calls = (fn: typeof mocks.GET, path: string) => fn.mock.calls.filter((c) => c[0] === path)

beforeEach(() => {
  vi.clearAllMocks()
  composeStore.close()
})

function Harness({ initial = {} }: { initial?: MailSearch }) {
  const [s, setS] = useState<MailSearch>(initial)
  return <MailView search={s} onSearch={(p) => setS((prev) => ({ ...prev, ...p }))} />
}

describe('MessageList', () => {
  it('renders unread vs read rows and thread count', async () => {
    routes({
      list: () => ({
        items: [
          hdr('1', { thread_id: 't-a', thread_count: 2, from_client: 'codex-cli' }),
          hdr('2', { thread_id: 't-a', thread_count: 2, read_at: '2026-11-17T09:00:00Z' }),
          hdr('3', { read_at: '2026-11-17T09:00:00Z' }),
        ],
      }),
    })
    wrap(<Harness />)
    const rows = await screen.findAllByRole('button', { name: /Subject/ })
    expect(rows).toHaveLength(3)
    expect(rows[0]).toHaveAttribute('data-unread', 'true')
    expect(rows[1]).toHaveAttribute('data-unread', 'false')
    expect(screen.getAllByRole('img', { name: 'Unread' })).toHaveLength(1)
    expect(screen.getAllByLabelText('2 messages in thread')).toHaveLength(2)
    expect(within(rows[0]!).getByAltText('Codex')).toBeInTheDocument()
    expect(calls(mocks.GET, '/api/messages')[0]![1].params.query).toMatchObject({ address: 'operator', state: 'all' })
  })

  it('loads more pages with the cursor', async () => {
    const user = userEvent.setup()
    routes({ list: (q) => (q.cursor ? { items: [hdr('2')] } : { items: [hdr('1')], next_cursor: 'c1' }) })
    wrap(<Harness />)
    await screen.findByText('Subject 1')
    await user.click(screen.getByRole('button', { name: 'Load more' }))
    await screen.findByText('Subject 2')
    expect(calls(mocks.GET, '/api/messages')[1]![1].params.query.cursor).toBe('c1')
    expect(screen.queryByRole('button', { name: 'Load more' })).toBeNull()
  })

  it('selecting a row sets the thread param', async () => {
    const user = userEvent.setup()
    routes({ list: () => ({ items: [hdr('1')] }), thread: [msg('1', { thread_id: 't-1' })] })
    const onSearch = vi.fn()
    wrap(<MailView search={{}} onSearch={onSearch} />)
    await user.click(await screen.findByRole('button', { name: /Subject 1/ }))
    expect(onSearch).toHaveBeenCalledWith({ thread: 't-1' })
  })

  it('archived tab sends a default 30 day range and shows the note', async () => {
    routes({ list: () => ({ items: [] }) })
    wrap(<Harness initial={{ state: 'archived' }} />)
    await screen.findByText('No messages.')
    const q = calls(mocks.GET, '/api/messages')[0]![1].params.query
    expect(q.state).toBe('archived')
    expect(Date.parse(q.until) - Date.parse(q.since)).toBe(30 * 86_400_000)
    expect(screen.getByText(/Last 30 days/)).toBeInTheDocument()
  })

  it('unread view queries all mailboxes; mailbox selects an address', async () => {
    routes({ list: () => ({ items: [] }) })
    wrap(<Harness initial={{ view: 'unread' }} />)
    await screen.findByText('No messages.')
    const q = calls(mocks.GET, '/api/messages')[0]![1].params.query
    expect(q.address).toBeUndefined()
    expect(q.state).toBe('unread')
  })
})

describe('ThreadView', () => {
  const thread = [msg('1', { from: 'builder@github.com/acme/api' }), msg('2'), msg('3')]
  const view = (extra: Partial<React.ComponentProps<typeof ThreadView>> = {}) => (
    <ThreadView threadId="t-1" replying={false} onReplying={() => {}} onArchived={() => {}} {...extra} />
  )

  it('collapses earlier messages and expands on click', async () => {
    const user = userEvent.setup()
    routes({ thread })
    wrap(view())
    await screen.findByText('html 3')
    expect(screen.queryByText('html 1')).toBeNull()
    expect(screen.queryByText('html 2')).toBeNull()
    expect(screen.getByText('body 1')).toBeInTheDocument() // collapsed snippet
    expect(screen.getByText('Claude Code on devbox')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Expand message from builder@github.com/acme/api' }))
    expect(screen.getByText('html 1')).toBeInTheDocument()
    await user.click(screen.getAllByRole('button', { name: /^Collapse/ })[0]!)
    expect(screen.queryByText('html 1')).toBeNull()
  })

  it('marks operator messages read once per message', async () => {
    const user = userEvent.setup()
    routes({ thread })
    wrap(view())
    await screen.findByText('html 3')
    await waitFor(() => expect(calls(mocks.POST, '/api/messages/{id}/read')).toHaveLength(1))
    // Toggle the latest closed and open again: no second call.
    await user.click(screen.getByRole('button', { name: /^Collapse message from reviewer/ }))
    await user.click(screen.getAllByRole('button', { name: /^Expand message from reviewer/ }).at(-1)!)
    expect(calls(mocks.POST, '/api/messages/{id}/read')).toHaveLength(1)
    expect(calls(mocks.POST, '/api/messages/{id}/read')[0]![1].params.path.id).toBe('3')
    // Expanding an earlier operator-addressed message reads it once too.
    await user.click(screen.getByRole('button', { name: 'Expand message from builder@github.com/acme/api' }))
    await waitFor(() => expect(calls(mocks.POST, '/api/messages/{id}/read')).toHaveLength(2))
  })

  it('archives and marks unread via markMessages', async () => {
    const user = userEvent.setup()
    routes({ thread })
    const onArchived = vi.fn()
    wrap(view({ onArchived }))
    await screen.findByText('html 3')
    await user.click(screen.getByRole('button', { name: 'Mark unread' }))
    await waitFor(() => expect(calls(mocks.POST, '/api/marks')).toHaveLength(1))
    expect(calls(mocks.POST, '/api/marks')[0]![1].body).toEqual({ message_ids: ['3'], state: 'unread' })
    await user.click(screen.getByRole('button', { name: 'Archive' }))
    await waitFor(() => expect(onArchived).toHaveBeenCalled())
    expect(calls(mocks.POST, '/api/marks')[1]![1].body).toEqual({ message_ids: ['1', '2', '3'], state: 'archived' })
  })

  it('offers Unarchive when viewing archived mail', async () => {
    const user = userEvent.setup()
    routes({ thread })
    const onArchived = vi.fn()
    wrap(view({ onArchived, archived: true }))
    await screen.findByText('html 3')
    expect(screen.queryByRole('button', { name: 'Archive' })).toBeNull()
    await user.click(screen.getByRole('button', { name: 'Unarchive' }))
    await waitFor(() => expect(onArchived).toHaveBeenCalled())
    expect(calls(mocks.POST, '/api/marks')[0]![1].body).toEqual({ message_ids: ['1', '2', '3'], state: 'unarchived' })
  })

  it('renders sanitized HTML in the prose container', async () => {
    routes({ thread: [msg('1', { body_html: '<h2>Head</h2><pre><code>x</code></pre>' })] })
    wrap(view())
    const h = await screen.findByRole('heading', { name: 'Head' })
    expect(h.closest('.prose-aim')).not.toBeNull()
  })
})

describe('Composer', () => {
  it('prefills a reply-all without the operator', () => {
    const m = msg('1', { to: ['operator', 'a@github.com/acme/x'], cc: ['b@github.com/acme/x', 'operator'] })
    expect(replyDefaults(m)).toEqual({
      to: ['reviewer@github.com/acme/api'],
      cc: ['a@github.com/acme/x', 'b@github.com/acme/x'],
      subject: 'Re: Deploy plan',
    })
    routes()
    wrap(<Composer mode={{ kind: 'reply', original: m }} onClose={() => {}} />)
    const form = screen.getByRole('form', { name: 'Reply' })
    expect(within(form).queryByRole('button', { name: 'Remove operator' })).toBeNull()
    expect(within(form).getByRole('button', { name: 'Remove reviewer@github.com/acme/api' })).toBeInTheDocument()
    expect(within(form).getByRole('button', { name: 'Remove b@github.com/acme/x' })).toBeInTheDocument()
  })

  it('adds, validates and removes chips', async () => {
    const user = userEvent.setup()
    routes({ roster: ['coordinator@github.com/acme', 'reviewer@github.com/acme/api'] })
    wrap(<Composer mode={{ kind: 'new' }} onClose={() => {}} />)
    const to = screen.getByLabelText('To recipients')
    await user.type(to, 'not an address{Enter}')
    expect(await screen.findByRole('alert')).toHaveTextContent('Invalid address')
    await user.clear(to)
    await user.type(to, 'ops@github.com/acme/api{Enter}')
    expect(screen.getByRole('button', { name: 'Remove ops@github.com/acme/api' })).toBeInTheDocument()
    await user.type(to, 'coord')
    await user.click(await screen.findByRole('button', { name: /coordinator/ }))
    expect(screen.getByRole('button', { name: 'Remove coordinator@github.com/acme' })).toBeInTheDocument()
    await user.type(to, 'operator,')
    expect(screen.getByRole('button', { name: 'Remove operator' })).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Remove operator' }))
    expect(screen.queryByRole('button', { name: 'Remove operator' })).toBeNull()
  })

  it('previews through renderMarkdown', async () => {
    const user = userEvent.setup()
    routes()
    wrap(<Composer mode={{ kind: 'new' }} onClose={() => {}} />)
    await user.type(screen.getByLabelText('Message body'), '**rendered**')
    await user.click(screen.getByRole('button', { name: /Preview/ }))
    await screen.findByText('rendered')
    expect(calls(mocks.POST, '/api/render')[0]![1].body).toEqual({ markdown: '**rendered**' })
  })

  it('sends with Ctrl+Enter and the right payload', async () => {
    const user = userEvent.setup()
    routes()
    const onClose = vi.fn()
    wrap(<Composer mode={{ kind: 'new' }} onClose={onClose} />)
    await user.type(screen.getByLabelText('To recipients'), 'ops@github.com/acme/api{Enter}')
    await user.type(screen.getByLabelText('Subject'), 'Hello')
    await user.type(screen.getByLabelText('Message body'), 'Hi there')
    await user.keyboard('{Control>}{Enter}{/Control}')
    await waitFor(() => expect(calls(mocks.POST, '/api/messages')).toHaveLength(1))
    expect(calls(mocks.POST, '/api/messages')[0]![1].body).toEqual({
      to: ['ops@github.com/acme/api'],
      cc: [],
      subject: 'Hello',
      body: 'Hi there',
    })
    await waitFor(() => expect(onClose).toHaveBeenCalled())
  })

  it('sends a reply with in_reply_to and keeps the thread subject', async () => {
    const user = userEvent.setup()
    routes()
    wrap(<Composer mode={{ kind: 'reply', original: msg('4') }} onClose={() => {}} />)
    await user.type(screen.getByLabelText('Message body'), 'Thanks')
    await user.keyboard('{Control>}{Enter}{/Control}')
    await waitFor(() => expect(calls(mocks.POST, '/api/messages')).toHaveLength(1))
    expect(calls(mocks.POST, '/api/messages')[0]![1].body).toMatchObject({
      to: ['reviewer@github.com/acme/api'],
      cc: ['builder@github.com/acme/api'],
      subject: 'Re: Deploy plan',
      in_reply_to: '4',
    })
  })

  it('renders 404 suggestions and applies one', async () => {
    const user = userEvent.setup()
    routes()
    mocks.POST.mockImplementation(async () => fail(404, 'unknown recipient ops@github.com/acme/ap', ['ops@github.com/acme/api']))
    wrap(<Composer mode={{ kind: 'new' }} onClose={() => {}} />)
    await user.type(screen.getByLabelText('To recipients'), 'ops@github.com/acme/ap{Enter}')
    await user.type(screen.getByLabelText('Subject'), 'S')
    await user.type(screen.getByLabelText('Message body'), 'B')
    await user.click(screen.getByRole('button', { name: /^Send/ }))
    const fix = await screen.findByRole('button', { name: 'ops@github.com/acme/api' })
    await user.click(fix)
    expect(screen.getByRole('button', { name: 'Remove ops@github.com/acme/api' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Remove ops@github.com/acme/ap' })).toBeNull()
  })
})

describe('keyboard shortcuts', () => {
  const press = (key: string) => act(() => void window.dispatchEvent(new KeyboardEvent('keydown', { key, bubbles: true })))
  const items = () => ({ items: [hdr('1'), hdr('2'), hdr('3')] })

  it('j/k move the cursor, Enter opens', async () => {
    routes({ list: items, thread: [msg('1', { thread_id: 't-2' })] })
    wrap(<Harness />)
    await screen.findByText('Subject 1')
    press('j')
    press('j')
    press('j')
    expect(screen.getByRole('button', { name: /Subject 3/ })).toHaveAttribute('data-cursor', 'true')
    press('k')
    const row2 = screen.getByRole('button', { name: /Subject 2/ })
    expect(row2).toHaveAttribute('data-cursor', 'true')
    press('Enter')
    await waitFor(() => expect(row2).toHaveAttribute('aria-current', 'true'))
    expect(calls(mocks.GET, '/api/threads/{id}')[0]![1].params.path.id).toBe('t-2')
  })

  it('e archives and u marks unread the cursor message', async () => {
    routes({ list: items })
    wrap(<Harness />)
    await screen.findByText('Subject 1')
    press('j')
    press('j') // cursor on message 2 (first j lands on 1 since no cursor => index -1 + 1 = 0)
    press('e')
    await waitFor(() => expect(calls(mocks.POST, '/api/marks')).toHaveLength(1))
    expect(calls(mocks.POST, '/api/marks')[0]![1].body).toEqual({ message_ids: ['2'], state: 'archived' })
    press('u')
    await waitFor(() => expect(calls(mocks.POST, '/api/marks')).toHaveLength(2))
    expect(calls(mocks.POST, '/api/marks')[1]![1].body.state).toBe('unread')
  })

  it('r opens the reply composer and Escape closes it; typing in inputs is ignored', async () => {
    routes({ list: items, thread: [msg('1', { thread_id: 't-1' })] })
    wrap(<Harness />)
    await screen.findByText('Subject 1')
    press('j')
    press('r')
    await screen.findByRole('form', { name: 'Reply' })
    const body = screen.getByLabelText('Message body')
    body.focus()
    await userEvent.setup().keyboard('jkeu')
    expect(calls(mocks.POST, '/api/marks')).toHaveLength(0)
    press('Escape')
    await waitFor(() => expect(screen.queryByRole('form', { name: 'Reply' })).toBeNull())
  })

  it('c opens the compose store', async () => {
    routes({ list: items })
    wrap(<Harness />)
    await screen.findByText('Subject 1')
    press('c')
    expect(composeStore.isOpen()).toBe(true)
    press('Escape')
    expect(composeStore.isOpen()).toBe(false)
  })
})
