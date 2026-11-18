import type { ReactNode } from 'react'
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { MessageView } from '@/api/mail'
import type { TaskDetail, TaskView } from '@/api/tasks'
import { taskStore } from '@/lib/tasks'
import { treeFixture } from '@/test/fixtures'
import { renderWithRouter } from '@/test/utils'

const mocks = vi.hoisted(() => ({ GET: vi.fn(), POST: vi.fn(), toastError: vi.fn(), toastSuccess: vi.fn() }))
vi.mock('@/api/client', async (orig) => ({ ...(await orig<typeof import('@/api/client')>()), api: mocks }))
vi.mock('sonner', () => ({ toast: { error: mocks.toastError, success: mocks.toastSuccess } }))
// jsdom cannot run Lexical: same Markdown-in, Markdown-out contract as the real editor.
vi.mock('@/components/mail/MarkdownEditor', () => ({
  default: ({ value, onChange, label = 'Message body' }: { value: string; onChange: (m: string) => void; label?: string }) => (
    <textarea aria-label={label} value={value} onChange={(e) => onChange(e.target.value)} />
  ),
}))

import { ThreadView } from '@/components/mail/ThreadView'
import { CreateTaskSheet } from './CreateTaskSheet'
import { TaskDrawer } from './TaskDrawer'
import { TasksPage } from './TasksPage'

const ok = (data: unknown) => ({ data, response: new Response(null, { status: 200 }) })
const fail = (status: number, detail: string, suggestions?: string[]) => ({
  error: { detail, errors: suggestions ? [{ location: 'suggestions', value: suggestions }] : [] },
  response: new Response(null, { status }),
})

const ago = (ms: number) => new Date(Date.now() - ms).toISOString()
const mk = (over: Partial<TaskView>): TaskView => ({
  id: 'task-1',
  title: 'Task',
  body: '',
  status: 'not_started',
  priority: 'normal',
  created_by: 'operator',
  created_at: ago(3_600_000),
  updated_at: ago(60_000),
  ...over,
})

const REVIEWER = 'reviewer@github.com/acme/api'
let tasks: TaskView[]
let actionResult: (id: string, body: { action: string }) => unknown

const detail = (t: TaskView): TaskDetail => ({
  ...t,
  body_html: t.body ? `<p>${t.body}</p>` : '',
  events: [
    { actor: 'operator', at: ago(3_600_000), kind: 'created' },
    { actor: REVIEWER, at: ago(1_800_000), kind: 'comment', note: 'Looking into it' },
  ],
  links: [{ message_id: 'm-1', thread_id: 't-1', subject: 'Deploy plan', linked_by: 'operator', linked_at: ago(3_000_000) }],
})

function routes(threadTasks: unknown[] = []) {
  mocks.GET.mockImplementation(async (path: string, o?: { params?: { query?: Record<string, unknown>; path?: { id: string } } }) => {
    const q = o?.params?.query ?? {}
    if (path === '/api/tasks') {
      let items = tasks
      if (q.open_only) items = items.filter((t) => t.status !== 'finished')
      return ok({ items })
    }
    if (path === '/api/tasks/{id}') {
      const t = tasks.find((x) => x.id === o?.params?.path?.id)
      return t ? ok(detail(t)) : fail(404, 'no such task')
    }
    if (path === '/api/thread-tasks') return ok({ items: threadTasks })
    if (path === '/api/tree') return ok({ orgs: treeFixture })
    if (path === '/api/roster') {
      return ok({
        items: [REVIEWER, 'builder@github.com/acme/api'].map((address) => ({ address, role: 'dev', description: '', online: [], subagents: [] })),
      })
    }
    if (path === '/api/threads/{id}') return ok({ messages: [msg()] })
    return fail(404, 'nope')
  })
  mocks.POST.mockImplementation(async (path: string, o: { params?: { path?: { id: string } }; body: { action: string } }) => {
    if (path === '/api/tasks/{id}/actions') return actionResult(o.params!.path!.id, o.body)
    if (path === '/api/tasks') return ok(detail(mk({ id: 'task-new', title: (o.body as unknown as { title: string }).title })))
    return ok({})
  })
}

