import { useState, type ReactNode } from 'react'
import { fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { Header, MailSearch, MessageView } from '@/api/mail'
import { mockViewport } from '@/test/utils'

const mocks = vi.hoisted(() => ({ GET: vi.fn(), POST: vi.fn() }))
vi.mock('@/api/client', async (orig) => ({ ...(await orig<typeof import('@/api/client')>()), api: mocks }))
vi.mock('./MarkdownEditor', () => ({ default: () => <textarea aria-label="Message body" /> }))

import { MailView } from './MailView'
import { LIST_DEFAULT, LIST_MIN, LIST_WIDTH_KEY, SPLIT_MIN, THREAD_MIN, clampListWidth, mailLayout } from './mailLayout'

const ok = (data: unknown) => ({ data, response: new Response(null, { status: 200 }) })
const hdr: Header = {
  id: '1',
  thread_id: 't-1',
  from: 'reviewer@github.com/acme/api',
  subject: 'Subject 1',
  created_at: new Date().toISOString(),
  to: ['operator'],
  cc: [],
  mailbox: 'operator',
  kind: 'message',
  snippet: 'Snippet 1',
  thread_count: 1,
  muted: false,
}
const msg: MessageView = {
  id: '1',
  thread_id: 't-1',
  from: 'reviewer@github.com/acme/api',
  subject: 'Subject 1',
  body: 'body',
  body_html: '<p>body</p>',
  snippet: 'body',
  created_at: '2026-11-17T10:00:00Z',
  to: ['operator'],
  cc: [],
  muted: false,
}

function wrap(ui: ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(<QueryClientProvider client={qc}>{ui}</QueryClientProvider>)
}

function Harness({ initial = {} }: { initial?: MailSearch }) {
  const [s, setS] = useState<MailSearch>(initial)
  return <MailView search={s} onSearch={(p) => setS((prev) => ({ ...prev, ...p }))} />
}

/** Every element measures `width` px wide. */
function paneWidth(width: number) {
  vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockReturnValue({ width, height: 600, top: 0, left: 0, right: width, bottom: 600, x: 0, y: 0, toJSON: () => ({}) })
}

beforeEach(() => {
  vi.clearAllMocks()
  localStorage.clear()
  mocks.GET.mockImplementation(async (path: string) => {
    if (path === '/api/messages') return ok({ items: [hdr] })
    if (path === '/api/threads/{id}') return ok({ messages: [msg] })
    return { error: { detail: 'nope' }, response: new Response(null, { status: 404 }) }
  })
  mocks.POST.mockImplementation(async () => ok({}))
})
afterEach(() => vi.restoreAllMocks())

describe('mailLayout', () => {
  it('splits from 1100px when the pane fits both, else shows one pane', () => {
    expect(mailLayout(SPLIT_MIN, false)).toBe('split')
    expect(mailLayout(SPLIT_MIN - 1, false)).toBe('single')
    expect(mailLayout(0, false)).toBe('split') // not measured yet
    expect(mailLayout(1600, true)).toBe('single') // narrow window
  })

  it('clamps the list so the thread keeps its minimum', () => {
    expect(clampListWidth(100, 1000)).toBe(LIST_MIN)
    expect(clampListWidth(900, 1000)).toBe(1000 - THREAD_MIN)
    expect(clampListWidth(LIST_DEFAULT, 1000)).toBe(LIST_DEFAULT)
  })
})

describe('MailView layout', () => {
  afterEach(() => {
    delete (window as { matchMedia?: unknown }).matchMedia
  })

  it('below 1100px: list or thread, full width, with Back', async () => {
    const user = userEvent.setup()
    mockViewport(1024)
    paneWidth(776)
    wrap(<Harness />)
    expect(await screen.findByTestId('mail-pane')).toHaveAttribute('data-layout', 'single')
    expect(screen.queryByRole('article', { name: 'Thread' })).toBeNull()
    expect(screen.queryByRole('separator')).toBeNull()
    await user.click(await screen.findByRole('button', { name: /Subject 1/ }))
    expect(await screen.findByRole('article', { name: 'Thread' })).toBeInTheDocument()
    expect(screen.queryByRole('region', { name: 'Message list' })).toBeNull()
    await user.click(screen.getByRole('button', { name: 'Back' }))
    expect(await screen.findByRole('region', { name: 'Message list' })).toBeInTheDocument()
  })

  it('a wide window with a cramped pane stays single-pane', async () => {
    mockViewport(1440)
    paneWidth(SPLIT_MIN - 40)
    wrap(<Harness initial={{ thread: 't-1' }} />)
    expect(await screen.findByTestId('mail-pane')).toHaveAttribute('data-layout', 'single')
    expect(await screen.findByRole('button', { name: 'Back' })).toBeInTheDocument()
  })

  it('wide pane: list and thread side by side at the remembered list width', async () => {
    mockViewport(1440)
    paneWidth(1000)
    localStorage.setItem(LIST_WIDTH_KEY, '480')
    wrap(<Harness initial={{ thread: 't-1' }} />)
    expect(await screen.findByTestId('mail-pane')).toHaveAttribute('data-layout', 'split')
    const list = await screen.findByRole('region', { name: 'Message list' })
    expect(list).toHaveStyle({ width: '480px' })
    expect(screen.getByRole('article', { name: 'Thread' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Back' })).toBeNull()
  })

  it('resizes the list with the keyboard and remembers it', async () => {
    paneWidth(1000)
    wrap(<Harness />)
    const sep = await screen.findByRole('separator', { name: 'Resize message list' })
    expect(sep).toHaveAttribute('aria-valuenow', String(LIST_DEFAULT))
    fireEvent.keyDown(sep, { key: 'ArrowRight' })
    expect(sep).toHaveAttribute('aria-valuenow', String(LIST_DEFAULT + 16))
    expect(localStorage.getItem(LIST_WIDTH_KEY)).toBe(String(LIST_DEFAULT + 16))
    expect(screen.getByRole('region', { name: 'Message list' })).toHaveStyle({ width: `${LIST_DEFAULT + 16}px` })
  })
})