const msg = (): MessageView => ({
  id: 'm-1',
  thread_id: 't-1',
  from: REVIEWER,
  subject: 'Re: Deploy plan',
  body: 'b',
  body_html: '<p>b</p>',
  snippet: 'b',
  created_at: '2026-11-17T10:00:00Z',
  to: ['operator'],
  cc: [],
  muted: false,
})

function wrap(ui: ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return <QueryClientProvider client={qc}>{ui}</QueryClientProvider>
}
const renderBoard = () =>
  renderWithRouter(
    wrap(
      <>
        <TasksPage />
        <TaskDrawer />
        <CreateTaskSheet />
      </>,
    ),
    '/tasks',
  )

const col = (name: string) => screen.getByRole('region', { name })
const card = (title: string) => screen.getByRole('button', { name: title }).closest('li') as HTMLElement
const actionCalls = () => mocks.POST.mock.calls.filter((c) => c[0] === '/api/tasks/{id}/actions').map((c) => c[1].body as Record<string, unknown>)

function drag(title: string, to: string) {
  const dataTransfer = { setData: vi.fn(), effectAllowed: '', dropEffect: '' }
  fireEvent.dragStart(card(title), { dataTransfer })
  fireEvent.dragOver(col(to), { dataTransfer })
  fireEvent.drop(col(to), { dataTransfer })
}

beforeEach(() => {
  vi.clearAllMocks()
  taskStore.reset()
  tasks = [
    mk({ id: 'task-a', title: 'Write release notes', priority: 'high', scope: 'github.com/acme/api', assignee: REVIEWER }),
    mk({ id: 'task-b', title: 'Fix flaky test', status: 'claimed', claimant: REVIEWER, health: 'active', last_activity_at: ago(30_000) }),
    mk({ id: 'task-c', title: 'Migrate schema', status: 'started', claimant: REVIEWER, health: 'stale', last_activity_at: ago(7_200_000), due_at: ago(86_400_000), priority: 'urgent' }),
    mk({ id: 'task-d', title: 'Rotate keys', status: 'abandoned', health: 'abandoned' }),
    mk({ id: 'task-e', title: 'Ship v1', status: 'finished', finished_at: ago(1000) }),
  ]
  actionResult = (id, body) => {
    const t = tasks.find((x) => x.id === id)!
    return ok(detail({ ...t, ...(body.action === 'start' ? { status: 'started' as const } : {}) }))
  }
  routes()
})

describe('TasksPage board', () => {
  it('groups cards by status and shows priority, health, due, scope and people', async () => {
    await renderBoard()
    await screen.findByText('Write release notes')
    expect(within(col('Not started')).getByText('Write release notes')).toBeInTheDocument()
    expect(within(col('Claimed')).getByText('Fix flaky test')).toBeInTheDocument()
    expect(within(col('Started')).getByText('Migrate schema')).toBeInTheDocument()
    expect(within(col('Abandoned')).getByText('Rotate keys')).toBeInTheDocument()

    const a = within(card('Write release notes'))
    expect(a.getByText('high')).toBeInTheDocument()
    expect(a.getByText('github.com/acme/api')).toBeInTheDocument()
    expect(a.getByLabelText(`Assigned to ${REVIEWER}`)).toHaveTextContent('reviewer')

    expect(within(card('Fix flaky test')).getByLabelText(`Claimed by ${REVIEWER}`)).toBeInTheDocument()
    expect(within(card('Migrate schema')).getByText('urgent')).toBeInTheDocument()
  })

  it('shows a distinct, labelled health indicator with last activity', async () => {
    await renderBoard()
    await screen.findByText('Fix flaky test')
    const active = within(card('Fix flaky test')).getByRole('img', { name: /Health: active, last activity 30s ago/ })
    const stale = within(card('Migrate schema')).getByRole('img', { name: /Health: stale, last activity 2h ago/ })
    expect(active).toHaveAttribute('data-health', 'active')
    expect(stale).toHaveAttribute('data-health', 'stale')
    expect(stale).toHaveAttribute('title', expect.stringContaining('last activity 2h ago'))
    expect(within(card('Rotate keys')).getByRole('img', { name: /Health: abandoned/ })).toBeInTheDocument()
    // Unclaimed tasks have no health.
    expect(within(card('Write release notes')).queryByRole('img', { name: /Health/ })).toBeNull()
  })

  it('highlights overdue deadlines', async () => {
    await renderBoard()
    await screen.findByText('Migrate schema')
    const due = within(card('Migrate schema')).getByText(/Overdue, due/).parentElement!
    expect(due).toHaveAttribute('data-overdue', 'true')
  })

  it('keeps finished tasks out of the board until asked, and collapses the column', async () => {
    const user = userEvent.setup()
    await renderBoard()
    await screen.findByText('Write release notes')
    expect(screen.queryByText('Ship v1')).toBeNull()
    expect(col('Finished')).toHaveAttribute('data-collapsed', 'true')
    expect(mocks.GET.mock.calls.find((c) => c[0] === '/api/tasks')![1].params.query).toMatchObject({ open_only: true })

    await user.click(screen.getByRole('checkbox', { name: 'Show finished' }))
    expect(await within(col('Finished')).findByText('Ship v1')).toBeInTheDocument()
    expect(col('Finished')).not.toHaveAttribute('data-collapsed')
    const last = mocks.GET.mock.calls.filter((c) => c[0] === '/api/tasks').at(-1)![1].params.query
    expect(last.open_only).toBeUndefined()

    await user.click(screen.getByRole('button', { name: 'Collapse finished column' }))
    expect(col('Finished')).toHaveAttribute('data-collapsed', 'true')
  })

  it('filters by scope, assignee and operator involvement on the server and by priority locally', async () => {
    const user = userEvent.setup()
    await renderBoard()
    await screen.findByText('Write release notes')
    const lastQuery = () => mocks.GET.mock.calls.filter((c) => c[0] === '/api/tasks').at(-1)![1].params.query

    await user.selectOptions(screen.getByRole('combobox', { name: 'Scope' }), 'github.com/acme/api')
    await waitFor(() => expect(lastQuery()).toMatchObject({ scope: 'github.com/acme/api' }))
    await user.selectOptions(screen.getByRole('combobox', { name: 'Assignee' }), REVIEWER)
    await waitFor(() => expect(lastQuery()).toMatchObject({ assignee: REVIEWER }))
    await user.click(screen.getByRole('checkbox', { name: 'Involves operator' }))
    await waitFor(() => expect(lastQuery()).toMatchObject({ involves: 'operator' }))

    await user.click(screen.getByRole('button', { name: 'Clear filters' }))
    await waitFor(() => expect(lastQuery().scope).toBeUndefined())

    await user.selectOptions(screen.getByRole('combobox', { name: 'Priority' }), 'urgent')
    expect(screen.getByText('Migrate schema')).toBeInTheDocument()
    expect(screen.queryByText('Write release notes')).toBeNull()
  })

  it('says so when there are no tasks', async () => {
    tasks = []
    await renderBoard()
    expect(await screen.findByText(/No open tasks yet/)).toBeInTheDocument()
  })

  it('opens the drawer from a card and the create sheet from New task', async () => {
    const user = userEvent.setup()
    await renderBoard()
    await user.click(await screen.findByRole('button', { name: /New task/ }))
    expect(taskStore.get().create).toEqual({})
    await user.keyboard('{Escape}')
    await waitFor(() => expect(taskStore.get().create).toBeUndefined())
    await user.click(screen.getByRole('button', { name: 'Fix flaky test' }))
    expect(taskStore.get().selected).toBe('task-b')
  })
})

describe('drag and drop', () => {
  it('start: not_started to started runs the start action', async () => {
    await renderBoard()
    await screen.findByText('Write release notes')
    drag('Write release notes', 'Started')
    await waitFor(() => expect(actionCalls()).toEqual([{ action: 'start' }]))
    expect(mocks.POST.mock.calls[0]![1].params.path.id).toBe('task-a')
  })

  it('moves the card at once, before the server answers', async () => {
    let release!: () => void
    actionResult = () => new Promise((res) => (release = () => res(ok(detail(tasks[0]!)))))
    await renderBoard()
    await screen.findByText('Write release notes')
    drag('Write release notes', 'Started')
    expect(await within(col('Started')).findByText('Write release notes')).toBeInTheDocument()
    expect(within(col('Not started')).queryByText('Write release notes')).toBeNull()
    await act(async () => release())
  })

  it('finish: started to finished runs finish', async () => {
    await renderBoard()
    await screen.findByText('Migrate schema')
    drag('Migrate schema', 'Finished')
    await waitFor(() => expect(actionCalls()).toEqual([{ action: 'finish' }]))
  })

  it('release asks first, and does nothing when cancelled', async () => {
    const user = userEvent.setup()
    await renderBoard()
    await screen.findByText('Migrate schema')
    drag('Migrate schema', 'Not started')
    const dlg = await screen.findByRole('dialog', { name: /Move to Not started/ })
    expect(dlg).toHaveTextContent('Release this task')
    await user.click(within(dlg).getByRole('button', { name: 'Cancel' }))
    expect(actionCalls()).toEqual([])

    drag('Migrate schema', 'Not started')
    await user.click(await screen.findByRole('button', { name: 'Move task' }))
    await waitFor(() => expect(actionCalls()).toEqual([{ action: 'release' }]))
  })

  it('finishing an unstarted task starts it first, after confirmation', async () => {
    const user = userEvent.setup()
    await renderBoard()
    await screen.findByText('Write release notes')
    drag('Write release notes', 'Finished')
    await user.click(await screen.findByRole('button', { name: 'Move task' }))
    await waitFor(() => expect(actionCalls()).toEqual([{ action: 'start' }, { action: 'finish' }]))
  })

  it('reopens an abandoned task dropped on not started, without asking', async () => {
    await renderBoard()
    await screen.findByText('Rotate keys')
    drag('Rotate keys', 'Not started')
    await waitFor(() => expect(actionCalls()).toEqual([{ action: 'reopen' }]))
  })

  it('refuses a drop on abandoned with a toast and no request', async () => {
    await renderBoard()
    await screen.findByText('Migrate schema')
    drag('Migrate schema', 'Abandoned')
    expect(mocks.toastError).toHaveBeenCalledWith(expect.stringContaining('abandoned'))
    expect(actionCalls()).toEqual([])
  })

  it('toasts a 409 and rolls the card back', async () => {
    actionResult = () => fail(409, 'invalid transition: task is already finished')
    await renderBoard()
    await screen.findByText('Write release notes')
    drag('Write release notes', 'Started')
    await waitFor(() => expect(mocks.toastError).toHaveBeenCalledWith('invalid transition: task is already finished'))
    await waitFor(() => expect(within(col('Not started')).getByText('Write release notes')).toBeInTheDocument())
    expect(within(col('Started')).queryByText('Write release notes')).toBeNull()
  })
})

describe('TaskDrawer', () => {
  const open = async (title: string) => {
    const user = userEvent.setup()
    await renderBoard()
    await user.click(await screen.findByRole('button', { name: title }))
    const dlg = await screen.findByRole('dialog', { name: title })
    await within(dlg).findByText('Looking into it')
    return { user, dlg }
  }

  it('shows body, history and linked threads', async () => {
    tasks[0]!.body = 'Compile the **changes**'
    const { dlg } = await open('Write release notes')
    expect(within(dlg).getByText('Compile the **changes**')).toBeInTheDocument()
    const history = within(dlg).getByRole('list', { name: 'History' })
    expect(within(history).getByText(/created the task/)).toBeInTheDocument()
    expect(within(history).getByText(/commented/)).toBeInTheDocument()
    expect(within(dlg).getByRole('button', { name: /Deploy plan/ })).toBeInTheDocument()
  })

  it('offers only the actions valid for the status', async () => {
    const { dlg } = await open('Migrate schema')
    const group = within(dlg).getByRole('group', { name: 'Task actions' })
    expect(within(group).getAllByRole('button').map((b) => b.textContent)).toEqual(['Finish', 'Release'])
  })

  it('runs lifecycle actions', async () => {
    const { user, dlg } = await open('Write release notes')
    await user.click(within(dlg).getByRole('button', { name: 'Start' }))
    await waitFor(() => expect(actionCalls()).toEqual([{ action: 'start' }]))
  })

  it('edits the title inline', async () => {
    const { user, dlg } = await open('Write release notes')
    await user.click(within(dlg).getByRole('button', { name: 'Write release notes' }))
    const input = within(dlg).getByRole('textbox', { name: 'Title' })
    await user.clear(input)
    await user.type(input, 'Write the notes{Enter}')
    await waitFor(() => expect(actionCalls()).toEqual([{ action: 'edit', title: 'Write the notes' }]))
  })

  it('edits priority, scope and clears the due date', async () => {
    const { user, dlg } = await open('Migrate schema')
    await user.selectOptions(within(dlg).getByRole('combobox', { name: 'Priority' }), 'low')
    await waitFor(() => expect(actionCalls()).toContainEqual({ action: 'edit', priority: 'low' }))
    const scope = within(dlg).getByRole('combobox', { name: 'Scope' })
    await user.type(scope, 'github.com/acme/web{Enter}')
    await waitFor(() => expect(actionCalls()).toContainEqual({ action: 'edit', scope: 'github.com/acme/web' }))
    await user.click(within(dlg).getByRole('button', { name: 'Clear due date' }))
    await waitFor(() => expect(actionCalls()).toContainEqual({ action: 'edit', clear_due: true }))
  })

  it('edits the body in the Markdown editor', async () => {
    const { user, dlg } = await open('Write release notes')
    await user.click(within(dlg).getByRole('button', { name: 'Edit' }))
    // The lazy editor first shows a disabled placeholder with the same name.
    await waitFor(() => expect(within(dlg).getByRole('textbox', { name: 'Description' })).toBeEnabled())
    await user.type(within(dlg).getByRole('textbox', { name: 'Description' }), 'New plan')
    await user.click(within(dlg).getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(actionCalls()).toEqual([{ action: 'edit', body: 'New plan' }]))
  })

  it('posts a comment and clears the box', async () => {
    const { user, dlg } = await open('Write release notes')
    const box = within(dlg).getByRole('textbox', { name: 'Comment' })
    expect(within(dlg).getByRole('button', { name: 'Comment' })).toBeDisabled()
    await user.type(box, 'Needs review')
    await user.click(within(dlg).getByRole('button', { name: 'Comment' }))
    await waitFor(() => expect(actionCalls()).toEqual([{ action: 'comment', note: 'Needs review' }]))
    await waitFor(() => expect(box).toHaveValue(''))
  })

  it('assigns through the identity type-ahead and unassigns', async () => {
    const { user, dlg } = await open('Fix flaky test')
    const box = within(dlg).getByRole('combobox', { name: 'Assign to' })
    await user.type(box, 'build')
    const list = await within(dlg).findByRole('listbox', { name: 'Assign to suggestions' })
    const opts = within(list).getAllByRole('option')
    // Identities only: no orgs, repos or subagents.
    expect(opts).toHaveLength(1)
    await user.click(opts[0]!)
    expect(box).toHaveValue('builder@github.com/acme/api')
    await user.click(within(dlg).getByRole('button', { name: 'Assign' }))
    await waitFor(() => expect(actionCalls()).toEqual([{ action: 'assign', assignee: 'builder@github.com/acme/api' }]))
  })

  it('offers suggestions when the assignee is unknown', async () => {
    actionResult = (_id, body) => (body.action === 'assign' ? fail(404, 'unknown assignee', [REVIEWER]) : ok(detail(tasks[0]!)))
    const { user, dlg } = await open('Fix flaky test')
    await user.type(within(dlg).getByRole('combobox', { name: 'Assign to' }), 'nobody@github.com/acme/api')
    await user.keyboard('{Enter}')
    const alert = await within(dlg).findByRole('alert')
    expect(alert).toHaveTextContent('unknown assignee')
    actionResult = (id) => ok(detail(tasks.find((t) => t.id === id)!))
    await user.click(within(alert).getByRole('button', { name: REVIEWER }))
    await waitFor(() => expect(actionCalls().at(-1)).toEqual({ action: 'assign', assignee: REVIEWER }))
  })

  it('unassigns an assigned task', async () => {
    const { user, dlg } = await open('Write release notes')
    await user.click(within(dlg).getByRole('button', { name: 'Unassign' }))
    await waitFor(() => expect(actionCalls()).toEqual([{ action: 'unassign' }]))
  })

  it('closes and clears the selection when a linked thread is opened', async () => {
    const { user, dlg } = await open('Write release notes')
    await user.click(within(dlg).getByRole('button', { name: /Deploy plan/ }))
    expect(taskStore.get().selected).toBeUndefined()
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
  })

  it('says when the task is gone', async () => {
    await renderBoard()
    act(() => taskStore.select('task-missing'))
    expect(await screen.findByText('This task no longer exists.')).toBeInTheDocument()
  })
})

describe('CreateTaskSheet', () => {
  it('creates a task with every field and opens it', async () => {
    const user = userEvent.setup()
    await renderBoard()
    await user.click(await screen.findByRole('button', { name: /New task/ }))
    const sheet = await screen.findByRole('dialog', { name: 'New task' })
    await user.type(within(sheet).getByRole('textbox', { name: 'Title' }), 'Audit logs')
    await waitFor(() => expect(within(sheet).getByRole('textbox', { name: 'Description' })).toBeEnabled())
    await user.type(within(sheet).getByRole('textbox', { name: 'Description' }), 'Look for errors')
    await user.selectOptions(within(sheet).getByRole('combobox', { name: 'Priority' }), 'high')
    await user.type(within(sheet).getByRole('combobox', { name: 'Scope' }), 'github.com/acme/api')
    fireEvent.change(within(sheet).getByLabelText('Due date'), { target: { value: '2026-12-05' } })
    await user.type(within(sheet).getByRole('combobox', { name: 'Assignee' }), `${REVIEWER}{Enter}`)
    await user.click(within(sheet).getByRole('button', { name: /Create task/ }))

    await waitFor(() => expect(mocks.POST).toHaveBeenCalledWith('/api/tasks', expect.anything()))
    const body = mocks.POST.mock.calls.find((c) => c[0] === '/api/tasks')![1].body
    expect(body).toMatchObject({ title: 'Audit logs', body: 'Look for errors', priority: 'high', scope: 'github.com/acme/api', assignee: REVIEWER })
    expect(body.due_at).toMatch(/^2026-12-0[56]T/)
    expect(body.message_id).toBeUndefined()
    await waitFor(() => expect(taskStore.get().selected).toBe('task-new'))
    expect(taskStore.get().create).toBeUndefined()
  })

  it('requires a title and validates the scope', async () => {
    const user = userEvent.setup()
    await renderBoard()
    act(() => taskStore.openCreate())
    const sheet = await screen.findByRole('dialog', { name: 'New task' })
    await user.click(within(sheet).getByRole('button', { name: /Create task/ }))
    expect(await within(sheet).findByRole('alert')).toHaveTextContent('Title is required')
    await user.type(within(sheet).getByRole('textbox', { name: 'Title' }), 'x')
    await user.type(within(sheet).getByRole('combobox', { name: 'Scope' }), 'not a scope')
    await user.click(within(sheet).getByRole('button', { name: /Create task/ }))
    expect(await within(sheet).findByRole('alert')).toHaveTextContent('Invalid scope')
    expect(mocks.POST).not.toHaveBeenCalled()
  })

  it('offers suggestions for an unknown assignee', async () => {
    mocks.POST.mockImplementation(async () => fail(404, 'unknown assignee ghost@github.com/acme/api', [REVIEWER]))
    const user = userEvent.setup()
    await renderBoard()
    act(() => taskStore.openCreate({ title: 'Prefilled' }))
    const sheet = await screen.findByRole('dialog', { name: 'New task' })
    expect(within(sheet).getByRole('textbox', { name: 'Title' })).toHaveValue('Prefilled')
    await user.type(within(sheet).getByRole('combobox', { name: 'Assignee' }), 'ghost@github.com/acme/api')
    await user.click(within(sheet).getByRole('button', { name: /Create task/ }))
    await user.click(await within(sheet).findByRole('button', { name: REVIEWER }))
    expect(within(sheet).getByRole('combobox', { name: 'Assignee' })).toHaveValue(REVIEWER)
  })

  it('links the message when created from a thread, and keeps the thread open', async () => {
    const user = userEvent.setup()
    await renderBoard()
    act(() => taskStore.openCreate({ title: 'Deploy plan', messageId: 'm-1' }))
    const sheet = await screen.findByRole('dialog', { name: 'New task' })
    expect(sheet).toHaveTextContent('latest message in this thread will be linked')
    await user.click(within(sheet).getByRole('button', { name: /Create task/ }))
    await waitFor(() => expect(mocks.POST.mock.calls.find((c) => c[0] === '/api/tasks')![1].body).toMatchObject({ title: 'Deploy plan', message_id: 'm-1' }))
    await waitFor(() => expect(taskStore.get().create).toBeUndefined())
    expect(taskStore.get().selected).toBeUndefined()
  })
})

describe('thread integration', () => {
  const thread = () =>
    render(wrap(<ThreadView threadId="t-1" replying={false} onReplying={() => {}} onArchived={() => {}} />))

  it('shows linked tasks as chips with status, and opens the drawer on click', async () => {
    routes([{ thread_id: 't-1', task_id: 'task-b', title: 'Fix flaky test', status: 'claimed' }])
    const user = userEvent.setup()
    thread()
    const chip = await screen.findByRole('button', { name: 'Task: Fix flaky test (Claimed)' })
    expect(chip).toHaveAttribute('data-status', 'claimed')
    await user.click(chip)
    expect(taskStore.get().selected).toBe('task-b')
    expect(mocks.GET).toHaveBeenCalledWith('/api/thread-tasks', { params: { query: { thread_id: ['t-1'] } } })
  })

  it('shows no chip row for an unlinked thread', async () => {
    thread()
    await screen.findByText('Re: Deploy plan')
    expect(screen.queryByRole('list', { name: 'Linked tasks' })).toBeNull()
  })

  it('creates a task from the thread, prefilling the subject and the latest message', async () => {
    const user = userEvent.setup()
    thread()
    await user.click(await screen.findByRole('button', { name: 'Create task from thread' }))
    expect(taskStore.get().create).toEqual({ title: 'Deploy plan', messageId: 'm-1' })
  })
})
